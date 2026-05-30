package tenant

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// GrantExecer is the subset of pgx used to write an integrator grant. Both
// *pgx.Conn and pgx.Tx satisfy it, so the same helper serves the
// non-transactional provision paths (createFresh / createUpsert via
// callWrapper) and the transactional one (createForceRecreate), as well as
// the standalone `lecrm-admin integrator grant` CLI.
type GrantExecer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// InsertIntegratorGrant records a pending integrator grant for (workspaceID,
// email). The conflict target matches the case-insensitive unique index
// integrator_grants_workspace_email_uk created in migration 0018, so a
// repeat grant is a no-op rather than a constraint violation — which keeps
// re-provisioning (--upsert) idempotent.
//
// grantedBy is informational (the operator who created the grant, '' = the
// provisioning system). email is stored verbatim; uniqueness and lookup are
// case-insensitive via lower(email).
func InsertIntegratorGrant(ctx context.Context, q GrantExecer, workspaceID uuid.UUID, email, grantedBy string) error {
	_, err := q.Exec(ctx,
		`INSERT INTO core.integrator_grants (workspace_id, email, granted_by)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (workspace_id, lower(email)) DO NOTHING`,
		workspaceID, email, grantedBy)
	if err != nil {
		return New(ErrKindDBProvision, "insert integrator grant for %q: %v", email, err)
	}
	return nil
}
