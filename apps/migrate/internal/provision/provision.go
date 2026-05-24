// Package provision wraps the core.lecrm_provision_workspace_with_registry
// SECURITY DEFINER function (Story 8.1 / ADR-009 §2.1) which provisions a
// workspace role + schema + River queue + registry row + audit row
// atomically. The caller is responsible for opening a connection as
// lecrm_provisioner.
//
// Per D11 (Story 8.1) this package is the single Go entry point into the
// SECURITY DEFINER wrapper. apps/admin (lecrm-admin tenant create) and
// apps/migrate (provision-workspace) both call this same wrapper — there
// is no shared Go provisioning package; the wrapper IS the atomicity
// boundary. The bootstrap path used by apps/migrate passes empty
// admin_email / creator_email / template (no pipeline seed).
package provision

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Result is returned by Run after a successful provisioning call.
type Result struct {
	WorkspaceID uuid.UUID
	Slug        string
	RoleName    string
	IsNew       bool
}

// Run provisions a workspace identified by slug via the SECURITY DEFINER
// wrapper. If the slug is already in core.workspaces the function re-uses
// the existing UUID so the wrapper's ON CONFLICT (id) DO NOTHING path
// kicks in (silent idempotent re-run). Connection must have EXECUTE on
// core.lecrm_provision_workspace_with_registry (lecrm_provisioner satisfies
// this; a Postgres superuser also works for tests).
//
// This is the bootstrap path. The integrator-facing path (apps/admin)
// supplies real admin_email / creator_email / template values; here we
// pass empty strings so no pipeline-stages seed runs.
func Run(ctx context.Context, conn *pgx.Conn, slug string, logger *slog.Logger) (Result, error) {
	var id uuid.UUID
	var isNew bool

	err := conn.QueryRow(ctx,
		"SELECT id FROM core.workspaces WHERE slug = $1", slug).Scan(&id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// D10 (Story 8.1): UUIDv7 — time-ordered, index-friendly under load.
		id, err = uuid.NewV7()
		if err != nil {
			return Result{}, fmt.Errorf("provision: mint UUIDv7: %w", err)
		}
		isNew = true
	case err != nil:
		return Result{}, fmt.Errorf("provision: lookup workspace: %w", err)
	}

	logger.InfoContext(ctx, "calling lecrm_provision_workspace_with_registry",
		"slug", slug, "id", id, "is_new", isNew)

	var roleName string
	if err := conn.QueryRow(ctx,
		`SELECT core.lecrm_provision_workspace_with_registry($1, $2, $3, $4, $5)`,
		id, slug, "", "", "").Scan(&roleName); err != nil {
		return Result{}, fmt.Errorf("provision: provision_workspace_with_registry(%s): %w", id, err)
	}

	logger.InfoContext(ctx, "workspace provisioned",
		"slug", slug, "role", roleName, "is_new", isNew)
	return Result{WorkspaceID: id, Slug: slug, RoleName: roleName, IsNew: isNew}, nil
}
