//go:build integration

// End-to-end RBAC regression suite — the role × endpoint matrix required by
// docs/test-strategy.md non-negotiable category (b). Spins up a real
// Postgres (all migrations), provisions a workspace, seeds member/admin/
// owner users, mints real V2 session cookies, and drives the production
// middleware chain (workspace → rbac.Resolve → RequireRole) in front of the
// real CRM and member-management handlers.
//
// Run:
//
//	go -C apps/api test -tags integration -count 1 -race -run TestRBAC ./internal/rbac
package rbac_test

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
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	"github.com/gbconsult/lecrm/apps/api/internal/crm"
	"github.com/gbconsult/lecrm/apps/api/internal/members"
	"github.com/gbconsult/lecrm/apps/api/internal/rbac"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

const (
	rbacDomainTLD = "lecrm.test"
	rbacSlug      = "acme"
)

var rbacSecret = bytes.Repeat([]byte("k"), 32)

func migrationsList() []string {
	return []string{
		"0001_init.sql", "0002_identity.sql", "0003_metadata_engine.sql",
		"0004_workspaces_admin_email_registry.sql", "0005_slug_tombstoning.sql",
		"0006_security_definer_hardening.sql", "0007_session_revocations.sql",
		"0008_crm_entities.sql", "0009_metadata_json_type.sql",
		"0010_pgcrypto_to_core_schema.sql", "0011_external_sync.sql",
		"0012_email_suppression.sql", "0013_workspace_ro_role.sql",
		"0014_idempotency_keys.sql", "0015_activities_notes_tasks.sql",
		"0016_service_tokens.sql",
	}
}

func migrationPath(t *testing.T, filename string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile: apps/api/internal/rbac/rbac_integration_test.go → repo root is 5 up.
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	p := filepath.Join(repoRoot, "packages", "db", "migrations", filename)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("migration %s not found at %s: %v", filename, p, err)
	}
	return p
}

type rbacEnv struct {
	srv      *httptest.Server
	pool     *pgxpool.Pool
	wsID     uuid.UUID
	roleName string
	// seeded user IDs per role
	memberID uuid.UUID
	adminID  uuid.UUID
	ownerID  uuid.UUID
}

