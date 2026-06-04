//go:build integration

// Integration tests for the pipeline-stages endpoints and atomic deal-stage
// transitions added in the Sprint 13 Kanban tasket.
//
// Run:
//
//	~/.local/go/bin/go -C apps/api test -tags integration -count 1 -race -v \
//	    -run TestPipeline ./internal/crm

package crm_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/gbconsult/lecrm/apps/api/internal/crm"
	"github.com/gbconsult/lecrm/apps/api/internal/rbac"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

const pipelineDomainTLD = "lecrm.test"

func connectWithRetry(ctx context.Context, connStr string, maxWait time.Duration) (*pgx.Conn, error) {
	deadline := time.Now().Add(maxWait)
	var (
		conn *pgx.Conn
		err  error
	)
	for {
		conn, err = pgx.Connect(ctx, connStr)
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("after %s: %w", maxWait, err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// pipelineMigrationPaths returns the FULL production migration chain (every
// NNNN_*.sql, sorted), shared by every crm integration harness. It replaces a
// per-file hardcoded list that had to be hand-extended for each new migration
// and silently lagged prod otherwise (e.g. the json-property regression in
// 0024 would never reach a harness pinned at 0023). Globbing keeps the chain
// in lockstep with prod; the zero-padded NNNN_ prefix makes lexical sort ==
// numeric order and a renumber gap (no 0020) is handled transparently.
func pipelineMigrationPaths(t *testing.T) []string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile: apps/api/internal/crm/pipeline_integration_test.go
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	migrationsDir := filepath.Join(repoRoot, "packages", "db", "migrations")
	paths, err := filepath.Glob(filepath.Join(migrationsDir, "[0-9]*.sql"))
	if err != nil {
		t.Fatalf("glob migrations in %s: %v", migrationsDir, err)
	}
	if len(paths) == 0 {
		t.Fatalf("no migrations found in %s", migrationsDir)
	}
	sort.Strings(paths)
	return paths
}

type pipelineTestEnv struct {
	pool      *pgxpool.Pool
	srv       *httptest.Server
	transport http.RoundTripper
	wsA       workspaceFixture
	wsB       workspaceFixture
}

type workspaceFixture struct {
	id       uuid.UUID
	slug     string
	roleName string
	client   *http.Client
}

type hostRoundTripper struct {
	base http.RoundTripper
	host string
}

func (h *hostRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Host = h.host
	return h.base.RoundTrip(r)
}

func setupPipelineEnv(t *testing.T) *pipelineTestEnv {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("lecrm"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("testpass"),
		// Full prod migration chain (sorted glob). 0021 re-seeds the
		// gbconsult-default pipeline with French stage labels (Découverte,
		// Qualifié, …) that TestPipeline_ListStages asserts and the connector
		// path depends on; 0023 admits the 'connector' actor_type in
		// core.audit_log. 0021's _with_registry delegates to the base provision
		// fn (redefined by 0022, which seeds no stages) then seeds the French
		// stages itself, so 0022 ordering after 0021 neither duplicates nor
		// reverts them.
		tcpostgres.WithInitScripts(pipelineMigrationPaths(t)...),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() {
		_ = ctr.Terminate(context.Background())
	})

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	// Postgres briefly resets connections while finishing init scripts and
	// restarting. Open one connection with retry to wait the server out
	// before handing the connection string to a pgxpool.
	probe, err := connectWithRetry(ctx, connStr, 30*time.Second)
	if err != nil {
		t.Fatalf("probe connect: %v", err)
	}
	_ = probe.Close(ctx)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	provision := func(slug string) workspaceFixture {
		id := uuid.New()
		var roleName string
		err := pool.QueryRow(ctx,
			"SELECT core.lecrm_provision_workspace_with_registry($1, $2, $3, $4, $5)",
			id, slug, "admin@"+slug+".test", "creator@"+slug+".test", "gbconsult-default",
		).Scan(&roleName)
		if err != nil {
			t.Fatalf("provision %s: %v", slug, err)
		}
		return workspaceFixture{id: id, slug: slug, roleName: roleName}
	}

	wsA := provision("acme-a")
	wsB := provision("acme-b")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	resolver := &workspace.PoolResolver{Pool: pool}
	handler := &crm.Handler{Pool: pool, Logger: logger}

	router := chi.NewRouter()
	router.Group(func(r chi.Router) {
		r.Use(workspace.Middleware(logger, resolver, pipelineDomainTLD))
		// The CRM write handlers resolve their capability.Principal from an
		// rbac.Principal in context (handlers.go principalFrom); production
		// installs it via rbac.Resolve from the session/token. This harness has
		// no auth front-end, so inject an owner principal — without it the
		// CreateContact/import-commit write calls return 401 before any handler
		// logic runs. Mirrors the dedup harness fix (commit 82844ade).
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := rbac.WithPrincipal(req.Context(), &rbac.Principal{
					Role:      rbac.RoleOwner,
					ActorType: "human_api",
				})
				next.ServeHTTP(w, req.WithContext(ctx))
			})
		})
		handler.RegisterRoutes(r)
		// Activities/notes/tasks live on a separate registrar; production (and
		// the contract test) wire both. Without this, the ANT tests that share
		// this harness 404.
		handler.RegisterANTRoutes(r)
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	transport := srv.Client().Transport

	for _, ws := range []*workspaceFixture{&wsA, &wsB} {
		ws.client = &http.Client{
			Transport: &hostRoundTripper{base: transport, host: ws.slug + "." + pipelineDomainTLD},
			Timeout:   10 * time.Second,
		}
	}

	return &pipelineTestEnv{
		pool:      pool,
		srv:       srv,
		transport: transport,
		wsA:       wsA,
		wsB:       wsB,
	}
}

