//go:build integration

// Integration tests for the native report engine against a real Postgres.
// They cover what the unit tests cannot: workspace isolation (a report run in
// tenant A never sees tenant B's deals), grouping by a custom property, the
// N-1 year-over-year comparison column, and the saved-definition CRUD roundtrip
// — all through capability.{Read,Write}Tx (search_path-scoped), exactly as the
// HTTP handler runs them.
//
// Run (docker required):
//
//	~/.local/go/bin/go -C apps/api test -tags integration -count 1 -race -v \
//	    -run TestReportEngine ./internal/reports
package reports

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

type reportEnv struct {
	pool *pgxpool.Pool
	wsA  uuid.UUID
	wsB  uuid.UUID
}

func schemaName(id uuid.UUID) string {
	return "workspace_" + strings.ReplaceAll(id.String(), "-", "")
}

func migPath(t *testing.T, filename string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile: apps/api/internal/reports/run_integration_test.go → repo root is ../../../..
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	p := filepath.Join(repoRoot, "packages", "db", "migrations", filename)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("migration %s not found at %s: %v", filename, p, err)
	}
	return p
}

func setupReportEnv(t *testing.T) *reportEnv {
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
		"0016_service_tokens.sql", "0017_app_role.sql",
		"0018_integrator_role_and_grants.sql", "0019_integrator_audit_actor.sql",
		"0021_french_pipeline_stages.sql",
	}
	scripts := make([]string, len(migrations))
	for i, m := range migrations {
		scripts[i] = migPath(t, m)
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
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if c, e := pgx.Connect(ctx, connStr); e == nil {
			if e = c.Ping(ctx); e == nil {
				_ = c.Close(ctx)
				break
			}
			_ = c.Close(ctx)
		}
		time.Sleep(250 * time.Millisecond)
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

	return &reportEnv{pool: pool, wsA: provision("rep-a"), wsB: provision("rep-b")}
}

func (e *reportEnv) stageID(t *testing.T, ws uuid.UUID, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.pool.QueryRow(context.Background(),
		`SELECT id FROM `+pgx.Identifier{schemaName(ws)}.Sanitize()+`.pipeline_stages WHERE name = $1`,
		name).Scan(&id); err != nil {
		t.Fatalf("stage %q: %v", name, err)
	}
	return id
}

// seedDeal inserts a deal with an explicit created_at + amount + closed flag.
func (e *reportEnv) seedDeal(t *testing.T, ws uuid.UUID, title, stage string, amount float64, createdAt time.Time, closed bool) uuid.UUID {
	t.Helper()
	tbl := pgx.Identifier{schemaName(ws)}.Sanitize()
	var closedAt *time.Time
	if closed {
		closedAt = &createdAt
	}
	var id uuid.UUID
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO `+tbl+`.deals (title, amount, stage_id, created_at, closed_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		title, amount, e.stageID(t, ws, stage), createdAt, closedAt).Scan(&id); err != nil {
		t.Fatalf("seed deal: %v", err)
	}
	return id
}

func (e *reportEnv) setCustomProp(t *testing.T, ws, dealID uuid.UUID, jsonData string) {
	t.Helper()
	tbl := pgx.Identifier{schemaName(ws)}.Sanitize()
	if _, err := e.pool.Exec(context.Background(),
		`INSERT INTO `+tbl+`.objects (object_type, parent_type, parent_id, data)
		 VALUES ('custom_properties', 'deal', $1, $2::jsonb)`,
		dealID, jsonData); err != nil {
		t.Fatalf("set custom prop: %v", err)
	}
}

func TestReportEngine_WorkspaceIsolation(t *testing.T) {
	e := setupReportEnv(t)
	ctx := context.Background()
	now := time.Now()

	e.seedDeal(t, e.wsA, "A deal 1", "Découverte", 1000, now, false)
	e.seedDeal(t, e.wsA, "A deal 2", "Qualifié", 2000, now, false)
	// Tenant B has 5 deals — none of which must show up in A's report.
	for i := 0; i < 5; i++ {
		e.seedDeal(t, e.wsB, "B deal", "Découverte", 999, now, false)
	}

	storeA := NewStore(e.pool, schemaName(e.wsA), e.wsA)
	res, err := storeA.Run(ctx, Definition{Metric: MetricDealCount, Dimension: DimNone, Period: PeriodAll}, now)
	if err != nil {
		t.Fatalf("run A: %v", err)
	}
	if len(res.Rows) != 1 || res.Rows[0].Current != 2 {
		t.Fatalf("tenant A must count exactly its 2 deals, got %+v", res.Rows)
	}

	storeB := NewStore(e.pool, schemaName(e.wsB), e.wsB)
	resB, err := storeB.Run(ctx, Definition{Metric: MetricDealCount, Dimension: DimNone, Period: PeriodAll}, now)
	if err != nil {
		t.Fatalf("run B: %v", err)
	}
	if resB.Rows[0].Current != 5 {
		t.Fatalf("tenant B should count its 5 deals, got %+v", resB.Rows)
	}
}

