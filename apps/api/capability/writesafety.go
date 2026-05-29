// Package capability – MCP write-safety primitives (ADR-012 §6/§7, TO
// RESOLVE 3 & 4).
//
// This file implements the *shared* safety controls every MCP write tool
// reuses — defined here before any write tool exists, so the next increment
// (advance_deal / log_interaction / capture_lead) only composes them. It is
// the MCP counterpart of the OpenAPI + service-token contract tasket
// (20260525-1006): a contract first, tools second.
//
// The controls and their reuse (ADR-012 §6):
//
//   - Scope → RBAC: a token's scope set maps to a capability Role; a write
//     op authorizes against that Role. Read-only scopes can never reach a
//     write op (primary blast-radius control, §8).
//   - Dry-run / preview: a write op accepts DryRun → returns the would-be
//     effect (a Preview/diff), no mutation.
//   - Confirmation handshake: destructive/bulk ops require a confirmation
//     token returned by a prior dry-run and echoed on the real call. The
//     token is HMAC-bound to the planned effect, so a confirmed write can
//     only commit the exact effect that was previewed.
//   - Idempotency: a duplicate call replays the cached result
//     (core.idempotency_keys, ADR-007).
//   - Audit attribution: handled by the existing fail-closed EmitAudit
//     inside the apply tx (actor_type from the Principal, never hardcoded).
//
// Mechanism-agnostic by construction (ADR-012 §7): every decision here reads
// only Principal fields (Role/Scopes/WorkspaceID/ActorType). Nothing depends
// on *how* the principal authenticated — a service token today, an OAuth
// access token later — so the deferred end-user-LLM (S4) path is a pure
// edge addition with zero tool changes. No helper assumes a single trusted
// machine actor, and no path hardcodes actor_type=mcp_agent as the only
// possible writer.
package capability

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// --- write scopes & scope→role mapping (ADR-012 §6, ADR-009 §4.1) ---

// Service-token / access-token scope literals. A write tool declares the
// scope it needs; the edge stamps the granted scopes onto the Principal.
// These mirror the scope vocabulary rbac.roleFromScopes already recognises
// (any scope containing "write", or the "*" wildcard, grants write) so a
// single token works identically against REST and MCP.
const (
	// ScopeWildcard grants full CRM read+write (capped at admin — never
	// owner; owner-only ops are not delegated to long-lived tokens).
	ScopeWildcard = "*"
	// ScopeCRMRead is the canonical read-only MCP scope.
	ScopeCRMRead = "crm:read"
	// ScopeCRMWrite is the canonical write scope every MCP write tool
	// declares. A token lacking it (read-only) is denied at AuthorizeWrite.
	ScopeCRMWrite = "crm:write"
)

// RoleFromScopes maps a token's scope set to the effective capability Role.
// It is mechanism-agnostic: the same mapping applies whether the scopes came
// from a service token (today) or a future OAuth access token (§7).
//
// It mirrors rbac.roleFromScopes verbatim in policy — any "*" or any scope
// containing "write" grants RoleAdmin; everything else (read scopes, empty,
// unrecognised) resolves to RoleMember — but returns capability.Role so the
// capability layer stays free of the rbac package (which imports net/http;
// see the Role doc in capability.go). Service tokens / delegated agents are
// deliberately capped at RoleAdmin: member management, token administration
// and workspace deletion are owner-only and must come from a human session,
// never a delegated token (ADR-012 §6, §7 non-foreclosure checklist).
func RoleFromScopes(scopes []string) Role {
	for _, s := range scopes {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == ScopeWildcard || strings.Contains(s, "write") {
			return RoleAdmin
		}
	}
	return RoleMember
}

// ErrReadOnlyScope is the clean denial returned when a write op is attempted
// with a token whose scopes grant read access only. It is distinct from
// ErrForbidden so the adapter can hand the agent an actionable message
// ("this token is read-only") rather than a bare 403. Transport adapters map
// it to 403.
var ErrReadOnlyScope = errors.New("write requires a write-scoped token; this token grants read-only access")

