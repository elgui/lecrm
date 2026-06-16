// Package seqruntime is the wiring layer that runs the sequences engine inside
// lecrm-api: it acquires workspace-scoped transactions for the workers and runs
// one River client per workspace (foundation + Gmail-reply workers + the daily
// watch-renew). It exists because the engine packages (sequences, gmailreply)
// are deliberately seam-only — this package injects the concrete production
// collaborators and owns the client lifecycle.
package seqruntime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gbconsult/lecrm/apps/api/internal/sequences"
)

// WorkspaceSchema returns the per-workspace data schema name (workspace_<32hex>),
// matching core.lecrm_provision_workspace (the role/schema name) and
// core.workspaces.role_name.
func WorkspaceSchema(workspaceID uuid.UUID) string {
	return "workspace_" + strings.ReplaceAll(workspaceID.String(), "-", "")
}

// SearchPathAcquirer implements sequences.WorkspaceTxAcquirer on the shared
// lecrm_api pool via `SET LOCAL search_path` — the same tenant-scoping model as
// capability.ReadTx/WriteTx (apps/api/capability/capability.go:173), NOT the
// role-per-workspace db.TenantPool (which would need the deferred role-password
// capture, tasket 1023). This is sound here because:
//   - lecrm_api holds full DML on every workspace schema (core.lecrm_grant_app_role,
//     0017), so unqualified table names resolve under the pinned search_path.
//   - lecrm_api is a privileged service role, exempt from the session_user guard
//     in core.lecrm_emit_audit (0026), so sequences.Transition can emit audit for
//     any workspace.
//
// SET LOCAL reverts on commit/rollback, leaving no search_path leakage across
// pooled connections.
type SearchPathAcquirer struct {
	Pool *pgxpool.Pool
}

// AcquireTx opens a read-write tx on the shared pool and pins search_path to the
// workspace schema. The returned release is idempotent and must be deferred: it
// rolls back (a no-op once the worker has committed) and returns the connection.
func (a *SearchPathAcquirer) AcquireTx(
	ctx context.Context,
	workspaceID uuid.UUID,
) (context.Context, pgx.Tx, func(), error) {
	tx, err := a.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ctx, nil, nil, fmt.Errorf("seqruntime: begin tx: %w", err)
	}
	schema := WorkspaceSchema(workspaceID)
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+pgx.Identifier{schema}.Sanitize()); err != nil {
		_ = tx.Rollback(ctx)
		return ctx, nil, nil, fmt.Errorf("seqruntime: set search_path %s: %w", schema, err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() { _ = tx.Rollback(ctx) })
	}
	return ctx, tx, release, nil
}

// Compile-time proof SearchPathAcquirer satisfies the worker entry contract.
var _ sequences.WorkspaceTxAcquirer = (*SearchPathAcquirer)(nil)
