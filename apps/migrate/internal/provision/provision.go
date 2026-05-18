// Package provision wraps the core.lecrm_provision_workspace SECURITY
// DEFINER function (ADR-009 §2.1) with the idempotent upsert into
// core.workspaces. The caller is responsible for opening a connection as
// lecrm_provisioner.
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

// Run provisions a workspace identified by slug. If the slug is already
// in core.workspaces the function re-invokes the idempotent SQL function
// and updates the stored role_name. Connection must have EXECUTE on
// core.lecrm_provision_workspace and write access to core.workspaces
// (lecrm_provisioner satisfies both; a Postgres superuser also works
// for tests).
func Run(ctx context.Context, conn *pgx.Conn, slug string, logger *slog.Logger) (Result, error) {
	var id uuid.UUID
	var isNew bool

	err := conn.QueryRow(ctx,
		"SELECT id FROM core.workspaces WHERE slug = $1", slug).Scan(&id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		id = uuid.New()
		isNew = true
	case err != nil:
		return Result{}, fmt.Errorf("provision: lookup workspace: %w", err)
	}

	logger.InfoContext(ctx, "calling lecrm_provision_workspace",
		"slug", slug, "id", id, "is_new", isNew)

	var roleName string
	if err := conn.QueryRow(ctx,
		"SELECT core.lecrm_provision_workspace($1)", id).Scan(&roleName); err != nil {
		return Result{}, fmt.Errorf("provision: provision_workspace(%s): %w", id, err)
	}

	_, err = conn.Exec(ctx, `
		INSERT INTO core.workspaces (id, slug, role_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (slug) DO UPDATE SET
			role_name  = EXCLUDED.role_name,
			updated_at = now()
	`, id, slug, roleName)
	if err != nil {
		return Result{}, fmt.Errorf("provision: upsert workspace: %w", err)
	}

	logger.InfoContext(ctx, "workspace provisioned",
		"slug", slug, "role", roleName, "is_new", isNew)
	return Result{WorkspaceID: id, Slug: slug, RoleName: roleName, IsNew: isNew}, nil
}