// AuthorizeWrite authorizes a write operation against the Principal and is
// the single gate every MCP write tool calls first. It inspects only
// Principal fields, never the authentication mechanism (§7).
//
// Two gates, belt-and-suspenders:
//
//  1. Scope gate (primary blast-radius control, §8): when the Principal
//     carries scopes (token-based callers), they must resolve to a
//     write-capable role; a read-only scope set yields ErrReadOnlyScope even
//     if Role were somehow mis-set. A scope-less Principal (e.g. a human
//     admin session, which authorizes purely by membership role) skips this
//     gate.
//  2. Role gate: the resolved Role must be RoleAdmin+ (RoleNone →
//     ErrUnauthenticated, present-but-insufficient → ErrForbidden), reusing
//     the same authorize() the REST write ops use.
func AuthorizeWrite(p Principal) error {
	if p.Role == RoleNone {
		return ErrUnauthenticated
	}
	if len(p.Scopes) > 0 && !RoleFromScopes(p.Scopes).AtLeast(RoleAdmin) {
		return ErrReadOnlyScope
	}
	return authorize(p, RoleAdmin)
}

// --- dry-run preview shape (ADR-012 §6, TO RESOLVE 4) ---

// Effect is one would-be change a write op intends to make, described
// without mutating. It is the unit of a dry-run Preview and the substrate of
// the confirmation digest.
type Effect struct {
	// Action is the mutation verb: "create" | "update" | "delete" |
	// "stage_change". Free-form but stable per tool.
	Action string `json:"action"`
	// EntityType is the domain entity ("contact" | "company" | "deal" | …).
	EntityType string `json:"entity_type"`
	// EntityID is the affected row, empty for a not-yet-created entity.
	EntityID string `json:"entity_id,omitempty"`
	// Before/After are the field-level diff. Before is omitted for creates;
	// After is omitted for deletes. Map keys are JSON-marshalled in sorted
	// order, which keeps the confirmation digest deterministic.
	Before map[string]any `json:"before,omitempty"`
	After  map[string]any `json:"after,omitempty"`
}

// Preview is the structured result a dry-run returns instead of mutating
// (TO RESOLVE 4). The transport adapter serialises it verbatim to the agent.
type Preview struct {
	// DryRun is always true on a Preview (it makes the shape self-describing
	// when embedded in a tool result alongside real results).
	DryRun bool `json:"dry_run"`
	// Operation is the tool/op name, e.g. "advance_deal".
	Operation string `json:"operation"`
	// Summary is a one-line human/agent-readable description of the effect.
	Summary string `json:"summary"`
	// Effects is the would-be diff.
	Effects []Effect `json:"effects"`
	// ConfirmationRequired is true for destructive/bulk ops: the agent must
	// echo ConfirmationToken on the real call.
	ConfirmationRequired bool `json:"confirmation_required"`
	// ConfirmationToken is the opaque handshake token, present only when
	// ConfirmationRequired. It is HMAC-bound to (workspace, operation,
	// effects) and expires; see Confirmer.
	ConfirmationToken string `json:"confirmation_token,omitempty"`
}

