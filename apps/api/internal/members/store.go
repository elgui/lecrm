// Package members implements the workspace member-management endpoints
// (owner-only) and the current-user self-service endpoint, plus the
// Postgres-backed role lookup that the rbac layer depends on.
//
// All reads and writes target the global identity tables from
// 0002_identity.sql (core.users, core.workspace_members). Membership is
// keyed on (workspace_id, user_id); there is no surrogate membership id, so
// the REST :id path parameter is the user_id.
package members

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gbconsult/lecrm/apps/api/internal/rbac"
)

// ErrMemberNotFound is returned by UpdateRole / RemoveMember when no
// membership row matches (workspace_id, user_id).
var ErrMemberNotFound = errors.New("members: member not found")

// Member is one workspace membership joined with its user's identity.
// JoinedAt is nil for a pending invite (the invitee has not yet completed
// an OIDC login that calls auth.Store.EnsureMember).
type Member struct {
	UserID      uuid.UUID  `json:"user_id"`
	Email       *string    `json:"email"`
	DisplayName *string    `json:"display_name"`
	Role        string     `json:"role"`
	InvitedAt   time.Time  `json:"invited_at"`
	JoinedAt    *time.Time `json:"joined_at"`
	Pending     bool       `json:"pending"`
}

// Store is the persistence seam for member management. The production
// binding is *PgMemberStore; handler unit tests inject an in-memory stub.
type Store interface {
	ListMembers(ctx context.Context, workspaceID uuid.UUID) ([]Member, error)
	Invite(ctx context.Context, workspaceID uuid.UUID, email string, role rbac.Role) (Member, error)
	UpdateRole(ctx context.Context, workspaceID, userID uuid.UUID, role rbac.Role) error
	RemoveMember(ctx context.Context, workspaceID, userID uuid.UUID) error
}

// PgMemberStore reads and writes core.workspace_members / core.users. It
// also satisfies rbac.RoleLookup so the same instance backs both member
// management and the authorization middleware.
type PgMemberStore struct {
	Pool *pgxpool.Pool
}

var _ Store = (*PgMemberStore)(nil)
var _ rbac.RoleLookup = (*PgMemberStore)(nil)

// LookupRole resolves a user's role within a workspace for the rbac layer.
func (s *PgMemberStore) LookupRole(ctx context.Context, workspaceID, userID uuid.UUID) (rbac.Role, bool, error) {
	var roleStr string
	err := s.Pool.QueryRow(ctx,
		`SELECT role FROM core.workspace_members WHERE workspace_id = $1 AND user_id = $2`,
		workspaceID, userID,
	).Scan(&roleStr)
	if errors.Is(err, pgx.ErrNoRows) {
		return rbac.RoleNone, false, nil
	}
	if err != nil {
		return rbac.RoleNone, false, fmt.Errorf("lookup role: %w", err)
	}
	role, ok := rbac.ParseRole(roleStr)
	if !ok {
		// A row exists but its role is unrecognized — fail closed.
		return rbac.RoleNone, false, fmt.Errorf("lookup role: unknown role %q", roleStr)
	}
	return role, true, nil
}

