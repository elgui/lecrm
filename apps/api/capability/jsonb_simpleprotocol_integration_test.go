//go:build integration

// Regression guard for the production-only jsonb-encoding bug (2026-06-04).
//
// The control-plane and per-tenant pools run with
// pgx.QueryExecModeSimpleProtocol (db.Open / tenant_pool.go) so they are
// PgBouncer-transaction-mode safe. Under the simple protocol pgx has no
// parameter-OID information, so a []byte argument is encoded as a bytea
// literal (\x7b…) — which a jsonb column rejects with
// "invalid input syntax for type json" (SQLSTATE 22P02). Every audit /
// activity / objects.data write marshals its payload to []byte, so the
// entire audited write surface 500'd in production while the integration
// suite (which builds its pools with pgxpool.New → the DEFAULT extended
// protocol, where OID inference makes []byte→jsonb work) stayed green.
//
// The fix passes string(body) instead of body at every jsonb insert. This
// test reproduces the prod conditions — a SIMPLE-protocol pool driving the
// real capability ops — so a regression to []byte fails here, not just in
// production.
//
// Run (docker required):
//
//	~/.local/go/bin/go -C apps/api test -tags integration -count 1 -race -v \
//	    -run TestSimpleProtocol ./capability
package capability

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// simpleProtocolService clones env.pool's configuration, forces the simple
// query protocol (matching db.Open in production), and returns a Service
// backed by the new pool.
func simpleProtocolService(t *testing.T, env *intentEnv) *Service {
	t.Helper()
	ctx := context.Background()
	cfg := env.pool.Config().Copy()
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("simple-protocol pool: %v", err)
	}
	t.Cleanup(pool.Close)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return New(pool, logger)
}

// TestSimpleProtocol_JSONBWrites exercises the audited write paths over a
// simple-protocol pool. With the pre-fix []byte arguments every one of
// these returns 22P02; with string(body) they all succeed.
func TestSimpleProtocol_JSONBWrites(t *testing.T) {
	env := setupIntentEnv(t)
	svc := simpleProtocolService(t, env)
	ctx := context.Background()

	// (1) CreateContact → core.audit_log (contact.created) + per-tenant
	// activities (entity.created). Covers EmitAudit + EmitActivity.
	email := "simpleproto@example.test"
	res, err := svc.CreateContact(ctx, writeP(env.wsA),
		CreateContactInput{FirstName: "Simple", LastName: "Protocol", Email: &email}, "")
	if err != nil {
		t.Fatalf("CreateContact under simple protocol: %v (regression: []byte→jsonb 22P02?)", err)
	}
	if res.Status != 201 {
		t.Fatalf("CreateContact status = %d, want 201", res.Status)
	}
	if n := env.count(t, `SELECT count(*) FROM core.audit_log WHERE workspace_id=$1 AND event='contact.created'`, env.wsA); n != 1 {
		t.Fatalf("want 1 contact.created audit row, got %d", n)
	}

	// (2) AdvanceDeal → objects.data deal activity (intentops.go) +
	// core.audit_log (deal.advanced). Covers the objects.data jsonb insert.
	// Discover stage names dynamically (the gbconsult-default template's
	// stage labels are not stable across migrations).
	sc := env.schema(env.wsA)
	var firstStage, secondStage string
	rows, err := env.pool.Query(ctx,
		`SELECT name FROM `+pgIdent(sc)+`.pipeline_stages ORDER BY order_index LIMIT 2`)
	if err != nil {
		t.Fatalf("list stages: %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan stage: %v", err)
		}
		names = append(names, n)
	}
	if len(names) < 2 {
		t.Fatalf("need >=2 pipeline stages, got %v", names)
	}
	firstStage, secondStage = names[0], names[1]

	deal := env.seedDeal(t, env.wsA, "Simple-protocol deal", firstStage)
	if _, err := svc.AdvanceDeal(ctx, writeP(env.wsA),
		AdvanceDealInput{Deal: deal.String(), ToStage: secondStage},
		WriteOptions{}, nil, nil); err != nil {
		t.Fatalf("AdvanceDeal under simple protocol: %v (regression: []byte→jsonb 22P02?)", err)
	}
	if n := env.count(t, `SELECT count(*) FROM core.audit_log WHERE workspace_id=$1 AND event='deal.advanced'`, env.wsA); n != 1 {
		t.Fatalf("want 1 deal.advanced audit row, got %d", n)
	}
}
