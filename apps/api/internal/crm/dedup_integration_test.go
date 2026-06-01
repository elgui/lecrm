//go:build integration

// Integration tests for the duplicate-detection + merge endpoints
// (integrator-gap tasket 20260601-110828-76e8).
//
// Run:
//
//	go -C apps/api test -tags integration -count 1 -race -v \
//	    -run TestDedup ./internal/crm

package crm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/gbconsult/lecrm/apps/api/internal/crm"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"

	"log/slog"
	"net/http/httptest"
	"os"
)

// setupDedupEnv creates a fresh Postgres container with all migrations through
// 0022 applied, provisions two workspaces, and starts an httptest.Server.
func setupDedupEnv(t *testing.T) *pipelineTestEnv {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("lecrm"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.WithInitScripts(
			pipelineMigrationPath(t, "0001_init.sql"),
			pipelineMigrationPath(t, "0002_identity.sql"),
			pipelineMigrationPath(t, "0003_metadata_engine.sql"),
			pipelineMigrationPath(t, "0004_workspaces_admin_email_registry.sql"),
			pipelineMigrationPath(t, "0005_slug_tombstoning.sql"),
			pipelineMigrationPath(t, "0006_security_definer_hardening.sql"),
			pipelineMigrationPath(t, "0007_session_revocations.sql"),
			pipelineMigrationPath(t, "0008_crm_entities.sql"),
			pipelineMigrationPath(t, "0009_metadata_json_type.sql"),
			pipelineMigrationPath(t, "0010_pgcrypto_to_core_schema.sql"),
			pipelineMigrationPath(t, "0011_external_sync.sql"),
			pipelineMigrationPath(t, "0012_email_suppression.sql"),
			pipelineMigrationPath(t, "0013_workspace_ro_role.sql"),
			pipelineMigrationPath(t, "0014_idempotency_keys.sql"),
			pipelineMigrationPath(t, "0015_activities_notes_tasks.sql"),
			pipelineMigrationPath(t, "0022_dedup_no_merge_rules.sql"),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

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

	wsA := provision("dedup-a")
	wsB := provision("dedup-b")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	resolver := &workspace.PoolResolver{Pool: pool}
	handler := &crm.Handler{Pool: pool, Logger: logger}

	router := chi.NewRouter()
	router.Group(func(r chi.Router) {
		r.Use(workspace.Middleware(logger, resolver, pipelineDomainTLD))
		handler.RegisterRoutes(r)
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

// helpers for dedup tests.

func dedupCreateContact(t *testing.T, env *pipelineTestEnv, ws workspaceFixture, first, last, email string) string {
	t.Helper()
	body := map[string]any{"first_name": first, "last_name": last, "email": email}
	if email == "" {
		body["email"] = nil
	}
	status, resp := env.doJSON(t, ws, http.MethodPost, "/v1/contacts", body)
	if status != http.StatusCreated {
		t.Fatalf("createContact %s %s: status=%d body=%s", first, last, status, resp)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		t.Fatalf("decode contact: %v", err)
	}
	return out.ID
}

func dedupCreateCompany(t *testing.T, env *pipelineTestEnv, ws workspaceFixture, name, domain string) string {
	t.Helper()
	body := map[string]any{"name": name}
	if domain != "" {
		body["domain"] = domain
	}
	status, resp := env.doJSON(t, ws, http.MethodPost, "/v1/companies", body)
	if status != http.StatusCreated {
		t.Fatalf("createCompany %s: status=%d body=%s", name, status, resp)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		t.Fatalf("decode company: %v", err)
	}
	return out.ID
}

type dedupContactPairJSON struct {
	A      map[string]any `json:"a"`
	B      map[string]any `json:"b"`
	Reason string         `json:"reason"`
	Score  float64        `json:"score"`
}

type dedupContactsResp struct {
	Pairs []dedupContactPairJSON `json:"pairs"`
}

type dedupCompanyPairJSON struct {
	A      map[string]any `json:"a"`
	B      map[string]any `json:"b"`
	Reason string         `json:"reason"`
	Score  float64        `json:"score"`
}

type dedupCompaniesResp struct {
	Pairs []dedupCompanyPairJSON `json:"pairs"`
}

func listContactDups(t *testing.T, env *pipelineTestEnv, ws workspaceFixture) dedupContactsResp {
	t.Helper()
	status, body := env.doJSON(t, ws, http.MethodGet, "/v1/dedup/contacts", nil)
	if status != http.StatusOK {
		t.Fatalf("list contact dups: status=%d body=%s", status, body)
	}
	var out dedupContactsResp
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v body=%s", err, body)
	}
	return out
}

func listCompanyDups(t *testing.T, env *pipelineTestEnv, ws workspaceFixture) dedupCompaniesResp {
	t.Helper()
	status, body := env.doJSON(t, ws, http.MethodGet, "/v1/dedup/companies", nil)
	if status != http.StatusOK {
		t.Fatalf("list company dups: status=%d body=%s", status, body)
	}
	var out dedupCompaniesResp
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v body=%s", err, body)
	}
	return out
}

func countInSchema(t *testing.T, pool *pgxpool.Pool, schema, table string) int {
	t.Helper()
	var n int
	q := fmt.Sprintf(`SELECT count(*) FROM %q.%q`, schema, table)
	if err := pool.QueryRow(context.Background(), q).Scan(&n); err != nil {
		t.Fatalf("count %s.%s: %v", schema, table, err)
	}
	return n
}

// --- tests ---

func TestDedup_Contacts_ExactEmailMatch(t *testing.T) {
	env := setupDedupEnv(t)

	// Two contacts with the same email (different cases) — must be flagged.
	dedupCreateContact(t, env, env.wsA, "Alice", "Smith", "alice@example.com")
	dedupCreateContact(t, env, env.wsA, "Alicia", "Smyth", "ALICE@EXAMPLE.COM")

	pairs := listContactDups(t, env, env.wsA).Pairs
	if len(pairs) == 0 {
		t.Fatal("expected at least one duplicate pair, got none")
	}
	for _, p := range pairs {
		if p.Reason == "exact_email" {
			return
		}
	}
	t.Errorf("expected reason=exact_email, got pairs=%+v", pairs)
}

func TestDedup_Contacts_FuzzyNameMatch(t *testing.T) {
	env := setupDedupEnv(t)

	// Very similar names, no email — should be caught by fuzzy matching.
	dedupCreateContact(t, env, env.wsA, "Jean", "Dupont", "")
	dedupCreateContact(t, env, env.wsA, "Jean", "Dupond", "")

	pairs := listContactDups(t, env, env.wsA).Pairs
	if len(pairs) == 0 {
		t.Fatal("expected at least one fuzzy-name pair, got none")
	}
	for _, p := range pairs {
		if p.Reason == "similar_name" {
			return
		}
	}
	t.Errorf("expected reason=similar_name, got pairs=%+v", pairs)
}

func TestDedup_Contacts_NoMergeExcludesFromList(t *testing.T) {
	env := setupDedupEnv(t)

	idA := dedupCreateContact(t, env, env.wsA, "Jean", "Dupont", "")
	idB := dedupCreateContact(t, env, env.wsA, "Jean", "Dupond", "")

	// Verify duplicate is detected before marking distinct.
	pairs := listContactDups(t, env, env.wsA).Pairs
	if len(pairs) == 0 {
		t.Fatal("expected pair before marking distinct")
	}

	// Mark as distinct.
	status, _ := env.doJSON(t, env.wsA, http.MethodPost, "/v1/dedup/contacts/distinct",
		map[string]string{"id_a": idA, "id_b": idB})
	if status != http.StatusOK {
		t.Fatalf("mark distinct: status=%d", status)
	}

	// Should no longer appear.
	pairs = listContactDups(t, env, env.wsA).Pairs
	for _, p := range pairs {
		aID := fmt.Sprint(p.A["id"])
		bID := fmt.Sprint(p.B["id"])
		if (aID == idA && bID == idB) || (aID == idB && bID == idA) {
			t.Errorf("pair should be excluded after mark-distinct, but still present: %+v", p)
		}
	}
}

func TestDedup_Contacts_MergeRepointsRelations(t *testing.T) {
	env := setupDedupEnv(t)

	survivorID := dedupCreateContact(t, env, env.wsA, "Jean", "Dupont", "jean@example.com")
	loserID := dedupCreateContact(t, env, env.wsA, "Jean", "Dupond", "jean.old@example.com")

	// Create a deal linked to the loser.
	dStatus, dBody := env.doJSON(t, env.wsA, http.MethodPost, "/v1/deals", map[string]any{
		"title":      "Test Deal",
		"contact_id": loserID,
	})
	if dStatus != http.StatusCreated {
		t.Fatalf("create deal: status=%d body=%s", dStatus, dBody)
	}
	var dealOut struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(dBody, &dealOut); err != nil {
		t.Fatalf("decode deal: %v", err)
	}

	// Merge loser into survivor.
	status, body := env.doJSON(t, env.wsA, http.MethodPost, "/v1/dedup/contacts/merge", map[string]any{
		"survivor_id": survivorID,
		"loser_id":    loserID,
		"fields":      map[string]string{"email": "survivor"},
	})
	if status != http.StatusOK {
		t.Fatalf("merge: status=%d body=%s", status, body)
	}

	// Loser should be deleted.
	gStatus, _ := env.doJSON(t, env.wsA, http.MethodGet, "/v1/contacts/"+loserID, nil)
	if gStatus != http.StatusNotFound {
		t.Errorf("loser should be 404 after merge, got %d", gStatus)
	}

	// Deal should now point to survivor.
	gStatus, gBody := env.doJSON(t, env.wsA, http.MethodGet, "/v1/deals/"+dealOut.ID, nil)
	if gStatus != http.StatusOK {
		t.Fatalf("get deal: status=%d body=%s", gStatus, gBody)
	}
	var dj map[string]any
	_ = json.Unmarshal(gBody, &dj)
	if dj["contact_id"] != survivorID {
		t.Errorf("deal.contact_id: got %v want %s", dj["contact_id"], survivorID)
	}
}

func TestDedup_Contacts_MergeEmitsAuditEvent(t *testing.T) {
	env := setupDedupEnv(t)

	survivorID := dedupCreateContact(t, env, env.wsA, "Alice", "Smith", "alice@example.com")
	loserID := dedupCreateContact(t, env, env.wsA, "Alicia", "Smyth", "ALICE@EXAMPLE.COM")

	before := countInSchema(t, env.pool, "core", "audit_log")

	status, body := env.doJSON(t, env.wsA, http.MethodPost, "/v1/dedup/contacts/merge", map[string]any{
		"survivor_id": survivorID,
		"loser_id":    loserID,
	})
	if status != http.StatusOK {
		t.Fatalf("merge: status=%d body=%s", status, body)
	}

	after := countInSchema(t, env.pool, "core", "audit_log")
	if after <= before {
		t.Errorf("expected new audit_log row after merge, count before=%d after=%d", before, after)
	}

	// Verify the audit event.
	var event, payload string
	err := env.pool.QueryRow(context.Background(),
		`SELECT event, payload::text FROM core.audit_log WHERE event='contact.merged' ORDER BY id DESC LIMIT 1`,
	).Scan(&event, &payload)
	if err != nil {
		t.Fatalf("get audit row: %v", err)
	}
	if event != "contact.merged" {
		t.Errorf("event: got %s want contact.merged", event)
	}
	if !containsString(payload, survivorID) || !containsString(payload, loserID) {
		t.Errorf("audit payload should contain both IDs, got: %s", payload)
	}
}

func TestDedup_Companies_ExactDomainMatch(t *testing.T) {
	env := setupDedupEnv(t)

	dedupCreateCompany(t, env, env.wsA, "Acme Corp", "acme.com")
	dedupCreateCompany(t, env, env.wsA, "ACME Corporation", "ACME.COM")

	pairs := listCompanyDups(t, env, env.wsA).Pairs
	if len(pairs) == 0 {
		t.Fatal("expected at least one company duplicate pair")
	}
	for _, p := range pairs {
		if p.Reason == "exact_domain" {
			return
		}
	}
	t.Errorf("expected reason=exact_domain, got pairs=%+v", pairs)
}

func TestDedup_Companies_MergeRepointsContacts(t *testing.T) {
	env := setupDedupEnv(t)

	survivorCoID := dedupCreateCompany(t, env, env.wsA, "Acme Corp", "acme.com")
	loserCoID := dedupCreateCompany(t, env, env.wsA, "ACME Corporation", "acme.com")

	// Create a contact linked to the loser company.
	cStatus, cBody := env.doJSON(t, env.wsA, http.MethodPost, "/v1/contacts", map[string]any{
		"first_name": "Bob",
		"last_name":  "Jones",
		"company_id": loserCoID,
	})
	if cStatus != http.StatusCreated {
		t.Fatalf("create contact: status=%d body=%s", cStatus, cBody)
	}
	var cOut struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(cBody, &cOut)

	// Merge loser into survivor.
	status, body := env.doJSON(t, env.wsA, http.MethodPost, "/v1/dedup/companies/merge", map[string]any{
		"survivor_id": survivorCoID,
		"loser_id":    loserCoID,
	})
	if status != http.StatusOK {
		t.Fatalf("merge companies: status=%d body=%s", status, body)
	}

	// Contact should now point to the survivor company.
	gStatus, gBody := env.doJSON(t, env.wsA, http.MethodGet, "/v1/contacts/"+cOut.ID, nil)
	if gStatus != http.StatusOK {
		t.Fatalf("get contact: status=%d body=%s", gStatus, gBody)
	}
	var cj map[string]any
	_ = json.Unmarshal(gBody, &cj)
	if cj["company_id"] != survivorCoID {
		t.Errorf("contact.company_id: got %v want %s", cj["company_id"], survivorCoID)
	}
}

func TestDedup_CrossTenantIsolation(t *testing.T) {
	env := setupDedupEnv(t)

	// Create duplicate contacts in workspace A only.
	dedupCreateContact(t, env, env.wsA, "Alice", "Smith", "alice@example.com")
	dedupCreateContact(t, env, env.wsA, "Alice", "Smyth", "alice@example.com")

	// Workspace B should see no duplicates.
	pairsB := listContactDups(t, env, env.wsB).Pairs
	if len(pairsB) != 0 {
		t.Errorf("workspace B should see 0 pairs (cross-tenant leak), got %d", len(pairsB))
	}
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