// ListMembers returns all members of the workspace, pending invites
// included, ordered by invitation time.
func (s *PgMemberStore) ListMembers(ctx context.Context, workspaceID uuid.UUID) ([]Member, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT m.user_id, u.email, u.display_name, m.role, m.invited_at, m.joined_at
		FROM core.workspace_members m
		JOIN core.users u ON u.id = m.user_id
		WHERE m.workspace_id = $1
		ORDER BY m.invited_at ASC, m.user_id ASC
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	var out []Member
	for rows.Next() {
		var (
			m        Member
			email    pgtype.Text
			display  pgtype.Text
			joinedAt pgtype.Timestamptz
		)
		if err := rows.Scan(&m.UserID, &email, &display, &m.Role, &m.InvitedAt, &joinedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		if email.Valid {
			m.Email = &email.String
		}
		if display.Valid {
			m.DisplayName = &display.String
		}
		if joinedAt.Valid {
			t := joinedAt.Time
			m.JoinedAt = &t
		}
		m.Pending = m.JoinedAt == nil
		out = append(out, m)
	}
	return out, rows.Err()
}

// inviteIssuer is the synthetic OIDC issuer used for placeholder user rows
// created by an email invite before the invitee has logged in. The real
// OIDC identity (a different issuer/subject) supersedes it on first login;
// at v0 the invite is explicitly a placeholder (no email is actually sent).
const inviteIssuer = "lecrm-invite"

// Invite upserts a pending membership for an email. If a user already
// exists with that email it is reused; otherwise a placeholder user row is
// created. The membership row is left with joined_at NULL (pending) until
// the invitee logs in. Re-inviting an existing member updates their role.
func (s *PgMemberStore) Invite(ctx context.Context, workspaceID uuid.UUID, email string, role rbac.Role) (Member, error) {
	var m Member
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var userID uuid.UUID
		// Reuse an existing identity if one matches the email.
		err := tx.QueryRow(ctx,
			`SELECT id FROM core.users WHERE lower(email) = lower($1) ORDER BY created_at ASC LIMIT 1`,
			email,
		).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			// Create a placeholder identity keyed on the synthetic issuer.
			err = tx.QueryRow(ctx, `
				INSERT INTO core.users (issuer, subject, email)
				VALUES ($1, lower($2), $2)
				ON CONFLICT (issuer, subject) DO UPDATE SET email = EXCLUDED.email, updated_at = now()
				RETURNING id
			`, inviteIssuer, email).Scan(&userID)
		}
		if err != nil {
			return fmt.Errorf("resolve invitee: %w", err)
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO core.workspace_members (workspace_id, user_id, role, invited_at)
			VALUES ($1, $2, $3, now())
			ON CONFLICT (workspace_id, user_id) DO UPDATE SET role = EXCLUDED.role
		`, workspaceID, userID, role.String())
		if err != nil {
			return fmt.Errorf("upsert membership: %w", err)
		}

		return tx.QueryRow(ctx, `
			SELECT m.user_id, u.email, u.display_name, m.role, m.invited_at, m.joined_at
			FROM core.workspace_members m
			JOIN core.users u ON u.id = m.user_id
			WHERE m.workspace_id = $1 AND m.user_id = $2
		`, workspaceID, userID).Scan(
			&m.UserID, scanTextPtr(&m.Email), scanTextPtr(&m.DisplayName),
			&m.Role, &m.InvitedAt, scanTimePtr(&m.JoinedAt),
		)
	})
	if err != nil {
		return Member{}, err
	}
	m.Pending = m.JoinedAt == nil
	return m, nil
}

// UpdateRole changes a member's role. Returns ErrMemberNotFound when no
// matching membership exists.
func (s *PgMemberStore) UpdateRole(ctx context.Context, workspaceID, userID uuid.UUID, role rbac.Role) error {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE core.workspace_members SET role = $3 WHERE workspace_id = $1 AND user_id = $2`,
		workspaceID, userID, role.String(),
	)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMemberNotFound
	}
	return nil
}

// RemoveMember deletes a membership. Returns ErrMemberNotFound when no
// matching membership exists.
func (s *PgMemberStore) RemoveMember(ctx context.Context, workspaceID, userID uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx,
		`DELETE FROM core.workspace_members WHERE workspace_id = $1 AND user_id = $2`,
		workspaceID, userID,
	)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMemberNotFound
	}
	return nil
}

// scanTextPtr / scanTimePtr adapt nullable columns into *string / *time.Time
// destinations via a pgtype intermediary inside a single Scan call.
func scanTextPtr(dst **string) *textScanner    { return &textScanner{dst: dst} }
func scanTimePtr(dst **time.Time) *timeScanner { return &timeScanner{dst: dst} }

type textScanner struct{ dst **string }

func (t *textScanner) Scan(src any) error {
	var v pgtype.Text
	if err := v.Scan(src); err != nil {
		return err
	}
	if v.Valid {
		s := v.String
		*t.dst = &s
	} else {
		*t.dst = nil
	}
	return nil
}

type timeScanner struct{ dst **time.Time }

func (t *timeScanner) Scan(src any) error {
	var v pgtype.Timestamptz
	if err := v.Scan(src); err != nil {
		return err
	}
	if v.Valid {
		tm := v.Time
		*t.dst = &tm
	} else {
		*t.dst = nil
	}
	return nil
}