func TestReportEngine_GroupByCustomProperty(t *testing.T) {
	e := setupReportEnv(t)
	ctx := context.Background()
	now := time.Now()

	d1 := e.seedDeal(t, e.wsA, "web1", "Découverte", 100, now, false)
	d2 := e.seedDeal(t, e.wsA, "web2", "Qualifié", 200, now, false)
	d3 := e.seedDeal(t, e.wsA, "salon1", "Découverte", 300, now, false)
	e.setCustomProp(t, e.wsA, d1, `{"source_du_lead":"Site web"}`)
	e.setCustomProp(t, e.wsA, d2, `{"source_du_lead":"Site web"}`)
	e.setCustomProp(t, e.wsA, d3, `{"source_du_lead":"Salon"}`)

	storeA := NewStore(e.pool, schemaName(e.wsA), e.wsA)
	res, err := storeA.Run(ctx, Definition{
		Metric: MetricDealCount, Dimension: "custom:source_du_lead", Period: PeriodAll}, now)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := map[string]float64{}
	for _, r := range res.Rows {
		got[r.Label] = r.Current
	}
	if got["Site web"] != 2 || got["Salon"] != 1 {
		t.Fatalf("custom-property buckets wrong: %+v", res.Rows)
	}
}

func TestReportEngine_YearOverYearComparison(t *testing.T) {
	e := setupReportEnv(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	// Current year (2026): 3 deals. Prior year (2025): 1 deal.
	e.seedDeal(t, e.wsA, "cur1", "Découverte", 100, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), false)
	e.seedDeal(t, e.wsA, "cur2", "Découverte", 100, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), false)
	e.seedDeal(t, e.wsA, "cur3", "Découverte", 100, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), false)
	e.seedDeal(t, e.wsA, "prev1", "Découverte", 100, time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC), false)

	storeA := NewStore(e.pool, schemaName(e.wsA), e.wsA)
	res, err := storeA.Run(ctx, Definition{
		Metric: MetricDealCount, Dimension: DimNone, Period: PeriodYear, CompareYoY: true}, now)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.CompareYoY {
		t.Fatal("expected CompareYoY=true in result")
	}
	if len(res.Rows) != 1 {
		t.Fatalf("expected single total row, got %+v", res.Rows)
	}
	row := res.Rows[0]
	if row.Current != 3 {
		t.Errorf("current year count = %v, want 3", row.Current)
	}
	if row.Prior == nil || *row.Prior != 1 {
		t.Errorf("prior year count = %v, want 1", row.Prior)
	}
	if res.CurrentLabel != "2026" || res.PriorLabel != "2025" {
		t.Errorf("labels: cur=%q prior=%q", res.CurrentLabel, res.PriorLabel)
	}
}

func TestReportEngine_SavedDefinitionCRUD(t *testing.T) {
	e := setupReportEnv(t)
	ctx := context.Background()
	storeA := NewStore(e.pool, schemaName(e.wsA), e.wsA)

	def := Definition{Name: "Mix CA par étape", Metric: MetricDealAmountSum, Dimension: DimStage, Period: PeriodYear, CompareYoY: true}
	created, err := storeA.CreateSaved(ctx, def)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == uuid.Nil || created.Definition.Name != def.Name {
		t.Fatalf("created malformed: %+v", created)
	}

	got, err := storeA.GetSaved(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Definition.Metric != MetricDealAmountSum || !got.Definition.CompareYoY {
		t.Fatalf("get mismatch: %+v", got.Definition)
	}

	got.Definition.Name = "renamed"
	updated, err := storeA.UpdateSaved(ctx, created.ID, got.Definition)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Definition.Name != "renamed" {
		t.Fatalf("update did not persist name: %+v", updated.Definition)
	}

	list, err := storeA.ListSaved(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 saved report, got %d", len(list))
	}

	if err := storeA.DeleteSaved(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := storeA.GetSaved(ctx, created.ID); !IsNotFound(err) {
		t.Fatalf("expected not-found after delete, got %v", err)
	}

	// A saved report in A must be invisible to B (workspace isolation).
	if _, err := storeA.CreateSaved(ctx, def); err != nil {
		t.Fatalf("recreate: %v", err)
	}
	storeB := NewStore(e.pool, schemaName(e.wsB), e.wsB)
	bList, err := storeB.ListSaved(ctx)
	if err != nil {
		t.Fatalf("list B: %v", err)
	}
	if len(bList) != 0 {
		t.Fatalf("tenant B must not see A's saved reports, got %d", len(bList))
	}
}