// ComputeEffectDigest returns a stable hex SHA-256 over (operation, effects).
// encoding/json marshals map keys in sorted order, so the digest is
// deterministic for a given logical effect. The confirmation token binds to
// this digest, so confirming an effect that no longer matches what was
// previewed (the underlying rows changed between dry-run and confirm) fails
// validation and forces a fresh dry-run.
func ComputeEffectDigest(operation string, effects []Effect) string {
	payload := struct {
		Operation string   `json:"operation"`
		Effects   []Effect `json:"effects"`
	}{Operation: operation, Effects: effects}
	body, _ := json.Marshal(payload)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// --- confirmation-token handshake (ADR-012 §6, TO RESOLVE 4) ---

// confirmTokenTTL bounds how long a dry-run's confirmation token stays
// valid. Short enough that a stale token can't authorise a much-later
// mutation, long enough for a human/parent-agent to inspect and confirm.
const confirmTokenTTL = 10 * time.Minute

// confirmTokenVersion prefixes every token so the format can evolve.
const confirmTokenVersion = "v1"

var (
	// ErrConfirmationRequired means a destructive/bulk op was called for real
	// (not dry-run) without a confirmation token. The caller must first
	// dry-run, then echo the returned token. Transport adapters map it to a
	// 409-class "confirmation required" tool error.
	ErrConfirmationRequired = errors.New("destructive operation requires a confirmation_token from a prior dry_run")
	// ErrConfirmationInvalid means the supplied token is malformed, expired,
	// for a different workspace/operation, or no longer matches the planned
	// effect. The caller must re-run the dry-run to obtain a fresh token.
	ErrConfirmationInvalid = errors.New("confirmation_token is invalid, expired, or does not match the planned effect")
)

// Confirmer issues and verifies confirmation tokens. A token is a stateless,
// HMAC-signed assertion that a specific effect was previewed for a specific
// workspace+operation and has not yet expired — so no server-side table is
// needed (the "new, thin" mechanism ADR-012 §6 calls for). The secret is the
// only thing that makes a token unforgeable from public inputs (the agent
// can compute the effect digest itself); source it from the same secrets
// store as other HMAC keys (ADR-007), never hardcode it.
type Confirmer struct {
	secret []byte
}

// NewConfirmer builds a Confirmer from a signing secret. A zero-length secret
// is rejected so a misconfigured deployment fails closed rather than issuing
// trivially-forgeable tokens.
func NewConfirmer(secret []byte) (*Confirmer, error) {
	if len(secret) == 0 {
		return nil, errors.New("confirmer: signing secret required")
	}
	return &Confirmer{secret: secret}, nil
}

// sign computes the HMAC over the canonical token payload.
func (c *Confirmer) sign(ws uuid.UUID, operation, digest string, expUnix int64) []byte {
	mac := hmac.New(sha256.New, c.secret)
	// NUL-delimited so no field boundary is ambiguous.
	fmt.Fprintf(mac, "%s\x00%s\x00%s\x00%s\x00%d",
		confirmTokenVersion, ws.String(), operation, digest, expUnix)
	return mac.Sum(nil)
}

// Issue mints a confirmation token binding (workspace, operation, digest) for
// confirmTokenTTL from now. now is passed in (not read from the clock) so the
// handshake is deterministic under test.
func (c *Confirmer) Issue(ws uuid.UUID, operation, digest string, now time.Time) string {
	exp := now.Add(confirmTokenTTL).Unix()
	sig := c.sign(ws, operation, digest, exp)
	return strings.Join([]string{
		confirmTokenVersion,
		strconv.FormatInt(exp, 10),
		base64.RawURLEncoding.EncodeToString(sig),
	}, ".")
}

// Verify checks a token against the expected (workspace, operation, digest)
// at time now. It returns nil only when the token is well-formed, unexpired,
// and its HMAC matches — using a constant-time comparison so a failed match
// leaks no timing signal. Any failure returns ErrConfirmationInvalid.
func (c *Confirmer) Verify(token string, ws uuid.UUID, operation, digest string, now time.Time) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != confirmTokenVersion {
		return ErrConfirmationInvalid
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return ErrConfirmationInvalid
	}
	if now.Unix() > exp {
		return ErrConfirmationInvalid
	}
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return ErrConfirmationInvalid
	}
	want := c.sign(ws, operation, digest, exp)
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrConfirmationInvalid
	}
	return nil
}

// --- write orchestration: the single shape every MCP write tool composes ---

// WriteOptions carries the cross-cutting safety controls a write tool accepts
// from its caller (ADR-012 §6). The zero value is a normal, non-idempotent,
// non-dry-run, unconfirmed write.
type WriteOptions struct {
	// IdempotencyKey, when non-empty, makes the op idempotent: a duplicate
	// call within the key's TTL replays the cached result. Supplied by the
	// caller or derived by the tool (e.g. hash of intent args).
	IdempotencyKey string
	// DryRun requests a Preview of the would-be effect with no mutation.
	DryRun bool
	// ConfirmationToken is echoed back from a prior dry-run to authorise a
	// destructive/bulk op's real execution.
	ConfirmationToken string
}

// WriteResult is the outcome of GuardedWrite.Run. Exactly one of Preview
// (dry-run) or (Status, Body) (real write / replay) is meaningful.
type WriteResult struct {
	// Preview is non-nil iff this was a dry-run; no mutation occurred.
	Preview *Preview
	// Status / Body are the canonical response of a real (or replayed) write.
	Status int
	Body   []byte
	// Replayed is true when Body came from the idempotency cache.
	Replayed bool
}

