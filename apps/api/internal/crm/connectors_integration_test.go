//go:build integration

// Integration tests for the connector event-ingestion endpoint
// (Sprint 9, ADR-011). Exercises the full path: workspace resolution,
// service-token bearer auth + connector.push_events scope, envelope
// validation, event → CRM mutation, idempotency replay, and
// cross-tenant isolation against a real Postgres.
//
// Run:
//
//	~/.local/go/bin/go -C apps/api test -tags integration -count 1 -race -v \
//	    -run TestConnector ./internal/crm

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
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	"github.com/gbconsult/lecrm/apps/api/internal/crm"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

const connectorDomainTLD = "lecrm.test"

type connectorTestEnv struct {
	pool          *pgxpool.Pool
	srv           *httptest.Server
	wsA           workspaceFixture
	wsB           workspaceFixture
	tokenA        string // connector.push_events token for wsA
	noScopeTokenA string // a token WITHOUT the connector scope (wsA)
}

func setupConnectorEnv(t *testing.T) *connectorTestEnv {
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
			pipelineMigrationPath(t, "0016_service_tokens.sql"),
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
	wsA := provision("acme-a")
	wsB := provision("acme-b")

	tokenStore := &auth.PgServiceTokenStore{Pool: pool}
	mkToken := func(ws workspaceFixture, scopes []string) string {
		created, err := tokenStore.Create(ctx, ws.id, ws.slug, auth.CreateServiceTokenInput{
			Name:      "test-" + scopes[0],
			ActorType: "connector",
			Scopes:    scopes,
		})
		if err != nil {
			t.Fatalf("create token for %s: %v", ws.slug, err)
		}
		return created.Plaintext
	}
	tokenA := mkToken(wsA, []string{crm.ConnectorPushScope})
	noScopeTokenA := mkToken(wsA, []string{"crm.read"})

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	resolver := &workspace.PoolResolver{Pool: pool}
	bearerAuth := &auth.HTTPBearerAuthenticator{Loader: tokenStore}
	handler := &crm.Handler{Pool: pool, Logger: logger}

	router := chi.NewRouter()
	router.Group(func(r chi.Router) {
		r.Use(workspace.MiddlewareWithBearer(logger, resolver, connectorDomainTLD, bearerAuth))
		r.Group(func(r chi.Router) {
			r.Use(crm.RequireConnectorScope)
			handler.RegisterConnectorRoutes(r)
		})
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return &connectorTestEnv{
		pool: pool, srv: srv, wsA: wsA, wsB: wsB,
		tokenA: tokenA, noScopeTokenA: noScopeTokenA,
	}
}

// post sends a connector event to the given source for the workspace
// identified by `host` (subdomain) with the supplied bearer token and
// idempotency-carrying envelope. Returns status, the Idempotency-Replayed
// header, and the body.
func (e *connectorTestEnv) post(t *testing.T, slug, token, source string, envelope any) (int, string, []byte) {
	t.Helper()
	b, _ := json.Marshal(envelope)
	req, err := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/connectors/"+source+"/events", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = slug + "." + connectorDomainTLD
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Idempotency-Replayed"), body
}

func (e *connectorTestEnv) contactCountByEmail(t *testing.T, ws workspaceFixture, email string) int {
	t.Helper()
	q := fmt.Sprintf(`SELECT count(*) FROM %q.contacts WHERE email = $1`, ws.roleName)
	var n int
	if err := e.pool.QueryRow(context.Background(), q, email).Scan(&n); err != nil {
		t.Fatalf("count contacts: %v", err)
	}
	return n
}

func (e *connectorTestEnv) contactIDByEmail(t *testing.T, ws workspaceFixture, email string) uuid.UUID {
	t.Helper()
	q := fmt.Sprintf(`SELECT id FROM %q.contacts WHERE email = $1`, ws.roleName)
	var id uuid.UUID
	if err := e.pool.QueryRow(context.Background(), q, email).Scan(&id); err != nil {
		t.Fatalf("contact id: %v", err)
	}
	return id
}

func (e *connectorTestEnv) customProps(t *testing.T, ws workspaceFixture, parentType string, parentID uuid.UUID) map[string]any {
	t.Helper()
	q := fmt.Sprintf(`SELECT data FROM %q.objects WHERE object_type='custom_properties' AND parent_type=$1 AND parent_id=$2`, ws.roleName)
	var raw map[string]any
	if err := e.pool.QueryRow(context.Background(), q, parentType, parentID).Scan(&raw); err != nil {
		t.Fatalf("custom props: %v", err)
	}
	return raw
}

func (e *connectorTestEnv) dealByExternalID(t *testing.T, ws workspaceFixture, source, externalID string) (uuid.UUID, string, bool) {
	t.Helper()
	q := fmt.Sprintf(`
		SELECT d.id, s.name, d.closed_at IS NOT NULL
		  FROM %q.external_entity_mappings m
		  JOIN %q.deals d ON d.id = m.entity_id
		  LEFT JOIN %q.pipeline_stages s ON s.id = d.stage_id
		 WHERE m.provider_id=$1 AND m.external_id=$2 AND m.entity_type='deal'`,
		ws.roleName, ws.roleName, ws.roleName)
	var id uuid.UUID
	var stage string
	var closed bool
	if err := e.pool.QueryRow(context.Background(), q, source, externalID).Scan(&id, &stage, &closed); err != nil {
		t.Fatalf("deal by external id: %v", err)
	}
	return id, stage, closed
}

func (e *connectorTestEnv) dealActivityCount(t *testing.T, ws workspaceFixture, dealID uuid.UUID) int {
	t.Helper()
	q := fmt.Sprintf(`SELECT count(*) FROM %q.activities WHERE entity_type='deal' AND entity_id=$1 AND actor_type='connector'`, ws.roleName)
	var n int
	if err := e.pool.QueryRow(context.Background(), q, dealID).Scan(&n); err != nil {
		t.Fatalf("activity count: %v", err)
	}
	return n
}

// --- tests ---

func candidateEnvelope(idemKey, url, email string) map[string]any {
	return map[string]any{
		"event":           "candidate.enriched",
		"source":          "chatboting",
		"idempotency_key": idemKey,
		"workspace":       "acme-a",
		"payload": map[string]any{
			"candidate": map[string]any{
				"url": url, "email": email,
				"first_name": "Ada", "last_name": "Lovelace",
				"score": 87, "cms": "WordPress", "geo": "FR-75", "category": "restaurant",
			},
		},
	}
}

func invitationEnvelope(event, idemKey, invID string) map[string]any {
	return map[string]any{
		"event":           event,
		"source":          "chatboting",
		"idempotency_key": idemKey,
		"workspace":       "acme-a",
		"payload": map[string]any{
			"invitation": map[string]any{
				"id": invID, "title": "Invite " + invID,
				"candidate_email": "ada@example.com", "tenant_url": "https://t.example.com/" + invID,
			},
		},
	}
}

func TestConnector_CandidateEnriched_CreatesContactWithProperties(t *testing.T) {
	e := setupConnectorEnv(t)
	status, _, body := e.post(t, "acme-a", e.tokenA, "chatboting",
		candidateEnvelope("k-enrich-1", "https://site.example.com/ada", "ada@example.com"))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if got := e.contactCountByEmail(t, e.wsA, "ada@example.com"); got != 1 {
		t.Fatalf("contacts with email: got %d want 1", got)
	}
	id := e.contactIDByEmail(t, e.wsA, "ada@example.com")
	props := e.customProps(t, e.wsA, "contact", id)
	for _, k := range []string{"score", "cms", "geo", "category"} {
		if _, ok := props[k]; !ok {
			t.Errorf("custom property %q missing: %v", k, props)
		}
	}
	if props["cms"] != "WordPress" {
		t.Errorf("cms = %v, want WordPress", props["cms"])
	}
}

func TestConnector_InvitationClaimed_MovesDealToClosedWonAndCreatesActivity(t *testing.T) {
	e := setupConnectorEnv(t)
	// created → deal at Discovery
	if st, _, b := e.post(t, "acme-a", e.tokenA, "chatboting",
		invitationEnvelope("invitation.created", "k-created-1", "inv-1")); st != http.StatusOK {
		t.Fatalf("created: status=%d body=%s", st, b)
	}
	_, stage, _ := e.dealByExternalID(t, e.wsA, "chatboting", "inv-1")
	if stage != "Discovery" {
		t.Fatalf("stage after created = %q, want Discovery", stage)
	}
	// claimed → Closed-Won (resolves to combined Closed-Won/Lost), closed_at set
	if st, _, b := e.post(t, "acme-a", e.tokenA, "chatboting",
		invitationEnvelope("invitation.claimed", "k-claimed-1", "inv-1")); st != http.StatusOK {
		t.Fatalf("claimed: status=%d body=%s", st, b)
	}
	dealID, stage, closed := e.dealByExternalID(t, e.wsA, "chatboting", "inv-1")
	if stage != "Closed-Won/Lost" {
		t.Fatalf("stage after claimed = %q, want Closed-Won/Lost", stage)
	}
	if !closed {
		t.Fatal("closed_at should be set after claim")
	}
	if got := e.dealActivityCount(t, e.wsA, dealID); got < 2 {
		t.Fatalf("connector activities = %d, want >= 2 (created + claimed)", got)
	}
}

func TestConnector_Idempotency_DuplicateKeyNoDuplicateEntities(t *testing.T) {
	e := setupConnectorEnv(t)
	env := candidateEnvelope("k-dup-1", "https://site.example.com/dup", "dup@example.com")

	st1, replay1, _ := e.post(t, "acme-a", e.tokenA, "chatboting", env)
	if st1 != http.StatusOK || replay1 == "true" {
		t.Fatalf("first delivery: status=%d replay=%q", st1, replay1)
	}
	st2, replay2, _ := e.post(t, "acme-a", e.tokenA, "chatboting", env)
	if st2 != http.StatusOK {
		t.Fatalf("duplicate delivery status=%d, want 200", st2)
	}
	if replay2 != "true" {
		t.Fatalf("duplicate should be replayed (Idempotency-Replayed=true), got %q", replay2)
	}
	if got := e.contactCountByEmail(t, e.wsA, "dup@example.com"); got != 1 {
		t.Fatalf("duplicate idempotency key created %d contacts, want 1", got)
	}
}

func TestConnector_InvalidEventType_400(t *testing.T) {
	e := setupConnectorEnv(t)
	st, _, _ := e.post(t, "acme-a", e.tokenA, "chatboting", map[string]any{
		"event": "candidate.detonated", "idempotency_key": "k-bad", "workspace": "acme-a",
	})
	if st != http.StatusBadRequest {
		t.Fatalf("invalid event type status=%d, want 400", st)
	}
}

func TestConnector_WrongScopeForbidden(t *testing.T) {
	e := setupConnectorEnv(t)
	st, _, _ := e.post(t, "acme-a", e.noScopeTokenA, "chatboting",
		candidateEnvelope("k-scope", "https://x", "x@example.com"))
	if st != http.StatusForbidden {
		t.Fatalf("wrong-scope token status=%d, want 403", st)
	}
}

func TestConnector_NoAuth401(t *testing.T) {
	e := setupConnectorEnv(t)
	st, _, _ := e.post(t, "acme-a", "", "chatboting",
		candidateEnvelope("k-noauth", "https://x", "x@example.com"))
	if st != http.StatusUnauthorized {
		t.Fatalf("no-auth status=%d, want 401", st)
	}
}

func TestConnector_CrossTenantWorkspaceMismatchRejected(t *testing.T) {
	e := setupConnectorEnv(t)
	// Token for acme-a, but the envelope declares workspace acme-b.
	env := candidateEnvelope("k-xtenant", "https://x", "x@example.com")
	env["workspace"] = "acme-b"
	st, _, _ := e.post(t, "acme-a", e.tokenA, "chatboting", env)
	if st != http.StatusForbidden {
		t.Fatalf("cross-tenant envelope status=%d, want 403", st)
	}
	// And the wsA token cannot be used against the acme-b subdomain at all
	// (bearer middleware rejects the slug mismatch).
	st2, _, _ := e.post(t, "acme-b", e.tokenA, "chatboting",
		candidateEnvelope("k-xtenant-2", "https://x", "x@example.com"))
	if st2 != http.StatusUnauthorized {
		t.Fatalf("wsA token on acme-b status=%d, want 401", st2)
	}
}
