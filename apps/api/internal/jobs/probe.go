package jobs

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ProbeWorkspaceConnectivity is a no-op job that confirms the
// RunWorkspaceJob pattern end-to-end: it acquires a workspace-scoped
// connection, verifies the search_path, and runs SELECT 1. Use it as a
// health check after provisioning a new workspace.
func ProbeWorkspaceConnectivity(
	ctx context.Context,
	resolver WorkspaceResolver,
	creds CredentialResolver,
	workspaceID uuid.UUID,
) error {
	_, err := RunWorkspaceJob(ctx, resolver, creds, workspaceID,
		func(ctx context.Context, conn *pgx.Conn) (struct{}, error) {
			var one int
			if err := conn.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
				return struct{}{}, fmt.Errorf("probe SELECT 1: %w", err)
			}
			return struct{}{}, nil
		},
	)
	return err
}
