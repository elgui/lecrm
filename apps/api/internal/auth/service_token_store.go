package auth

// PgServiceTokenStore is the production sqlc-backed CandidateLoader
// + token CRUD store. It is intentionally a thin wrapper around
// sqlcgen.Queries — most code lives in the handler / verifier.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gbconsult/lecrm/apps/api/internal/sqlcgen"
)

// PgServiceTokenStore implements CandidateLoader against a pgx pool.
type PgServiceTokenStore struct {
	Pool *pgxpool.Pool
}

// LoadCandidates returns all active (non-expired) tokens for a workspace.
func (s *PgServiceTokenStore) LoadCandidates(ctx context.Context, workspaceID uuid.UUID) ([]TokenCandidate, error) {
	rows, err := sqlcgen.New(s.Pool).ListServiceTokenCandidatesForVerify(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load candidates: %w", err)
	}
	out := make([]TokenCandidate, 0, len(rows))
	for _, r := range rows {
		c := TokenCandidate{
			ID:        r.ID,
			Hash:      r.TokenHash,
			ActorType: r.ActorType,
			Scopes:    r.Scopes,
		}
		if r.ExpiresAt.Valid {
			exp := r.ExpiresAt.Time.Unix()
			c.ExpiresAt = &exp
		}
		out = append(out, c)
	}
	return out, nil
}

// TouchLastUsed bumps last_used_at on a successful verification.
func (s *PgServiceTokenStore) TouchLastUsed(ctx context.Context, tokenID uuid.UUID) error {
	return sqlcgen.New(s.Pool).TouchServiceTokenLastUsed(ctx, tokenID)
}

// CreateServiceTokenInput is the validated payload for minting a new
// service token via the handler.
type CreateServiceTokenInput struct {
	Name      string
	ActorType string
	Scopes    []string
	ExpiresAt *time.Time
}

// CreatedToken bundles the plaintext (returned ONCE) plus the
// persisted row metadata that the handler exposes back to the caller.
type CreatedToken struct {
	Plaintext  string
	ID         uuid.UUID
	Name       string
	ActorType  string
	Scopes     []string
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

var allowedActorTypes = map[string]struct{}{
	"human_api":        {},
	"mcp_agent":        {},
	"internal_service": {},
	"connector":        {},
}

// ValidateActorType returns nil when t is in the allowlist.
func ValidateActorType(t string) error {
	if _, ok := allowedActorTypes[t]; !ok {
		return errors.New("invalid actor_type")
	}
	return nil
}

// Create mints a new token and persists its argon2id hash. The
// returned plaintext is shown to the caller exactly once.
func (s *PgServiceTokenStore) Create(ctx context.Context, workspaceID uuid.UUID, workspaceSlug string, in CreateServiceTokenInput) (*CreatedToken, error) {
	if in.Name == "" {
		return nil, errors.New("name required")
	}
	if err := ValidateActorType(in.ActorType); err != nil {
		return nil, err
	}
	scopes := in.Scopes
	if len(scopes) == 0 {
		scopes = []string{"*"}
	}
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return nil, fmt.Errorf("marshal scopes: %w", err)
	}

	plaintext, hash, err := GenerateServiceToken(workspaceSlug)
	if err != nil {
		return nil, err
	}

	exp := pgtype.Timestamptz{}
	if in.ExpiresAt != nil {
		exp = pgtype.Timestamptz{Time: *in.ExpiresAt, Valid: true}
	}

	row, err := sqlcgen.New(s.Pool).CreateServiceToken(ctx, sqlcgen.CreateServiceTokenParams{
		WorkspaceID: workspaceID,
		Name:        in.Name,
		TokenHash:   hash,
		ActorType:   in.ActorType,
		Scopes:      scopesJSON,
		ExpiresAt:   exp,
	})
	if err != nil {
		return nil, fmt.Errorf("create service token: %w", err)
	}

	return &CreatedToken{
		Plaintext: plaintext,
		ID:        row.ID,
		Name:      row.Name,
		ActorType: row.ActorType,
		Scopes:    decodeScopes(row.Scopes),
		ExpiresAt: ptrTimeFromPg(row.ExpiresAt),
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

// List returns the persisted tokens for a workspace (no hashes).
type ListedToken struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	ActorType  string     `json:"actor_type"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (s *PgServiceTokenStore) List(ctx context.Context, workspaceID uuid.UUID) ([]ListedToken, error) {
	rows, err := sqlcgen.New(s.Pool).ListServiceTokensByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]ListedToken, 0, len(rows))
	for _, r := range rows {
		out = append(out, ListedToken{
			ID:         r.ID,
			Name:       r.Name,
			ActorType:  r.ActorType,
			Scopes:     decodeScopes(r.Scopes),
			ExpiresAt:  ptrTimeFromPg(r.ExpiresAt),
			LastUsedAt: ptrTimeFromPg(r.LastUsedAt),
			CreatedAt:  r.CreatedAt.Time,
		})
	}
	return out, nil
}

// ErrTokenNotFound signals a Delete that touched zero rows.
var ErrTokenNotFound = errors.New("service token: not found")

// Delete revokes a token. Returns ErrTokenNotFound when no row matched
// (which can be either a wrong id or a wrong workspace — both look
// the same to the caller so the response is 404 either way).
func (s *PgServiceTokenStore) Delete(ctx context.Context, workspaceID, tokenID uuid.UUID) error {
	n, err := sqlcgen.New(s.Pool).DeleteServiceToken(ctx, sqlcgen.DeleteServiceTokenParams{
		WorkspaceID: workspaceID,
		ID:          tokenID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func ptrTimeFromPg(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
