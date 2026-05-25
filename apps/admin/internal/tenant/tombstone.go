package tenant

import (
	"context"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
)

// TombstoneOptions is the parsed flag set for tenant tombstone.
type TombstoneOptions struct {
	Slug string
}

// TombstoneResult is what Tombstone returns on success.
type TombstoneResult struct {
	Slug string
}

// Tombstone soft-deletes a workspace by setting tombstoned_at = NOW().
// The workspace data is preserved but the slug becomes permanently
// unavailable for re-registration (subdomain takeover prevention).
func Tombstone(ctx context.Context, conn *pgx.Conn, opts TombstoneOptions, stdout io.Writer) (TombstoneResult, error) {
	if err := ValidateSlug(opts.Slug); err != nil {
		return TombstoneResult{}, err
	}

	var exists bool
	err := conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM core.workspaces WHERE slug = $1)`, opts.Slug).Scan(&exists)
	if err != nil {
		return TombstoneResult{}, New(ErrKindDBConnect, "lookup workspace: %v", err)
	}
	if !exists {
		return TombstoneResult{}, New(ErrKindSlugConflict,
			"%s %q not found", OperatorNoun, opts.Slug)
	}

	var alreadyTombstoned bool
	err = conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM core.workspaces WHERE slug = $1 AND tombstoned_at IS NOT NULL)`,
		opts.Slug).Scan(&alreadyTombstoned)
	if err != nil {
		return TombstoneResult{}, New(ErrKindDBConnect, "check tombstone status: %v", err)
	}
	if alreadyTombstoned {
		return TombstoneResult{}, New(ErrKindSlugTombstoned,
			"%s %q is already tombstoned", OperatorNoun, opts.Slug)
	}

	tag, err := conn.Exec(ctx,
		`UPDATE core.workspaces SET tombstoned_at = now(), updated_at = now() WHERE slug = $1 AND tombstoned_at IS NULL`,
		opts.Slug)
	if err != nil {
		return TombstoneResult{}, New(ErrKindDBProvision, "tombstone workspace: %v", err)
	}
	if tag.RowsAffected() == 0 {
		return TombstoneResult{}, New(ErrKindSlugTombstoned,
			"%s %q could not be tombstoned (race condition or already tombstoned)", OperatorNoun, opts.Slug)
	}

	// Audit log entry for the tombstone event.
	_, err = conn.Exec(ctx,
		`INSERT INTO core.audit_log (event, workspace_id, actor_type, payload)
		 SELECT 'workspace.tombstoned', id, 'system', jsonb_build_object('slug', $1)
		 FROM core.workspaces WHERE slug = $1`,
		opts.Slug)
	if err != nil {
		return TombstoneResult{}, New(ErrKindDBProvision, "audit tombstone: %v", err)
	}

	_, _ = fmt.Fprintf(stdout, "%s %q tombstoned. Slug is permanently unavailable.\n", OperatorNoun, opts.Slug)
	return TombstoneResult{Slug: opts.Slug}, nil
}