func setupRBACEnv(t *testing.T) *rbacEnv {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	var scripts []string
	for _, m := range migrationsList() {
		scripts = append(scripts, migrationPath(t, m))
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

	// Provision the workspace + its per-tenant role.
	wsID := uuid.New()
	var roleName string
	if err := pool.QueryRow(ctx,
		"SELECT core.lecrm_provision_workspace_with_registry($1, $2, $3, $4, $5)",
		wsID, rbacSlug, "admin@"+rbacSlug+".test", "creator@"+rbacSlug+".test", "gbconsult-default",
	).Scan(&roleName); err != nil {
		t.Fatalf("provision workspace: %v", err)
	}

	env := &rbacEnv{pool: pool, wsID: wsID, roleName: roleName}
	env.memberID = env.seedUser(t, ctx, "member@acme.test", "member")
	env.adminID = env.seedUser(t, ctx, "admin@acme.test", "admin")
	env.ownerID = env.seedUser(t, ctx, "owner@acme.test", "owner")

	// Build the production-shaped router.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	resolver := &workspace.PoolResolver{Pool: pool}
	crmH := &crm.Handler{Pool: pool, Logger: logger}
	memberStore := &members.PgMemberStore{Pool: pool}
	decode := func(r *http.Request, slug string) (auth.Session, bool) {
		s, _, ok := auth.SessionFromRequestV2(r, slug, rbacSecret)
		return s, ok
	}
	rbacResolver := &rbac.Resolver{Store: memberStore, DecodeSession: decode, Logger: logger}
	membersH := &members.Handler{Store: memberStore, DecodeSession: decode, Logger: logger}

	router := chi.NewRouter()
	router.Group(func(r chi.Router) {
		r.Use(workspace.Middleware(logger, resolver, rbacDomainTLD))
		r.Group(func(r chi.Router) {
			r.Use(rbacResolver.Resolve)
			r.Use(rbac.RequireRoleByMethod(rbac.RoleMember, rbac.RoleAdmin))
			crmH.RegisterRoutes(r)
			crmH.RegisterANTRoutes(r)
		})
		r.Group(func(r chi.Router) {
			r.Use(rbacResolver.Resolve)
			r.Use(rbac.RequireRole(rbac.RoleMember))
			membersH.RegisterMeRoute(r)
		})
		r.Group(func(r chi.Router) {
			r.Use(rbacResolver.Resolve)
			r.Use(rbac.RequireRole(rbac.RoleOwner))
			membersH.RegisterRoutes(r)
		})
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	env.srv = srv
	return env
}

func (e *rbacEnv) seedUser(t *testing.T, ctx context.Context, email, role string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.pool.QueryRow(ctx, `
		INSERT INTO core.users (issuer, subject, email)
		VALUES ('test-idp', $1, $1)
		RETURNING id
	`, email).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	if _, err := e.pool.Exec(ctx, `
		INSERT INTO core.workspace_members (workspace_id, user_id, role, invited_at, joined_at)
		VALUES ($1, $2, $3, now(), now())
	`, e.wsID, id, role); err != nil {
		t.Fatalf("seed membership %s: %v", email, err)
	}
	return id
}

// cookie mints a real V2 session cookie for userID bound to the workspace.
func (e *rbacEnv) cookie(t *testing.T, userID uuid.UUID) *http.Cookie {
	t.Helper()
	token, err := auth.EncodeSessionV2(auth.Session{UserID: userID, WorkspaceID: e.wsID}, rbacSlug, rbacSecret)
	if err != nil {
		t.Fatalf("encode session: %v", err)
	}
	return &http.Cookie{Name: auth.SessionCookieName, Value: token}
}

// req issues a request as the given user (nil userID = anonymous).
func (e *rbacEnv) req(t *testing.T, userID *uuid.UUID, method, path, body string) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = bytes.NewReader([]byte(body))
	}
	r, err := http.NewRequest(method, e.srv.URL+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	r.Host = rbacSlug + "." + rbacDomainTLD
	r.Header.Set("Content-Type", "application/json")
	if userID != nil {
		r.AddCookie(e.cookie(t, *userID))
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func TestRBAC_RoleEndpointMatrix(t *testing.T) {
	env := setupRBACEnv(t)
	m, a, o := env.memberID, env.adminID, env.ownerID

	cases := []struct {
		name   string
		user   *uuid.UUID
		method string
		path   string
		body   string
		want   int
	}{
		// Reads: member+ (anonymous denied).
		{"anon GET contacts", nil, "GET", "/v1/contacts", "", 401},
		{"member GET contacts", &m, "GET", "/v1/contacts", "", 200},
		{"admin GET contacts", &a, "GET", "/v1/contacts", "", 200},
		{"owner GET contacts", &o, "GET", "/v1/contacts", "", 200},
		{"member GET companies", &m, "GET", "/v1/companies", "", 200},
		{"member GET deals", &m, "GET", "/v1/deals", "", 200},
		{"member GET pipeline stages", &m, "GET", "/v1/pipeline/stages", "", 200},
		{"member GET tasks", &m, "GET", "/v1/tasks", "", 200},

		// Writes: admin+ (member denied with 403).
		{"member POST contact → 403", &m, "POST", "/v1/contacts", `{"first_name":"A","last_name":"B"}`, 403},
		{"admin POST contact → 201", &a, "POST", "/v1/contacts", `{"first_name":"A","last_name":"B"}`, 201},
		{"owner POST contact → 201", &o, "POST", "/v1/contacts", `{"first_name":"C","last_name":"D"}`, 201},
		{"member POST company → 403", &m, "POST", "/v1/companies", `{"name":"Acme"}`, 403},
		{"admin POST company → 201", &a, "POST", "/v1/companies", `{"name":"Acme"}`, 201},
		{"member POST deal → 403", &m, "POST", "/v1/deals", `{"title":"Big"}`, 403},
		{"admin POST deal → 201", &a, "POST", "/v1/deals", `{"title":"Big"}`, 201},

		// Member-management: owner only.
		{"anon GET members → 401", nil, "GET", "/v1/workspace/members", "", 401},
		{"member GET members → 403", &m, "GET", "/v1/workspace/members", "", 403},
		{"admin GET members → 403", &a, "GET", "/v1/workspace/members", "", 403},
		{"owner GET members → 200", &o, "GET", "/v1/workspace/members", "", 200},
		{"member invite → 403", &m, "POST", "/v1/workspace/members/invite", `{"email":"x@y.com"}`, 403},
		{"owner invite → 201", &o, "POST", "/v1/workspace/members/invite", `{"email":"new@acme.test","role":"member"}`, 201},

		// Self-service: any member.
		{"anon me → 401", nil, "GET", "/v1/workspace/me", "", 401},
		{"member me → 200", &m, "GET", "/v1/workspace/me", "", 200},
		{"owner me → 200", &o, "GET", "/v1/workspace/me", "", 200},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, body := env.req(t, c.user, c.method, c.path, c.body)
			if code != c.want {
				t.Errorf("%s %s: got %d, want %d (body %s)", c.method, c.path, code, c.want, body)
			}
		})
	}
}

func TestRBAC_OwnerCanChangeRole(t *testing.T) {
	env := setupRBACEnv(t)
	code, body := env.req(t, &env.ownerID, "PATCH",
		"/v1/workspace/members/"+env.memberID.String()+"/role", `{"role":"admin"}`)
	if code != 200 {
		t.Fatalf("owner change role: got %d, want 200 (%s)", code, body)
	}
	// Verify the member can now write.
	code, _ = env.req(t, &env.memberID, "POST", "/v1/contacts", `{"first_name":"X","last_name":"Y"}`)
	if code != 201 {
		t.Errorf("promoted member POST contact: got %d, want 201", code)
	}
}

func TestRBAC_OwnerCannotDemoteSelf(t *testing.T) {
	env := setupRBACEnv(t)
	code, _ := env.req(t, &env.ownerID, "PATCH",
		"/v1/workspace/members/"+env.ownerID.String()+"/role", `{"role":"member"}`)
	if code != 400 {
		t.Fatalf("owner self-demote: got %d, want 400", code)
	}
}

func TestRBAC_OwnerCannotRemoveSelf(t *testing.T) {
	env := setupRBACEnv(t)
	code, _ := env.req(t, &env.ownerID, "DELETE",
		"/v1/workspace/members/"+env.ownerID.String(), "")
	if code != 400 {
		t.Fatalf("owner self-remove: got %d, want 400", code)
	}
}

func TestRBAC_OwnerCanRemoveOtherMember(t *testing.T) {
	env := setupRBACEnv(t)
	code, body := env.req(t, &env.ownerID, "DELETE",
		"/v1/workspace/members/"+env.memberID.String(), "")
	if code != 204 {
		t.Fatalf("owner remove member: got %d, want 204 (%s)", code, body)
	}
	// The removed member's session no longer resolves a role.
	code, _ = env.req(t, &env.memberID, "GET", "/v1/contacts", "")
	if code != 401 {
		t.Errorf("removed member GET contacts: got %d, want 401", code)
	}
}

func TestRBAC_MeReportsPermissions(t *testing.T) {
	env := setupRBACEnv(t)
	code, body := env.req(t, &env.memberID, "GET", "/v1/workspace/me", "")
	if code != 200 {
		t.Fatalf("me: got %d, want 200", code)
	}
	var resp struct {
		Role        string `json:"role"`
		Permissions struct {
			CanRead          bool `json:"can_read"`
			CanWrite         bool `json:"can_write"`
			CanManageMembers bool `json:"can_manage_members"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Role != "member" || !resp.Permissions.CanRead || resp.Permissions.CanWrite || resp.Permissions.CanManageMembers {
		t.Errorf("unexpected me payload: %+v", resp)
	}
}

// connectWithRetry waits out Postgres's init-script restart window.
func connectWithRetry(ctx context.Context, connStr string, maxWait time.Duration) (*pgx.Conn, error) {
	deadline := time.Now().Add(maxWait)
	for {
		conn, err := pgx.Connect(ctx, connStr)
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
