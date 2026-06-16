//go:build integration

// Integration test for the per-workspace river runtime against a throwaway
// testcontainers Postgres (NOT the shared staging DB). It exercises the real
// production path end-to-end as the lecrm_api role:
//
//   provision workspace (full migration chain incl. 0027)
//     → river-setup (rivermigrate into river_<hex> + grant lecrm_api)
//     → Manager.Start brings up a per-workspace river client  [proves grants+tables]
//     → Enqueuer inserts poll_mailbox into that workspace's queue [proves insert grant]
//     → the poll worker runs (SET LOCAL search_path as lecrm_api) and advances
//       the sync_connections cursor                              [proves worker tx path]
//
// Run: go test -tags=integration ./internal/sequences/seqruntime/... -run Runtime
// Skipped automatically when Docker is unreachable.
package seqruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/gbconsult/lecrm/apps/api/internal/sequences/gmailreply"
	"github.com/gbconsult/lecrm/apps/api/internal/testfixtures/tenantpair"
)

// fakeHistory is a HistoryClient that returns no new messages and a fixed
// "latest" history id, so the poll worker advances the cursor without any
// Google network I/O.
type fakeHistory struct{ newHID uint64 }

func (f fakeHistory) MessagesSince(context.Context, uint64) ([]gmailreply.InboundMessage, uint64, error) {
	return nil, f.newHID, nil
}
func (f fakeHistory) Watch(context.Context) (uint64, time.Time, error) {
	return 999, time.Unix(0, 0), nil
}

type fakeFactory struct{ h gmailreply.HistoryClient }

func (f fakeFactory) Client(context.Context, uuid.UUID, uuid.UUID) (gmailreply.HistoryClient, error) {
	return f.h, nil
}

func riverSchema(id uuid.UUID) string {
	return "river_" + strings.ReplaceAll(id.String(), "-", "")
}

