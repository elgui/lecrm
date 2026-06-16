package sequences

import (
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// defaultQueueMaxWorkers is the per-client parallelism for the sequences
// queue when the caller does not specify one. Modest by design: a workspace's
// send throughput is governed by volume caps and throttles (tasket d8f9), not
// by raw worker count.
const defaultQueueMaxWorkers = 10

// RiverSchema returns the per-workspace river schema name, river_<32hex>. It
// MUST match core.lecrm_provision_workspace (packages/db/migrations/0001_init.sql,
// step 5), which creates the schema as:
//
//	'river_' || lower(replace(p_workspace_id::text, '-', ''))
//
// i.e. the literal prefix "river_" followed by the workspace UUID with its
// hyphens stripped, lowercased. ADR-009 §8.3 names it river_<workspace_base36>;
// the provisioner in fact uses lowercase hex, and this function mirrors the SQL
// exactly. The river client targets this schema via river.Config.Schema; a
// mismatch would point the client at the wrong (or a non-existent) river_job
// table, so the formula is pinned by a unit test against an independent
// recomputation.
func RiverSchema(workspaceID uuid.UUID) string {
	return "river_" + strings.ReplaceAll(workspaceID.String(), "-", "")
}

// WorkspaceRiverConfig builds the river.Config for one workspace: its
// per-tenant schema (RiverSchema), the registered workers, and a single
// default queue. queueMaxWorkers ≤ 0 falls back to defaultQueueMaxWorkers.
//
// This is split out from NewWorkspaceClient so the schema/queue wiring is
// unit-testable without a database connection.
func WorkspaceRiverConfig(workspaceID uuid.UUID, workers *river.Workers, queueMaxWorkers int) *river.Config {
	if queueMaxWorkers <= 0 {
		queueMaxWorkers = defaultQueueMaxWorkers
	}
	return &river.Config{
		Schema:  RiverSchema(workspaceID),
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: queueMaxWorkers},
		},
	}
}

// NewWorkspaceClient constructs a river client bound to a workspace's
// per-tenant river schema (ADR-009 §8.3). pool MUST be authenticated as the
// workspace role so it can reach river_<hex>; the role barrier is what stops a
// mis-routed client from touching another tenant's job table. Once Started,
// the returned client fetches and works the four sequences job types
// (registered on workers via RegisterWorkers) against that schema.
func NewWorkspaceClient(
	pool *pgxpool.Pool,
	workspaceID uuid.UUID,
	workers *river.Workers,
	queueMaxWorkers int,
) (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(pool), WorkspaceRiverConfig(workspaceID, workers, queueMaxWorkers))
}
