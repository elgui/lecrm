package auth

// Bearer-token authentication for workspace-scoped requests
// (ADR-009 §4.1 service tokens, ADR-011 §6 connector auth).
//
// The flow is:
//
//   1. workspace.Middleware resolves the workspace from the request
//      subdomain (as before — no change to that path).
//   2. If the request carries `Authorization: Bearer …`, this package
//      decodes the bearer, asserts the embedded workspace slug matches
//      the subdomain-resolved workspace, and argon2id-verifies the
//      candidate against the stored hashes for that workspace.
//   3. On a match, the resolved BearerActor (id + actor_type + scopes)
//      is deposited into the request context for downstream consumers
//      (audit log, scope checks).
//
// If the bearer is malformed or no match is found the middleware
// returns 401 — downstream handlers must not see a partial-success
// state. If no Authorization header is present, the request falls
// through to the session-cookie path (handled by the caller).

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// BearerActor describes a verified service-token actor for the
// current request. It is deposited into the request context by
// BearerVerifier and read by emitAudit / scope checks.
type BearerActor struct {
	TokenID   uuid.UUID
	ActorType string   // human_api | mcp_agent | internal_service | connector
	Scopes    []string // application-defined; "*" = full access
}

type bearerCtxKey struct{}

// WithBearerActor returns a derived context carrying actor.
func WithBearerActor(ctx context.Context, actor *BearerActor) context.Context {
	return context.WithValue(ctx, bearerCtxKey{}, actor)
}

// BearerActorFromContext returns the BearerActor deposited by the
// middleware, or nil + false when no service-token actor authenticated
// this request (e.g. session-cookie path).
func BearerActorFromContext(ctx context.Context) (*BearerActor, bool) {
	a, ok := ctx.Value(bearerCtxKey{}).(*BearerActor)
	return a, ok && a != nil
}

// ExtractBearer returns the value following "Bearer " in the
// Authorization header, or "" when no bearer is present.
func ExtractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// TokenCandidate is one row returned by the workspace lookup. The
// middleware iterates candidates and constant-time-verifies each.
type TokenCandidate struct {
	ID        uuid.UUID
	Hash      string
	ActorType string
	Scopes    []byte // raw jsonb
	ExpiresAt *int64 // unix; nil = never
}

// CandidateLoader is the seam the middleware uses to load token rows
// for a given workspace. The production implementation hits
// core.service_tokens via sqlc; tests pass an in-memory stub.
type CandidateLoader interface {
	LoadCandidates(ctx context.Context, workspaceID uuid.UUID) ([]TokenCandidate, error)
	TouchLastUsed(ctx context.Context, tokenID uuid.UUID) error
}

// VerifyBearer matches the candidate plaintext against every active
// row for the resolved workspace. Returns the matched actor on
// success.
//
// The check rejects tokens whose embedded slug does not match the
// resolved workspace slug, even if the hash would otherwise match a
// token from a different tenant. This is defence-in-depth: the table
// is workspace-scoped, but a slug mismatch is a hard "no" before any
// expensive verify.
func VerifyBearer(
	ctx context.Context,
	loader CandidateLoader,
	resolvedWorkspaceID uuid.UUID,
	resolvedWorkspaceSlug string,
	plaintext string,
) (*BearerActor, error) {
	embeddedSlug, err := WorkspaceSlugFromToken(plaintext)
	if err != nil {
		return nil, err
	}
	if embeddedSlug != resolvedWorkspaceSlug {
		return nil, errors.New("service token: workspace mismatch")
	}

	cands, err := loader.LoadCandidates(ctx, resolvedWorkspaceID)
	if err != nil {
		return nil, err
	}
	for _, c := range cands {
		if err := VerifyServiceToken(plaintext, c.Hash); err == nil {
			scopes := decodeScopes(c.Scopes)
			// Touch last_used_at in the background; failures here
			// must not block the request. We deliberately swallow
			// the error and log nothing — the SQL is idempotent and
			// transient failures will resolve on the next request.
			_ = loader.TouchLastUsed(ctx, c.ID)
			return &BearerActor{
				TokenID:   c.ID,
				ActorType: c.ActorType,
				Scopes:    scopes,
			}, nil
		}
	}
	return nil, errors.New("service token: no match")
}

func decodeScopes(raw []byte) []string {
	if len(raw) == 0 {
		return []string{"*"}
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil || len(out) == 0 {
		return []string{"*"}
	}
	return out
}
