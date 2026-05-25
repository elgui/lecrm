package jobs

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Test-only exports. This file is only compiled during `go test`.

// CommandTag re-exports pgconn.CommandTag for external test packages.
type CommandTag = pgconn.CommandTag

// Row re-exports pgx.Row for external test packages.
type Row = pgx.Row

// WithSafeExecForTest exposes withSafeExec for external test packages.
func WithSafeExecForTest(
	ctx context.Context,
	db dbExecutor,
	workspaceID uuid.UUID,
	roleName string,
	fn func() error,
) error {
	return withSafeExec(ctx, db, workspaceID, roleName, fn)
}
