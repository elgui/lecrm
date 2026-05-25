// Package jobs implements the tenant-scoped background job pattern for
// leCRM. Per ADR-009 §8.3, every River worker must acquire a
// workspace-scoped Postgres connection BEFORE any data operation. This
// package enforces that invariant via RunWorkspaceJob, which opens the
// connection, acquires an advisory lock, verifies the search_path, and
// then delegates to the caller's job function.
//
// Advisory locks serialize concurrent jobs for the same workspace,
// preventing search_path races when multiple workers target the same
// tenant. Panic recovery ensures the advisory lock is always released
// and the connection is always closed, even if the job function panics.
package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// dbExecutor abstracts the Exec/QueryRow methods shared by *pgx.Conn
// and *pgxpool.Conn, enabling advisory lock logic to be tested without
// a real database connection.
type dbExecutor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

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

// acquireAdvisoryLock blocks until the workspace-scoped advisory lock
// is held. Uses hashtext(workspace_id) as the lock key, which provides
// a deterministic int4 for any UUID. The lock is database-wide: two
// connections in the same database contending for the same key will
// serialize.
func acquireAdvisoryLock(ctx context.Context, db dbExecutor, workspaceID uuid.UUID) error {
	_, err := db.Exec(ctx, "SELECT pg_advisory_lock(hashtext($1))", workspaceID.String())
	if err != nil {
		return fmt.Errorf("acquire advisory lock for workspace %s: %w", workspaceID, err)
	}
	return nil
}

// releaseAdvisoryLock releases the workspace-scoped advisory lock. For
// fresh connections (RunWorkspaceJob), this is belt-and-suspenders since
// conn.Close releases session-level locks. For pooled connections,
// explicit release is mandatory to prevent blocking other jobs.
func releaseAdvisoryLock(ctx context.Context, db dbExecutor, workspaceID uuid.UUID) {
	if _, err := db.Exec(ctx, "SELECT pg_advisory_unlock(hashtext($1))", workspaceID.String()); err != nil {
		slog.WarnContext(ctx, "jobs: advisory unlock failed; connection close will release",
			slog.String("workspace_id", workspaceID.String()),
			slog.String("error", err.Error()),
		)
	}
}

// verifySearchPath checks that the connection's search_path contains
// the expected workspace schema.
func verifySearchPath(ctx context.Context, db dbExecutor, expectedSchema string) error {
	var searchPath string
	if err := db.QueryRow(ctx, "SHOW search_path").Scan(&searchPath); err != nil {
		return fmt.Errorf("verify search_path: %w", err)
	}
	if !strings.Contains(searchPath, expectedSchema) {
		return fmt.Errorf(
			"search_path %q does not contain workspace schema %q; connection mis-scoped",
			searchPath, expectedSchema)
	}
	return nil
}

// withSafeExec wraps a workspace job with advisory lock acquisition,
// search_path verification, and panic recovery. The advisory lock
// serializes concurrent jobs for the same workspace. The defer chain
// ensures lock release even if fn panics.
//
// Defer execution order (LIFO):
//  1. panic recovery (last registered → first to run) — catches panic,
//     converts to error
//  2. releaseAdvisoryLock — runs normally after panic is recovered
//
// This ordering guarantees the lock is always released.
func withSafeExec(
	ctx context.Context,
	db dbExecutor,
	workspaceID uuid.UUID,
	roleName string,
	fn func() error,
) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("jobs: panic in workspace job for %s: %v", workspaceID, r)
			slog.ErrorContext(ctx, "jobs: workspace job panicked",
				slog.String("workspace_id", workspaceID.String()),
				slog.Any("panic", r),
			)
		}
	}()

	if lockErr := acquireAdvisoryLock(ctx, db, workspaceID); lockErr != nil {
		return lockErr
	}
	defer releaseAdvisoryLock(ctx, db, workspaceID)

	if spErr := verifySearchPath(ctx, db, roleName); spErr != nil {
		return spErr
	}

	if fnErr := fn(); fnErr != nil {
		return fnErr
	}

	// Post-job defense-in-depth: detect search_path drift.
	if driftErr := verifySearchPath(ctx, db, roleName); driftErr != nil {
		slog.WarnContext(ctx, "jobs: search_path drifted during job execution",
			slog.String("workspace_id", workspaceID.String()),
			slog.String("error", driftErr.Error()),
		)
	}

	return nil
}

// RunWorkspaceJob[T] runs fn inside a workspace-scoped Postgres
// connection with advisory lock protection. Steps:
//  1. Resolves the workspace's Postgres role via WorkspaceResolver.
//  2. Obtains a DSN for that role via CredentialResolver.
//  3. Opens a pgx.Conn authenticated as the workspace role.
//  4. Acquires a workspace-scoped advisory lock (serializes concurrent
//     jobs for the same tenant).
//  5. Verifies current_setting('search_path') contains the workspace
//     schema (defense-in-depth per ADR-009 §8.3).
//  6. Calls fn(ctx, conn) and returns its result.
//  7. On completion (success, error, or panic): releases advisory lock,
//     verifies search_path hasn't drifted, closes connection.
//
// The connection is closed before RunWorkspaceJob returns regardless of
// whether fn succeeds. If fn panics, the panic is recovered and
// returned as an error.
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

	var result T
	if execErr := withSafeExec(ctx, conn, workspaceID, roleName, func() error {
		var fnErr error
		result, fnErr = fn(ctx, conn)
		return fnErr
	}); execErr != nil {
		return zero, execErr
	}
	return result, nil
}
