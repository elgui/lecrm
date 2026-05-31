//go:build integration

// Integration tests for the MCP intent write tools (ADR-012 §3): advance_deal,
// log_interaction, capture_lead, exercised against a real Postgres. They cover
// the behaviours the unit tests cannot reach without a database: fuzzy
// stage-name matching, dry-run-mutates-nothing, the confirmation handshake,
// contact upsert/dedup (the shared connector path), deal creation in the first
// stage, idempotent replay, cross-tenant isolation, and audit attribution.
//
// Run (docker required):
//
//	~/.local/go/bin/go -C apps/api test -tags integration -count 1 -race -v \
//	    -run TestIntent ./capability
package capability

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

type intentEnv struct {
	pool *pgxpool.Pool
	svc  *Service
	wsA  uuid.UUID
	wsB  uuid.UUID
}

func migrationPath(t *testing.T, filename string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile: apps/api/capability/intentops_integration_test.go → repo root is ../../..
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	p := filepath.Join(repoRoot, "packages", "db", "migrations", filename)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("migration %s not found at %s: %v", filename, p, err)
	}
	return p
}

func connectRetry(ctx context.Context, connStr string, maxWait time.Duration) (*pgx.Conn, error) {
	deadline := time.Now().Add(maxWait)
	var lastErr error
	for time.Now().Before(deadline) {
		c, err := pgx.Connect(ctx, connStr)
		if err == nil {
			if err = c.Ping(ctx); err == nil {
				return c, nil
			}
			_ = c.Close(ctx)
		}
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	return nil, lastErr
}

func setupIntentEnv(t *testing.T) *intentEnv {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	migrations := []string{
		"0001_init.sql", "0002_identity.sql", "0003_metadata_engine.sql",
		"0004_workspaces_admin_email_registry.sql", "0005_slug_tombstoning.sql",
		"0006_security_definer_hardening.sql", "0007_session_revocations.sql",
		"0008_crm_entities.sql", "0009_metadata_json_type.sql",
		"0010_pgcrypto_to_core_schema.sql", "0011_external_sync.sql",
		"0012_email_suppression.sql", "0013_workspace_ro_role.sql",
		"0014_idempotency_keys.sql", "0015_activities_notes_tasks.sql",
		"0016_service_tokens.sql",
	}
	scripts := make([]string, len(migrations))
	for i, m := range migrations {
		scripts[i] = migrationPath(t, m)
	}

	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("lecrm"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.WithInitScripts(scripts...),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	if probe, err := connectRetry(ctx, connStr, 30*time.Second); err != nil {
		t.Fatalf("probe connect: %v", err)
	} else {
		_ = probe.Close(ctx)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	provision := func(slug string) uuid.UUID {
		id := uuid.New()
		var roleName string
		if err := pool.QueryRow(ctx,
			"SELECT core.lecrm_provision_workspace_with_registry($1, $2, $3, $4, $5)",
			id, slug, "admin@"+slug+".test", "creator@"+slug+".test", "gbconsult-default",
		).Scan(&roleName); err != nil {
			t.Fatalf("provision %s: %v", slug, err)
		}
		return id
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return &intentEnv{
		pool: pool,
		svc:  New(pool, logger),
		wsA:  provision("intent-a"),
		wsB:  provision("intent-b"),
	}
}

func (e *intentEnv) schema(ws uuid.UUID) string { return MCPSchemaName(ws) }

func (e *intentEnv) stageID(t *testing.T, ws uuid.UUID, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.pool.QueryRow(context.Background(),
		`SELECT id FROM `+pgIdent(e.schema(ws))+`.pipeline_stages WHERE name = $1`, name).Scan(&id); err != nil {
		t.Fatalf("stage %q: %v", name, err)
	}
	return id
}

func (e *intentEnv) seedDeal(t *testing.T, ws uuid.UUID, title, stage string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO `+pgIdent(e.schema(ws))+`.deals (title, stage_id) VALUES ($1, $2) RETURNING id`,
		title, e.stageID(t, ws, stage)).Scan(&id); err != nil {
		t.Fatalf("seed deal: %v", err)
	}
	return id
}

func (e *intentEnv) dealStageName(t *testing.T, ws, dealID uuid.UUID) string {
	t.Helper()
	var name string
	if err := e.pool.QueryRow(context.Background(),
		`SELECT s.name FROM `+pgIdent(e.schema(ws))+`.deals d
		   JOIN `+pgIdent(e.schema(ws))+`.pipeline_stages s ON s.id = d.stage_id
		  WHERE d.id = $1`, dealID).Scan(&name); err != nil {
		t.Fatalf("deal stage name: %v", err)
	}
	return name
}

func (e *intentEnv) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v\nquery: %s", err, q)
	}
	return n
}

// pgIdent quotes an identifier for safe interpolation in test SQL. The schema
// names are [0-9a-f_] by construction but quoting keeps the queries honest.
func pgIdent(s string) string { return pgx.Identifier{s}.Sanitize() }

func writeP(ws uuid.UUID) Principal { return MCPWritePrincipal(ws, []string{ScopeCRMWrite}) }
func readP(ws uuid.UUID) Principal  { return MCPWritePrincipal(ws, []string{ScopeCRMRead}) }

// --- advance_deal ---

func TestIntent_AdvanceDeal_FuzzyStageAndActivity(t *testing.T) {
	e := setupIntentEnv(t)
	ctx := context.Background()
	deal := e.seedDeal(t, e.wsA, "Acme renewal", "Découverte")

	res, err := e.svc.AdvanceDeal(ctx, writeP(e.wsA),
		AdvanceDealInput{Deal: deal.String(), ToStage: "négociation"}, // lowercase fuzzy
		WriteOptions{}, nil, nil)
	if err != nil {
		t.Fatalf("advance_deal: %v", err)
	}
	if res.Preview != nil {
		t.Fatal("non-dry-run must not return a preview")
	}
	if got := e.dealStageName(t, e.wsA, deal); got != "Négociation" {
		t.Fatalf("stage = %q, want Négociation (fuzzy match)", got)
	}
	sc := e.schema(e.wsA)
	if n := e.count(t, `SELECT count(*) FROM `+pgIdent(sc)+`.activities WHERE entity_type='deal' AND entity_id=$1 AND event_type='deal.stage_changed'`, deal); n != 1 {
		t.Fatalf("want 1 stage_changed activity, got %d", n)
	}
	// Audit row attributed to the MCP agent.
	if n := e.count(t, `SELECT count(*) FROM core.audit_log WHERE workspace_id=$1 AND event='deal.advanced' AND actor_type=$2`, e.wsA, ActorTypeMCPAgent); n != 1 {
		t.Fatalf("want 1 mcp_agent audit row, got %d", n)
	}
}

func TestIntent_AdvanceDeal_DryRunMutatesNothing(t *testing.T) {
	e := setupIntentEnv(t)
	ctx := context.Background()
	deal := e.seedDeal(t, e.wsA, "Globex pilot", "Découverte")
	sc := e.schema(e.wsA)
	before := e.count(t, `SELECT count(*) FROM `+pgIdent(sc)+`.activities`)

	res, err := e.svc.AdvanceDeal(ctx, writeP(e.wsA),
		AdvanceDealInput{Deal: deal.String(), ToStage: "Négociation"},
		WriteOptions{DryRun: true}, nil, nil)
	if err != nil {
		t.Fatalf("dry-run advance_deal: %v", err)
	}
	if res.Preview == nil || !res.Preview.DryRun {
		t.Fatal("dry-run must return a Preview")
	}
	if got := e.dealStageName(t, e.wsA, deal); got != "Découverte" {
		t.Fatalf("dry-run must not move the deal; stage = %q", got)
	}
	if after := e.count(t, `SELECT count(*) FROM `+pgIdent(sc)+`.activities`); after != before {
		t.Fatalf("dry-run wrote %d activities", after-before)
	}
}

func TestIntent_AdvanceDeal_MarkClosedRequiresConfirmation(t *testing.T) {
	e := setupIntentEnv(t)
	ctx := context.Background()
	deal := e.seedDeal(t, e.wsA, "Initech deal", "Découverte")
	conf, err := NewConfirmer([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC) }
	closing := "today"
	in := AdvanceDealInput{Deal: deal.String(), ToStage: "Gagné / Perdu", MarkClosedAt: &closing}

	// Real call with no token → confirmation required, no mutation.
	if _, err := e.svc.AdvanceDeal(ctx, writeP(e.wsA), in, WriteOptions{}, conf, now); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("want ErrConfirmationRequired, got %v", err)
	}
	// Dry-run → preview carries a confirmation token.
	dry, err := e.svc.AdvanceDeal(ctx, writeP(e.wsA), in, WriteOptions{DryRun: true}, conf, now)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if dry.Preview == nil || !dry.Preview.ConfirmationRequired || dry.Preview.ConfirmationToken == "" {
		t.Fatalf("dry-run of a destructive op must require + return a token: %+v", dry.Preview)
	}
	// Real call echoing the token → commits and stamps closed_at.
	if _, err := e.svc.AdvanceDeal(ctx, writeP(e.wsA), in,
		WriteOptions{ConfirmationToken: dry.Preview.ConfirmationToken}, conf, now); err != nil {
		t.Fatalf("confirmed advance_deal: %v", err)
	}
	sc := e.schema(e.wsA)
	if n := e.count(t, `SELECT count(*) FROM `+pgIdent(sc)+`.deals WHERE id=$1 AND closed_at IS NOT NULL`, deal); n != 1 {
		t.Fatal("confirmed close must set closed_at")
	}
}

// --- log_interaction ---

func TestIntent_LogInteraction_UpsertsContact(t *testing.T) {
	e := setupIntentEnv(t)
	ctx := context.Background()
	sc := e.schema(e.wsA)

	res, err := e.svc.LogInteraction(ctx, writeP(e.wsA),
		LogInteractionInput{ContactOrCompany: "newlead@example.com", Summary: "Discovery call, very keen"},
		WriteOptions{})
	if err != nil {
		t.Fatalf("log_interaction: %v", err)
	}
	if res.Preview != nil {
		t.Fatal("unexpected preview")
	}
	if n := e.count(t, `SELECT count(*) FROM `+pgIdent(sc)+`.contacts WHERE email='newlead@example.com'`); n != 1 {
		t.Fatalf("want 1 upserted contact, got %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM `+pgIdent(sc)+`.activities WHERE entity_type='contact' AND event_type='interaction.logged'`); n != 1 {
		t.Fatalf("want 1 interaction activity, got %d", n)
	}
}

// --- capture_lead ---

func TestIntent_CaptureLead_DedupByEmail_DealInFirstStage(t *testing.T) {
	e := setupIntentEnv(t)
	ctx := context.Background()
	sc := e.schema(e.wsA)

	in := CaptureLeadInput{Name: "Ada Lovelace", Email: ptr("ada@analytical.io"), Company: ptr("Analytical Engines"), Source: "web-chat"}
	r1, err := e.svc.CaptureLead(ctx, writeP(e.wsA), in, WriteOptions{})
	if err != nil {
		t.Fatalf("capture_lead 1: %v", err)
	}
	// Second capture, same email → dedups onto the same contact.
	r2, err := e.svc.CaptureLead(ctx, writeP(e.wsA), in, WriteOptions{})
	if err != nil {
		t.Fatalf("capture_lead 2: %v", err)
	}
	if n := e.count(t, `SELECT count(*) FROM `+pgIdent(sc)+`.contacts WHERE email='ada@analytical.io'`); n != 1 {
		t.Fatalf("dedup by email failed: %d contacts", n)
	}
	// The deal opens in the first pipeline stage (lowest order_index = Découverte).
	var firstStage string
	if err := e.pool.QueryRow(ctx, `SELECT name FROM `+pgIdent(sc)+`.pipeline_stages ORDER BY order_index LIMIT 1`).Scan(&firstStage); err != nil {
		t.Fatal(err)
	}
	if firstStage != "Découverte" {
		t.Fatalf("first stage = %q, want Découverte", firstStage)
	}
	if n := e.count(t, `SELECT count(*) FROM `+pgIdent(sc)+`.deals d JOIN `+pgIdent(sc)+`.pipeline_stages s ON s.id=d.stage_id WHERE s.name='Découverte'`); n < 2 {
		t.Fatalf("each capture must open a deal in the first stage, got %d", n)
	}
	_ = r1
	_ = r2

	// Shares the connector upsert path: a contact created via the same
	// capability op (UpsertContactByEmail) is found, not duplicated.
	if err := WriteTx(ctx, e.pool, sc, func(tx pgx.Tx) error {
		id, created, e2 := UpsertContactByEmail(ctx, tx, UpsertContactParams{Email: "ada@analytical.io"})
		if e2 != nil {
			return e2
		}
		if created {
			t.Fatal("UpsertContactByEmail must find the existing capture_lead contact, not create a new one")
		}
		_ = id
		return nil
	}); err != nil {
		t.Fatalf("shared upsert check: %v", err)
	}
}

func TestIntent_CaptureLead_IdempotentReplay(t *testing.T) {
	e := setupIntentEnv(t)
	ctx := context.Background()
	sc := e.schema(e.wsA)
	in := CaptureLeadInput{Name: "Grace Hopper", Email: ptr("grace@navy.mil"), Source: "voice"}

	if _, err := e.svc.CaptureLead(ctx, writeP(e.wsA), in, WriteOptions{IdempotencyKey: "lead-001"}); err != nil {
		t.Fatalf("capture 1: %v", err)
	}
	r2, err := e.svc.CaptureLead(ctx, writeP(e.wsA), in, WriteOptions{IdempotencyKey: "lead-001"})
	if err != nil {
		t.Fatalf("capture 2 (replay): %v", err)
	}
	if !r2.Replayed {
		t.Fatal("second call with same idempotency key must replay")
	}
	if n := e.count(t, `SELECT count(*) FROM `+pgIdent(sc)+`.contacts WHERE email='grace@navy.mil'`); n != 1 {
		t.Fatalf("idempotent replay duplicated the contact: %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM `+pgIdent(sc)+`.deals WHERE title='Grace Hopper'`); n != 1 {
		t.Fatalf("idempotent replay duplicated the deal: %d", n)
	}
}

// --- security: scope gate + cross-tenant ---

func TestIntent_ReadOnlyTokenDenied(t *testing.T) {
	e := setupIntentEnv(t)
	ctx := context.Background()
	deal := e.seedDeal(t, e.wsA, "RO test", "Découverte")

	_, err := e.svc.AdvanceDeal(ctx, readP(e.wsA),
		AdvanceDealInput{Deal: deal.String(), ToStage: "Négociation"}, WriteOptions{}, nil, nil)
	if !errors.Is(err, ErrReadOnlyScope) {
		t.Fatalf("read-only token must be denied with ErrReadOnlyScope, got %v", err)
	}
	if got := e.dealStageName(t, e.wsA, deal); got != "Découverte" {
		t.Fatalf("denied write must not mutate; stage = %q", got)
	}
}

func TestIntent_CrossTenantWriteBlocked(t *testing.T) {
	e := setupIntentEnv(t)
	ctx := context.Background()
	// A deal that lives in workspace B.
	dealB := e.seedDeal(t, e.wsB, "B-only deal", "Découverte")

	// A write principal for workspace A pins search_path to A's schema, so B's
	// deal id is invisible → not found, and B is untouched.
	_, err := e.svc.AdvanceDeal(ctx, writeP(e.wsA),
		AdvanceDealInput{Deal: dealB.String(), ToStage: "Négociation"}, WriteOptions{}, nil, nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant write must be ErrNotFound, got %v", err)
	}
	if got := e.dealStageName(t, e.wsB, dealB); got != "Découverte" {
		t.Fatalf("workspace B deal must be untouched; stage = %q", got)
	}
}

func ptr[T any](v T) *T { return &v }
