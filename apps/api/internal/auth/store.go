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
// workspace. New users auto-join as 'member' at v0. It is a thin wrapper
// over EnsureMemberWithRole and, like it, NEVER downgrades an existing
// higher role.
func (s *Store) EnsureMember(ctx context.Context, workspaceID, userID uuid.UUID) error {
	return s.EnsureMemberWithRole(ctx, workspaceID, userID, "member")
}

// roleRankCase is a SQL fragment that maps a role column/value to its
// position in the rbac total order (member < admin < owner < integrator).
// Unknown values rank 0 so they never win an upgrade comparison. It mirrors
// apps/api/internal/rbac/role.go; auth cannot import rbac (rbac imports auth),
// so the hierarchy is duplicated here as an inline CASE rather than shared.
const roleRankCase = `CASE %s WHEN 'member' THEN 1 WHEN 'admin' THEN 2 WHEN 'owner' THEN 3 WHEN 'integrator' THEN 4 ELSE 0 END`

// validRoles is the set of role strings EnsureMemberWithRole accepts. The
// workspace_members.role CHECK constraint is the ultimate guard, but
// validating in Go turns a typo into a clear error instead of a constraint
// violation buried in a transaction.
var validRoles = map[string]bool{"member": true, "admin": true, "owner": true, "integrator": true}

// EnsureMemberWithRole ensures the user is a member of the workspace at AT
// LEAST the given role. On an existing membership it UPGRADES to role only
// when role outranks the stored role — it never downgrades (ADR-009: a
// returning owner who logs in as a plain member keeps owner; an integrator
// grant elevates a member to integrator). A pending invite (joined_at NULL)
// is materialized to "joined" on first login via the COALESCE.
func (s *Store) EnsureMemberWithRole(ctx context.Context, workspaceID, userID uuid.UUID, role string) error {
	if !validRoles[role] {
		return fmt.Errorf("ensure membership: unknown role %q", role)
	}
	q := fmt.Sprintf(`
		INSERT INTO core.workspace_members (workspace_id, user_id, role, joined_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET
			role = CASE
				WHEN (%s) > (%s) THEN EXCLUDED.role
				ELSE core.workspace_members.role
			END,
			joined_at = COALESCE(core.workspace_members.joined_at, now())
	`,
		fmt.Sprintf(roleRankCase, "EXCLUDED.role"),
		fmt.Sprintf(roleRankCase, "core.workspace_members.role"),
	)
	if _, err := s.pool.Exec(ctx, q, workspaceID, userID, role); err != nil {
		return fmt.Errorf("ensure membership: %w", err)
	}
	return nil
}

// IntegratorGrantExists reports whether core.integrator_grants holds a
// pending grant for (workspaceID, lower(email)). It is the login-time
// elevation check: a match means the freshly-authenticated human should be
// materialized as an 'integrator' member rather than a plain 'member'. An
// empty email never matches (an integrator grant is always email-keyed).
func (s *Store) IntegratorGrantExists(ctx context.Context, workspaceID uuid.UUID, email string) (bool, error) {
	if email == "" {
		return false, nil
	}
	const q = `
		SELECT EXISTS (
			SELECT 1 FROM core.integrator_grants
			WHERE workspace_id = $1 AND lower(email) = lower($2)
		)
	`
	var exists bool
	if err := s.pool.QueryRow(ctx, q, workspaceID, email).Scan(&exists); err != nil {
		return false, fmt.Errorf("integrator grant exists: %w", err)
	}
	return exists, nil
}

// AccessibleWorkspace is one workspace a user can switch into, with the role
// they hold there. Role is the membership role when a workspace_members row
// exists, else "integrator" (the access comes from a pending grant).
type AccessibleWorkspace struct {
	Slug string
	Role string
}

// ListAccessibleWorkspaces returns the UNION of the workspaces the user is a
// member of (by user_id) and the workspaces they hold an integrator grant for
// (by lower(email)), restricted to live workspaces (tombstoned_at IS NULL).
//
// It is the data behind GET /auth/workspaces: a freshly-provisioned tenant
// the integrator has never logged into still appears, because the grant row
// exists before any workspace_members row does. The query reads only the
// caller's own rows ($1/$2) — there is no cross-user or slug-enumeration path.
func (s *Store) ListAccessibleWorkspaces(ctx context.Context, userID uuid.UUID, email string) ([]AccessibleWorkspace, error) {
	const q = `
		SELECT w.slug, COALESCE(m.role, 'integrator') AS role
		FROM core.workspaces w
		LEFT JOIN core.workspace_members m
			ON m.workspace_id = w.id AND m.user_id = $1
		LEFT JOIN core.integrator_grants g
			ON g.workspace_id = w.id AND lower(g.email) = lower($2)
		WHERE w.tombstoned_at IS NULL
			AND (m.user_id IS NOT NULL OR g.workspace_id IS NOT NULL)
		ORDER BY w.slug ASC
	`
	rows, err := s.pool.Query(ctx, q, userID, email)
	if err != nil {
		return nil, fmt.Errorf("list accessible workspaces: %w", err)
	}
	defer rows.Close()

	var out []AccessibleWorkspace
	for rows.Next() {
		var a AccessibleWorkspace
		if err := rows.Scan(&a.Slug, &a.Role); err != nil {
			return nil, fmt.Errorf("scan accessible workspace: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Pool returns the underlying connection pool for operations that need
// direct DB access (e.g. revocation writes). Prefer typed Store methods
// for new queries.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// ErrWorkspaceNotFound signals an unknown workspace slug.
var ErrWorkspaceNotFound = errors.New("workspace not found")