// GuardedWrite sequences the §6 controls around one capability write op. A
// write tool builds it once and calls Run, supplying three closures that
// isolate the only side-effecting work (DB I/O); GuardedWrite owns the
// security-relevant ordering so it cannot be gotten wrong per tool.
//
// Destructive ops (delete / bulk) additionally require a Confirmer and the
// confirmation handshake; non-destructive ops leave Confirmer nil.
type GuardedWrite struct {
	// Operation is the tool name; it scopes the confirmation token and labels
	// the Preview.
	Operation string
	// Destructive flags delete/bulk ops that require the confirmation
	// handshake. When true, Confirmer must be set.
	Destructive bool
	// Options are the per-call safety flags.
	Options WriteOptions
	// Confirmer signs/verifies confirmation tokens; required iff Destructive.
	Confirmer *Confirmer
	// Now supplies the current time for token issue/verify; defaults to
	// time.Now when nil. Injectable for deterministic tests.
	Now func() time.Time
}

func (g GuardedWrite) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

// Run executes the control sequence for one write, in this fixed order:
//
//  1. authorize  — scope→role gate (AuthorizeWrite). Read-only scope is
//     rejected here before any closure runs.
//  2. dry-run    — if Options.DryRun: build the Preview via plan(), attach a
//     confirmation token for destructive ops, and return. No mutation, no
//     idempotency write.
//  3. replay     — if an idempotency key is set: a cache hit returns the
//     stored result immediately (Replayed=true), before re-planning/applying.
//  4. confirm    — for destructive ops: re-plan and verify the supplied
//     confirmation token against the freshly-computed effect digest, so the
//     committed effect equals exactly what was previewed.
//  5. apply      — run the mutation. apply must, inside its own write tx,
//     emit fail-closed audit and (when keyed) store the idempotency result;
//     GuardedWrite does not reach into the tx.
//
// Closures:
//   - replay: looks up the idempotency cache; hit=false (and a nil error)
//     when no key is configured or no row exists. May be nil → never replays.
//   - plan:   computes the would-be Preview without mutating (a read tx). Its
//     Effects drive both the dry-run output and the confirmation digest.
//   - apply:  performs the mutation and returns the canonical (status, body).
func (g GuardedWrite) Run(
	p Principal,
	replay func() (status int, body []byte, hit bool, err error),
	plan func() (Preview, error),
	apply func() (status int, body []byte, err error),
) (WriteResult, error) {
	// 1. Scope→role authorization (mechanism-agnostic, §7).
	if err := AuthorizeWrite(p); err != nil {
		return WriteResult{}, err
	}

	if g.Destructive && g.Confirmer == nil {
		return WriteResult{}, errors.New("guarded write: destructive op requires a Confirmer")
	}

	// 2. Dry-run: preview only, never mutate.
	if g.Options.DryRun {
		preview, err := plan()
		if err != nil {
			return WriteResult{}, err
		}
		preview.DryRun = true
		preview.Operation = g.Operation
		if g.Destructive {
			digest := ComputeEffectDigest(g.Operation, preview.Effects)
			preview.ConfirmationRequired = true
			preview.ConfirmationToken = g.Confirmer.Issue(p.WorkspaceID, g.Operation, digest, g.now())
		}
		return WriteResult{Preview: &preview}, nil
	}

	// 3. Idempotent replay short-circuit.
	if g.Options.IdempotencyKey != "" && replay != nil {
		status, body, hit, err := replay()
		if err != nil {
			return WriteResult{}, err
		}
		if hit {
			return WriteResult{Status: status, Body: body, Replayed: true}, nil
		}
	}

	// 4. Confirmation handshake for destructive/bulk ops.
	if g.Destructive {
		if g.Options.ConfirmationToken == "" {
			return WriteResult{}, ErrConfirmationRequired
		}
		preview, err := plan()
		if err != nil {
			return WriteResult{}, err
		}
		digest := ComputeEffectDigest(g.Operation, preview.Effects)
		if err := g.Confirmer.Verify(g.Options.ConfirmationToken, p.WorkspaceID, g.Operation, digest, g.now()); err != nil {
			return WriteResult{}, err
		}
	}

	// 5. Apply the mutation (audit + idempotency persisted inside apply's tx).
	status, body, err := apply()
	if err != nil {
		return WriteResult{}, err
	}
	return WriteResult{Status: status, Body: body}, nil
}
