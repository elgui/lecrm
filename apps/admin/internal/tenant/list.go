package tenant

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TenantSummary is one row of the list/get output.
type TenantSummary struct {
	ID            uuid.UUID
	Slug          string
	RoleName      string
	AdminEmail    string
	CreatorEmail  string
	CreatedAt     time.Time
	Features      []string
}

// List prints all tenants in core.workspaces. Output is operator-facing
// per D12 (uses "Tenant" not "workspace"). One tenant per line, fields
// tab-separated for grep-friendliness.
func List(ctx context.Context, conn *pgx.Conn, stdout io.Writer) error {
	rows, err := conn.Query(ctx, `
		SELECT id, slug, role_name, admin_email, creator_email, created_at,
		       COALESCE(ARRAY(SELECT jsonb_array_elements_text(provisioning_features_applied)), '{}')
		  FROM core.workspaces
		 ORDER BY created_at ASC
	`)
	if err != nil {
		return New(ErrKindDBConnect, "query tenants: %v", err)
	}
	defer rows.Close()

	fmt.Fprintf(stdout, "%s\tSLUG\tROLE\tADMIN_EMAIL\tCREATOR_EMAIL\tCREATED_AT\tFEATURES\n", OperatorNoun+"_ID")
	count := 0
	for rows.Next() {
		var t TenantSummary
		if err := rows.Scan(&t.ID, &t.Slug, &t.RoleName, &t.AdminEmail, &t.CreatorEmail, &t.CreatedAt, &t.Features); err != nil {
			return New(ErrKindDBConnect, "scan row: %v", err)
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\t%v\n",
			t.ID, t.Slug, t.RoleName, t.AdminEmail, t.CreatorEmail,
			t.CreatedAt.UTC().Format(time.RFC3339), t.Features)
		count++
	}
	if err := rows.Err(); err != nil {
		return New(ErrKindDBConnect, "iterate tenants: %v", err)
	}
	fmt.Fprintf(stdout, "# %d %s(s)\n", count, OperatorNoun)
	return nil
}
