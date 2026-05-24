//go:build integration

// Package tenantpair provides the cross-tenant isolation test fixture required
// by docs/test-strategy.md §4.1 (non-negotiable category (a)).
//
// Usage:
//
//	pair := tenantpair.Provision(t)
//	resp, _ := pair.A.Client().Get(pair.URL() + "/v1/_test/workspaces")
//	// assert resp.workspace.slug == pair.A.Slug, not pair.B.Slug
//
// The package spins up testcontainers Postgres, applies 0001_init.sql,
// provisions two isolated workspaces, and starts a shared httptest.Server
// with the workspace middleware wired. Every t.Cleanup is registered so
// the container and server stop automatically at test exit.
package tenantpair

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

const domainTLD = "lecrm.test"

// Pair holds two provisioned workspaces sharing a single HTTP test server.
// Both tenants hit the same server; the workspace middleware dispatches
// on the Host header set by each Tenant's Client.
type Pair struct {
	A             *Tenant
	B             *Tenant
	srv           *httptest.Server
	baseTransport http.RoundTripper // direct route to srv without Host override
}

// URL returns the base URL of the shared HTTP test server.
func (p *Pair) URL() string { return p.srv.URL }

// ClientWithHost returns an HTTP client that routes to the pair's test server
// with the given Host header. Use this for boundary tests (unknown slugs,
// missing subdomain) where the built-in A/B tenants don't apply.
func (p *Pair) ClientWithHost(host string) *http.Client {
	return &http.Client{
		Transport: &tenantTransport{base: p.baseTransport, host: host},
		Timeout:   10 * time.Second,
	}
}

// Tenant is one provisioned workspace within a Pair.
type Tenant struct {
	ID       uuid.UUID
	Slug     string
	RoleName string
	client   *http.Client
	pool     *pgxpool.Pool
}

// Client returns an HTTP client scoped to this tenant. Every request
// sent through this client carries Host: <Slug>.lecrm.test so the
// workspace middleware resolves the correct workspace.
func (t *Tenant) Client() *http.Client { return t.client }

// DB returns a pgxpool.Pool connected to the testcontainers Postgres as
// the postgres superuser. Use it to seed fixture data or assert DB state.
// The pool is NOT switched to the workspace role; set search_path explicitly
// when testing role-level isolation.
func (t *Tenant) DB() *pgxpool.Pool { return t.pool }

// tenantTransport rewrites Host on every outbound request so the workspace
// middleware receives the correct subdomain for this tenant.
type tenantTransport struct {
	base http.RoundTripper
	host string // e.g. "acme-a.lecrm.test"
}

func (tt *tenantTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Host = tt.host
	return tt.base.RoundTrip(r)
}

// Provision starts a testcontainers Postgres 17 instance, applies
// 0001_init.sql, provisions two isolated workspaces ("acme-a" and "acme-b"),
// and starts a shared httptest.Server with the workspace middleware and
// TestListHandler wired. Cleanup is registered on t.
//
// The test is skipped automatically when Docker is not reachable
// (testcontainers.SkipIfProviderIsNotHealthy).
func Provision(t *testing.T) *Pair {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	initSQL := initSQLPath(t)

	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("lecrm"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.WithInitScripts(initSQL),
	)
	if err != nil {
		t.Fatalf("tenantpair: start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := ctr.Terminate(context.Background()); err != nil {
			t.Logf("tenantpair: terminate container: %v", err)
		}
	})

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("tenantpair: connection string: %v", err)
	}

	// Provision two workspaces using a single superuser connection.
	// Postgres can briefly reset connections while applying init scripts and
	// restarting; retry for up to 15 s before giving up.
	conn, err := connectWithRetry(ctx, connStr, 15*time.Second)
	if err != nil {
		t.Fatalf("tenantpair: connect: %v", err)
	}

	idA, roleA, err := provisionWorkspace(ctx, conn, "acme-a")
	if err != nil {
		t.Fatalf("tenantpair: provision acme-a: %v", err)
	}
	idB, roleB, err := provisionWorkspace(ctx, conn, "acme-b")
	if err != nil {
		t.Fatalf("tenantpair: provision acme-b: %v", err)
	}
	_ = conn.Close(ctx)

	// Shared superuser pool — used by the HTTP handlers and by test DB
	// assertions (e.g. to seed or read fixture data).
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("tenantpair: pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	// Build a minimal chi router with the workspace middleware wired.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	resolver := &workspace.PoolResolver{Pool: pool}
	testList := &workspace.TestListHandler{Pool: pool, Logger: logger}

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(workspace.Middleware(logger, resolver, domainTLD))
		r.Get("/v1/_test/workspaces", testList.ServeHTTP)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	baseTransport := srv.Client().Transport
	makeClient := func(slug string) *http.Client {
		return &http.Client{
			Transport: &tenantTransport{
				base: baseTransport,
				host: slug + "." + domainTLD,
			},
			Timeout: 10 * time.Second,
		}
	}

	tenantA := &Tenant{ID: idA, Slug: "acme-a", RoleName: roleA, client: makeClient("acme-a"), pool: pool}
	tenantB := &Tenant{ID: idB, Slug: "acme-b", RoleName: roleB, client: makeClient("acme-b"), pool: pool}

	return &Pair{A: tenantA, B: tenantB, srv: srv, baseTransport: baseTransport}
}

// connectWithRetry retries pgx.Connect until it succeeds or the deadline
// passes. Postgres briefly resets connections while processing init scripts
// and restarting after them; retrying is more reliable than a fixed sleep.
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
		// Brief pause; avoid hammering a starting postgres.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// provisionWorkspace calls the core.lecrm_provision_workspace SQL function
// (SECURITY DEFINER, defined in 0001_init.sql) and upserts a row in
// core.workspaces. The connection must be a Postgres superuser or the
// lecrm_provisioner role.
func provisionWorkspace(ctx context.Context, conn *pgx.Conn, slug string) (uuid.UUID, string, error) {
	id := uuid.New()

	var roleName string
	if err := conn.QueryRow(ctx,
		"SELECT core.lecrm_provision_workspace($1)", id).Scan(&roleName); err != nil {
		return uuid.Nil, "", fmt.Errorf("lecrm_provision_workspace(%s): %w", id, err)
	}

	_, err := conn.Exec(ctx, `
		INSERT INTO core.workspaces (id, slug, role_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (slug) DO UPDATE SET
			role_name  = EXCLUDED.role_name,
			updated_at = now()
	`, id, slug, roleName)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("upsert workspace %q: %w", slug, err)
	}
	return id, roleName, nil
}

// initSQLPath returns the absolute path to 0001_init.sql by navigating
// from this source file to the repo root.
func initSQLPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile is apps/api/internal/testfixtures/tenantpair/tenantpair.go
	// Five levels up reaches the repo root (leCRM/).
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile),
		"..", "..", "..", "..", ".."))
	p := filepath.Join(repoRoot, "packages", "db", "migrations", "0001_init.sql")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("tenantpair: init SQL not found at %s: %v", p, err)
	}
	return p
}
