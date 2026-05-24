// Package jobs implements the tenant-scoped background job pattern for
// leCRM. Per ADR-009 §8.3, every River worker must acquire a
// workspace-scoped Postgres connection BEFORE any data operation. This
// package enforces that invariant via RunWorkspaceJob, which opens the
// connection, verifies the search_path, and then delegates to the
// caller's job function.
package jobs

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// WorkspaceResolver looks up a workspace's Postgres role name from the
// control-plane database.
type WorkspaceResolver interface {
	WorkspaceRoleName(ctx context.Context, id uuid.UUID) (roleName string, err error)
}

// CredentialResolver returns a Postgres DSN for a given workspace role.
// At v0 this wraps the SOPS-decrypted secret manifest lookup; in tests
// it is stubbed.
type CredentialResolver interface {
	DSNForRole(ctx context.Context, roleName string) (dsn string, err error)
}

// RunWorkspaceJob[T] runs fn inside a workspace-scoped Postgres
// connection. Steps:
//  1. Resolves the workspace's Postgres role via WorkspaceResolver.
//  2. Obtains a DSN for that role via CredentialResolver.
//  3. Opens a pgx.Conn authenticated as the workspace role.
//  4. Verifies current_setting('search_path') contains the workspace
//     schema (defense-in-depth per ADR-009 §8.3).
//  5. Calls fn(ctx, conn) and returns its result.
//
// The connection is closed before RunWorkspaceJob returns regardless of
// whether fn succeeds.
func RunWorkspaceJob[T any](
	ctx context.Context,
	resolver WorkspaceResolver,
	creds CredentialResolver,
	workspaceID uuid.UUID,
	fn func(ctx context.Context, conn *pgx.Conn) (T, error),
) (T, error) {
	var zero T

	roleName, err := resolver.WorkspaceRoleName(ctx, workspaceID)
	if err != nil {
		return zero, fmt.Errorf("jobs: resolve workspace role for %s: %w", workspaceID, err)
	}

	dsn, err := creds.DSNForRole(ctx, roleName)
	if err != nil {
		return zero, fmt.Errorf("jobs: resolve credentials for role %q: %w", roleName, err)
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return zero, fmt.Errorf("jobs: connect as workspace role %q: %w", roleName, err)
	}
	defer func() { _ = conn.Close(ctx) }()

	// Defense-in-depth: verify the connection is scoped to the correct
	// workspace schema before any data operation (ADR-009 §8.3).
	// The role's search_path is set at provisioning time to
	// "<role_name>, public" via ALTER ROLE.
	var searchPath string
	if err := conn.QueryRow(ctx, "SHOW search_path").Scan(&searchPath); err != nil {
		return zero, fmt.Errorf("jobs: verify search_path: %w", err)
	}
	if !strings.Contains(searchPath, roleName) {
		return zero, fmt.Errorf(
			"jobs: search_path %q does not contain workspace schema %q; connection mis-scoped",
			searchPath, roleName)
	}

	return fn(ctx, conn)
}
