package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store reads and writes the identity + workspace-membership tables
// from packages/db/migrations/0002_identity.sql.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// UpsertUser inserts or updates a core.users row keyed on (issuer,
// subject). Returns the row's UUID. Email/display_name are best-effort:
// they are updated on every login (so an IdP-side display-name change
// flows through) but the (issuer, subject) tuple is immutable.
func (s *Store) UpsertUser(ctx context.Context, issuer, subject, email, displayName string) (uuid.UUID, error) {
	if issuer == "" || subject == "" {
		return uuid.Nil, errors.New("issuer and subject are required")
	}
	const q = `
		INSERT INTO core.users (issuer, subject, email, display_name, last_login_at)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), now())
		ON CONFLICT (issuer, subject) DO UPDATE SET
			email         = COALESCE(NULLIF(EXCLUDED.email, ''), core.users.email),
			display_name  = COALESCE(NULLIF(EXCLUDED.display_name, ''), core.users.display_name),
			last_login_at = now(),
			updated_at    = now()
		RETURNING id
	`
	var id uuid.UUID
	if err := s.pool.QueryRow(ctx, q, issuer, subject, email, displayName).Scan(&id); err != nil {
		return uuid.Nil, fmt.Errorf("upsert user: %w", err)
	}
	return id, nil
}

// WorkspaceBySlug resolves the workspace UUID for an inbound subdomain.
// Returns (uuid.Nil, ErrWorkspaceNotFound) when no workspace matches —
// callers should rate-limit and treat as a 404 (NOT 401, to avoid an
// enumeration oracle on the subdomain namespace).
func (s *Store) WorkspaceBySlug(ctx context.Context, slug string) (uuid.UUID, error) {
	if slug == "" {
		return uuid.Nil, errors.New("slug is required")
	}
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT id FROM core.workspaces WHERE slug = $1`, slug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrWorkspaceNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("workspace lookup: %w", err)
	}
	return id, nil
}

// GetUserProfile returns the user's email and display name for /auth/me.
// Both are best-effort: a missing column is returned as an empty string,
// and an unknown user yields ("", "", nil) so callers can fail open.
func (s *Store) GetUserProfile(ctx context.Context, userID uuid.UUID) (email, displayName string, err error) {
	const q = `SELECT COALESCE(email, ''), COALESCE(display_name, '') FROM core.users WHERE id = $1`
	err = s.pool.QueryRow(ctx, q, userID).Scan(&email, &displayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("get user profile: %w", err)
	}
	return email, displayName, nil
}

// EnsureMember ensures the user has at least 'member' role in the
// workspace. New users auto-join as 'member' at v0; RBAC promotion is
// Sprint 8 work. The function does NOT downgrade roles.
func (s *Store) EnsureMember(ctx context.Context, workspaceID, userID uuid.UUID) error {
	const q = `
		INSERT INTO core.workspace_members (workspace_id, user_id, role, joined_at)
		VALUES ($1, $2, 'member', now())
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET
			joined_at = COALESCE(core.workspace_members.joined_at, now())
	`
	_, err := s.pool.Exec(ctx, q, workspaceID, userID)
	if err != nil {
		return fmt.Errorf("ensure membership: %w", err)
	}
	return nil
}

// Pool returns the underlying connection pool for operations that need
// direct DB access (e.g. revocation writes). Prefer typed Store methods
// for new queries.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// ErrWorkspaceNotFound signals an unknown workspace slug.
var ErrWorkspaceNotFound = errors.New("workspace not found")
