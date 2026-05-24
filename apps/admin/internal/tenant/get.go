package tenant

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
)

// Get prints one tenant identified by slug. Returns ErrKindSlugConflict
// if the slug is not in the registry (re-using the slug-conflict kind
// since both paths surface "slug doesn't resolve to a known tenant").
func Get(ctx context.Context, conn *pgx.Conn, slug string, stdout io.Writer) error {
	if err := ValidateSlug(slug); err != nil {
		return err
	}
	var t TenantSummary
	err := conn.QueryRow(ctx, `
		SELECT id, slug, role_name, admin_email, creator_email, created_at,
		       COALESCE(ARRAY(SELECT jsonb_array_elements_text(provisioning_features_applied)), '{}')
		  FROM core.workspaces WHERE slug = $1
	`, slug).Scan(&t.ID, &t.Slug, &t.RoleName, &t.AdminEmail, &t.CreatorEmail, &t.CreatedAt, &t.Features)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return New(ErrKindSlugConflict, "%s %q not found", OperatorNoun, slug)
	case err != nil:
		return New(ErrKindDBConnect, "query tenant: %v", err)
	}
	fmt.Fprintf(stdout, "%s_ID:      %s\n", OperatorNoun, t.ID)
	fmt.Fprintf(stdout, "SLUG:           %s\n", t.Slug)
	fmt.Fprintf(stdout, "ROLE:           %s\n", t.RoleName)
	fmt.Fprintf(stdout, "ADMIN_EMAIL:    %s\n", t.AdminEmail)
	fmt.Fprintf(stdout, "CREATOR_EMAIL:  %s\n", t.CreatorEmail)
	fmt.Fprintf(stdout, "CREATED_AT:     %s\n", t.CreatedAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(stdout, "FEATURES:       %v\n", t.Features)
	return nil
}