func (e *pipelineTestEnv) doJSON(t *testing.T, ws workspaceFixture, method, path string, body any) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := ws.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}

func (e *pipelineTestEnv) countActivities(t *testing.T, ws workspaceFixture, dealID uuid.UUID) int {
	t.Helper()
	q := fmt.Sprintf(`SELECT count(*) FROM %q.objects WHERE object_type='activity' AND parent_type='deal' AND parent_id=$1`, ws.roleName)
	var n int
	if err := e.pool.QueryRow(context.Background(), q, dealID).Scan(&n); err != nil {
		t.Fatalf("count activities: %v", err)
	}
	return n
}

type pipelineStageJSON struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	OrderIndex int32     `json:"order_index"`
	CreatedAt  time.Time `json:"created_at"`
}

type dealJSON struct {
	ID      uuid.UUID `json:"id"`
	Title   string    `json:"title"`
	StageID *string   `json:"stage_id"`
}

func (e *pipelineTestEnv) listStages(t *testing.T, ws workspaceFixture) []pipelineStageJSON {
	t.Helper()
	status, body := e.doJSON(t, ws, http.MethodGet, "/v1/pipeline/stages", nil)
	if status != http.StatusOK {
		t.Fatalf("list stages: status=%d body=%s", status, body)
	}
	var resp struct {
		Data []pipelineStageJSON `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode stages: %v; body=%s", err, body)
	}
	return resp.Data
}

func (e *pipelineTestEnv) createDeal(t *testing.T, ws workspaceFixture, title string, stageID uuid.UUID) dealJSON {
	t.Helper()
	stageStr := stageID.String()
	status, body := e.doJSON(t, ws, http.MethodPost, "/v1/deals", map[string]any{
		"title":    title,
		"stage_id": stageStr,
	})
	if status != http.StatusCreated {
		t.Fatalf("create deal: status=%d body=%s", status, body)
	}
	var d dealJSON
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatalf("decode deal: %v; body=%s", err, body)
	}
	return d
}

// --- tests ---

func TestPipeline_ListStages_ReturnsSeededStagesOrdered(t *testing.T) {
	env := setupPipelineEnv(t)

	stages := env.listStages(t, env.wsA)
	want := []string{"Découverte", "Qualifié", "Proposition envoyée", "Négociation", "Gagné / Perdu"}
	if len(stages) != len(want) {
		t.Fatalf("len(stages): got %d want %d", len(stages), len(want))
	}
	for i, s := range stages {
		if s.Name != want[i] {
			t.Errorf("stage[%d]: got %q want %q", i, s.Name, want[i])
		}
		if int(s.OrderIndex) != i+1 {
			t.Errorf("stage[%d] order_index: got %d want %d", i, s.OrderIndex, i+1)
		}
	}
}

func TestPipeline_TransitionDealStage_WritesActivityAtomically(t *testing.T) {
	env := setupPipelineEnv(t)
	stages := env.listStages(t, env.wsA)

	deal := env.createDeal(t, env.wsA, "Big Deal", stages[0].ID)
	if got := env.countActivities(t, env.wsA, deal.ID); got != 0 {
		t.Fatalf("activities before transition: got %d want 0", got)
	}

	status, body := env.doJSON(t, env.wsA, http.MethodPatch,
		"/v1/deals/"+deal.ID.String()+"/stage",
		map[string]any{"stage_id": stages[1].ID.String()})
	if status != http.StatusOK {
		t.Fatalf("transition: status=%d body=%s", status, body)
	}

	var updated dealJSON
	if err := json.Unmarshal(body, &updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.StageID == nil || *updated.StageID != stages[1].ID.String() {
		t.Fatalf("stage_id after transition: got %v want %s", updated.StageID, stages[1].ID)
	}
	if got := env.countActivities(t, env.wsA, deal.ID); got != 1 {
		t.Fatalf("activities after transition: got %d want 1", got)
	}
}

func TestPipeline_TransitionDealStage_IdempotentSameStage(t *testing.T) {
	env := setupPipelineEnv(t)
	stages := env.listStages(t, env.wsA)
	deal := env.createDeal(t, env.wsA, "Same-Stage", stages[1].ID)

	status, _ := env.doJSON(t, env.wsA, http.MethodPatch,
		"/v1/deals/"+deal.ID.String()+"/stage",
		map[string]any{"stage_id": stages[1].ID.String()})
	if status != http.StatusOK {
		t.Fatalf("idempotent PATCH: status=%d want 200", status)
	}
	if got := env.countActivities(t, env.wsA, deal.ID); got != 0 {
		t.Fatalf("activities after no-op transition: got %d want 0", got)
	}
}

func TestPipeline_TransitionDealStage_RandomStageID_Returns400(t *testing.T) {
	env := setupPipelineEnv(t)
	stages := env.listStages(t, env.wsA)
	deal := env.createDeal(t, env.wsA, "Bad Stage", stages[0].ID)

	bogus := uuid.New().String()
	status, _ := env.doJSON(t, env.wsA, http.MethodPatch,
		"/v1/deals/"+deal.ID.String()+"/stage",
		map[string]any{"stage_id": bogus})
	if status != http.StatusBadRequest {
		t.Fatalf("bogus stage: status=%d want 400", status)
	}
	if got := env.countActivities(t, env.wsA, deal.ID); got != 0 {
		t.Fatalf("activities after bad PATCH: got %d want 0", got)
	}
}

func TestPipeline_TransitionDealStage_CrossTenantIsolation(t *testing.T) {
	env := setupPipelineEnv(t)
	stagesA := env.listStages(t, env.wsA)
	stagesB := env.listStages(t, env.wsB)
	dealA := env.createDeal(t, env.wsA, "Tenant A Deal", stagesA[0].ID)

	// Workspace B cannot find workspace A's deal.
	status, _ := env.doJSON(t, env.wsB, http.MethodPatch,
		"/v1/deals/"+dealA.ID.String()+"/stage",
		map[string]any{"stage_id": stagesB[1].ID.String()})
	if status != http.StatusNotFound {
		t.Fatalf("cross-tenant deal access: status=%d want 404", status)
	}

	// Workspace A cannot use a stage_id that belongs to workspace B's schema.
	status, _ = env.doJSON(t, env.wsA, http.MethodPatch,
		"/v1/deals/"+dealA.ID.String()+"/stage",
		map[string]any{"stage_id": stagesB[1].ID.String()})
	if status != http.StatusBadRequest {
		t.Fatalf("cross-tenant stage usage: status=%d want 400", status)
	}

	if got := env.countActivities(t, env.wsA, dealA.ID); got != 0 {
		t.Fatalf("cross-tenant should not mutate: activities=%d want 0", got)
	}
}
