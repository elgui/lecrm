package workspace

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gbconsult/lecrm/apps/api/internal/sqlcgen"
)

// PoolResolver is the live Resolver: it goes to core.workspaces via
// the sqlc-generated GetWorkspaceBySlug query. At v0 every middleware
// pass hits Postgres; a 60-second in-process LRU is a Sprint-3 cleanup
// (tracked in ADR-009 §6.4).
type PoolResolver struct {
	Pool *pgxpool.Pool
}

// WorkspaceBySlugFull satisfies the Resolver interface.
func (r *PoolResolver) WorkspaceBySlugFull(ctx context.Context, slug string) (uuid.UUID, string, error) {
	if slug == "" {
		return uuid.Nil, "", errors.New("slug is required")
	}
	q := sqlcgen.New(r.Pool)
	row, err := q.GetWorkspaceBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", ErrUnknownWorkspace
	}
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("get workspace by slug: %w", err)
	}
	return row.ID, row.RoleName, nil
}