func TestRuntimeStartsAndPolls(t *testing.T) {
	pair := tenantpair.Provision(t) // testcontainers PG + all migrations + 2 workspaces
	ctx := context.Background()
	su := pair.A.DB() // superuser pool
	wsID := pair.A.ID
	role := pair.A.RoleName // workspace_<hex>
	connStr := su.Config().ConnString()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Faithful to prod's _with_registry path: grant lecrm_api DML on the
	// workspace schema (tenantpair calls the base provision fn, which does not).
	if _, err := su.Exec(ctx, "SELECT core.lecrm_grant_app_role($1)", role); err != nil {
		t.Fatalf("grant app role: %v", err)
	}

	// river-setup: create River tables in river_<hex> as the workspace role
	// (the schema owner) and grant lecrm_api — mirrors lecrm-migrate river-setup.
	riverSetup(t, ctx, connStr, wsID, role)

	// Assert river-setup actually granted lecrm_api what the runtime needs.
	var hasUsage, hasTable bool
	if err := su.QueryRow(ctx,
		"SELECT has_schema_privilege('lecrm_api', $1, 'USAGE'), has_table_privilege('lecrm_api', $1 || '.river_leader', 'SELECT')",
		riverSchema(wsID)).Scan(&hasUsage, &hasTable); err != nil {
		t.Fatalf("privilege probe: %v", err)
	}
	if !hasUsage || !hasTable {
		t.Fatalf("river-setup grants missing for lecrm_api on %s: USAGE=%v river_leader.SELECT=%v",
			riverSchema(wsID), hasUsage, hasTable)
	}

	// Build a pool AS lecrm_api (simple protocol, like db.Open) so the test
	// exercises the real role's privileges, not the superuser's.
	if _, err := su.Exec(ctx, "ALTER ROLE lecrm_api WITH PASSWORD 'testpass'"); err != nil {
		t.Fatalf("set lecrm_api password: %v", err)
	}
	apiPool := poolAs(t, ctx, connStr, "lecrm_api", "testpass")
	t.Cleanup(apiPool.Close)

	// Start the runtime on the lecrm_api pool with a fake Gmail client factory.
	acq := &SearchPathAcquirer{Pool: apiPool}
	mgr := NewManager(ManagerConfig{
		Pool:     apiPool,
		Acquirer: acq,
		GmailDeps: gmailreply.Deps{
			Acquirer:   acq,
			Clients:    fakeFactory{h: fakeHistory{newHID: 555}},
			Classifier: gmailreply.DefaultClassifier{},
			Logger:     logger,
		},
		Logger: logger,
	})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("manager start: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })

	// Assertion 1: a river client came up for the workspace (river tables exist +
	// lecrm_api can operate them).
	if _, ok := mgr.Client(wsID); !ok {
		t.Fatal("no river client started for workspace (river tables missing or grant failed?)")
	}

	// Seed the Gmail connection row AFTER Start so the watch-renew RunOnStart is a
	// no-op and the cursor advance below is attributable to the poll worker.
	userID := uuid.New()
	if _, err := su.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s.sync_connections (provider_id, status, settings)
		 VALUES ('gmail','active', jsonb_build_object('user_id', $1::text, 'email_address', $2::text))`,
		pgx.Identifier{role}.Sanitize()),
		userID.String(), "rep@demo.test"); err != nil {
		t.Fatalf("seed sync_connections: %v", err)
	}

	// Assertion 2: enqueue via the production Enqueuer (proves lecrm_api insert
	// grant + per-workspace routing), then the poll worker runs and advances the
	// cursor to the fake's newHID (proves the SET-LOCAL-search_path worker tx path).
	enq := &Enqueuer{Manager: mgr}
	if err := enq.EnqueuePollMailbox(ctx, gmailreply.PollMailboxArgs{
		WorkspaceID:  wsID,
		UserID:       userID,
		EmailAddress: "rep@demo.test",
		HistoryID:    100,
	}); err != nil {
		t.Fatalf("enqueue poll_mailbox: %v", err)
	}

	if got := waitForCursor(t, ctx, su, role, 10*time.Second); got != 555 {
		t.Fatalf("cursor history_id = %d, want 555 (poll worker did not run to completion)", got)
	}
}

// riverSetup replicates apps/migrate riversetup.SetupWorkspace (which lives in a
// separate module and isn't importable here): rivermigrate into river_<hex> as
// the workspace role, then grant lecrm_api.
func riverSetup(t *testing.T, ctx context.Context, connStr string, wsID uuid.UUID, role string) {
	t.Helper()
	schema := riverSchema(wsID)
	cfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	cfg.MaxConns = 2
	setRole := "SET ROLE " + pgx.Identifier{role}.Sanitize()
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, setRole)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("river pool: %v", err)
	}
	defer pool.Close()

	m, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{Schema: schema})
	if err != nil {
		t.Fatalf("rivermigrate new: %v", err)
	}
	if _, err := m.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		t.Fatalf("rivermigrate up: %v", err)
	}
	q := pgx.Identifier{schema}.Sanitize()
	for _, g := range []string{
		"GRANT USAGE ON SCHEMA " + q + " TO lecrm_api",
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA " + q + " TO lecrm_api",
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA " + q + " TO lecrm_api",
	} {
		if _, err := pool.Exec(ctx, g); err != nil {
			t.Fatalf("grant (%s): %v", g, err)
		}
	}
}

func poolAs(t *testing.T, ctx context.Context, connStr, user, password string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	cfg.ConnConfig.User = user
	cfg.ConnConfig.Password = password
	cfg.MaxConns = 5
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol // match db.Open
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool as %s: %v", user, err)
	}
	return pool
}

func waitForCursor(t *testing.T, ctx context.Context, su *pgxpool.Pool, role string, timeout time.Duration) uint64 {
	t.Helper()
	q := fmt.Sprintf("SELECT sync_cursor FROM %s.sync_connections WHERE provider_id='gmail'",
		pgx.Identifier{role}.Sanitize())
	deadline := time.Now().Add(timeout)
	for {
		var raw []byte
		if err := su.QueryRow(ctx, q).Scan(&raw); err == nil && len(raw) > 0 && string(raw) != "null" {
			var c struct {
				HistoryID uint64 `json:"history_id"`
			}
			if json.Unmarshal(raw, &c) == nil && c.HistoryID != 0 {
				return c.HistoryID
			}
		}
		if time.Now().After(deadline) {
			return 0
		}
		time.Sleep(200 * time.Millisecond)
	}
}
