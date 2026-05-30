//go:build integration

// Integration coverage for login-time integrator elevation and the
// GET /auth/workspaces data path (Sprint: lecrm-integrator-switching,
// tasket 3). Spins up a real Postgres with the full migration chain
// (0001..0019), provisions two isolated workspaces, and exercises the auth
// Store + members Store against live SQL.
//
// Run:
//
//	go -C apps/api test -tags integration -count 1 -run TestIntegrator ./internal/auth
package auth_test

import (
	"context"
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

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	"github.com/gbconsult/lecrm/apps/api/internal/members"
	"github.com/gbconsult/lecrm/apps/api/internal/rbac"
)

func integratorMigrations() []string {
	return []string{
		"0001_init.sql", "0002_identity.sql", "0003_metadata_engine.sql",
		"0004_workspaces_admin_email_registry.sql", "0005_slug_tombstoning.sql",
		"0006_security_definer_hardening.sql", "0007_session_revocations.sql",
		"0008_crm_entities.sql", "0009_metadata_json_type.sql",
		"0010_pgcrypto_to_core_schema.sql", "0011_external_sync.sql",
		"0012_email_suppression.sql", "0013_workspace_ro_role.sql",
		"0014_idempotency_keys.sql", "0015_activities_notes_tasks.sql",
		"0016_service_tokens.sql", "0017_app_role.sql",
		"0018_integrator_role_and_grants.sql", "0019_integrator_audit_actor.sql",
	}
}

func integratorMigrationPath(t *testing.T, filename string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile: apps/api/internal/auth/integrator_integration_test.go → repo root is 5 up.
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	p := filepath.Join(repoRoot, "packages", "db", "migrations", filename)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("migration %s not found at %s: %v", filename, p, err)
	}
	return p
}

type integratorEnv struct {
	pool *pgxpool.Pool
	wsA  uuid.UUID
	wsB  uuid.UUID
}

func setupIntegratorEnv(t *testing.T) *integratorEnv {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	var scripts []string
	for _, m := range integratorMigrations() {
		scripts = append(scripts, integratorMigrationPath(t, m))
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

	probe := connectIntegratorWithRetry(ctx, t, connStr, 30*time.Second)
	_ = probe.Close(ctx)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	wsA := provisionIntegratorWorkspace(ctx, t, pool, "acme-a")
	wsB := provisionIntegratorWorkspace(ctx, t, pool, "acme-b")

	return &integratorEnv{pool: pool, wsA: wsA, wsB: wsB}
}

func connectIntegratorWithRetry(ctx context.Context, t *testing.T, connStr string, maxWait time.Duration) *pgx.Conn {
	t.Helper()
	deadline := time.Now().Add(maxWait)
	for {
		conn, err := pgx.Connect(ctx, connStr)
		if err == nil {
			return conn
		}
		if time.Now().After(deadline) {
			t.Fatalf("connect after %s: %v", maxWait, err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("connect: %v", ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func provisionIntegratorWorkspace(ctx context.Context, t *testing.T, pool *pgxpool.Pool, slug string) uuid.UUID {
	t.Helper()
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

// seedUser inserts a core.users row with the given email and returns its id.
func seedUser(ctx context.Context, t *testing.T, store *auth.Store, issuer, subject, email string) uuid.UUID {
	t.Helper()
	id, err := store.UpsertUser(ctx, issuer, subject, email, "Test "+subject)
	if err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

// TestIntegratorLoginElevation covers the no-downgrade + grant-driven
// elevation contract of EnsureMemberWithRole + IntegratorGrantExists.
func TestIntegratorLoginElevation(t *testing.T) {
	env := setupIntegratorEnv(t)
	ctx := context.Background()
	store := auth.NewStore(env.pool)

	// Grant the integrator access to workspace A only.
	leoEmail := "leo@vernayo.com"
	if _, err := env.pool.Exec(ctx,
		`INSERT INTO core.integrator_grants (workspace_id, email, granted_by) VALUES ($1, $2, 'test')`,
		env.wsA, "LEO@Vernayo.com", // mixed case to exercise lower() matching
	); err != nil {
		t.Fatalf("seed grant: %v", err)
	}

	leoID := seedUser(ctx, t, store, "https://idp.example", "leo-sub", leoEmail)

	// --- Workspace A: grant exists → elevate to integrator. ---
	grantedA, err := store.IntegratorGrantExists(ctx, env.wsA, leoEmail)
	if err != nil {
		t.Fatalf("grant exists A: %v", err)
	}
	if !grantedA {
		t.Fatal("expected integrator grant to exist for workspace A")
	}
	if err := store.EnsureMemberWithRole(ctx, env.wsA, leoID, "integrator"); err != nil {
		t.Fatalf("ensure integrator A: %v", err)
	}
	if got := readRole(ctx, t, env.pool, env.wsA, leoID); got != "integrator" {
		t.Fatalf("workspace A role = %q, want integrator", got)
	}

	// --- Workspace B: no grant → plain member, NOT elevated. ---
	grantedB, err := store.IntegratorGrantExists(ctx, env.wsB, leoEmail)
	if err != nil {
		t.Fatalf("grant exists B: %v", err)
	}
	if grantedB {
		t.Fatal("did not expect an integrator grant for workspace B")
	}
	role := "member"
	if grantedB {
		role = "integrator"
	}
	if err := store.EnsureMemberWithRole(ctx, env.wsB, leoID, role); err != nil {
		t.Fatalf("ensure member B: %v", err)
	}
	if got := readRole(ctx, t, env.pool, env.wsB, leoID); got != "member" {
		t.Fatalf("workspace B role = %q, want member (no auto-elevation)", got)
	}

	// --- No-downgrade: an existing owner logging in normally keeps owner. ---
	ownerID := seedUser(ctx, t, store, "https://idp.example", "owner-sub", "owner@acme-a.test")
	if _, err := env.pool.Exec(ctx,
		`INSERT INTO core.workspace_members (workspace_id, user_id, role, joined_at) VALUES ($1, $2, 'owner', now())`,
		env.wsA, ownerID,
	); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	if err := store.EnsureMember(ctx, env.wsA, ownerID); err != nil { // plain 'member' write
		t.Fatalf("ensure member (owner): %v", err)
	}
	if got := readRole(ctx, t, env.pool, env.wsA, ownerID); got != "owner" {
		t.Fatalf("owner role = %q, want owner (must never downgrade)", got)
	}

	// --- Idempotent re-login of integrator keeps integrator. ---
	if err := store.EnsureMemberWithRole(ctx, env.wsA, leoID, "integrator"); err != nil {
		t.Fatalf("re-ensure integrator A: %v", err)
	}
	if got := readRole(ctx, t, env.pool, env.wsA, leoID); got != "integrator" {
		t.Fatalf("workspace A role after re-login = %q, want integrator", got)
	}
}

// TestIntegratorListAccessibleWorkspaces covers the GET /auth/workspaces
// union: memberships + pending grants, scoped to the caller only.
func TestIntegratorListAccessibleWorkspaces(t *testing.T) {
	env := setupIntegratorEnv(t)
	ctx := context.Background()
	store := auth.NewStore(env.pool)

	leoEmail := "leo@vernayo.com"
	leoID := seedUser(ctx, t, store, "https://idp.example", "leo-sub", leoEmail)

	// Léo: a pending grant for B (never logged in) + a real membership in A.
	if _, err := env.pool.Exec(ctx,
		`INSERT INTO core.integrator_grants (workspace_id, email) VALUES ($1, $2)`,
		env.wsB, leoEmail,
	); err != nil {
		t.Fatalf("seed grant B: %v", err)
	}
	if err := store.EnsureMemberWithRole(ctx, env.wsA, leoID, "integrator"); err != nil {
		t.Fatalf("ensure integrator A: %v", err)
	}

	// A different user who is only a plain member of A.
	otherID := seedUser(ctx, t, store, "https://idp.example", "other-sub", "other@acme-a.test")
	if err := store.EnsureMember(ctx, env.wsA, otherID); err != nil {
		t.Fatalf("ensure other member: %v", err)
	}

	// Léo sees BOTH workspaces: A via membership (integrator), B via grant
	// (never-logged-into → role "integrator" from the grant default).
	leoWs, err := store.ListAccessibleWorkspaces(ctx, leoID, leoEmail)
	if err != nil {
		t.Fatalf("list accessible (leo): %v", err)
	}
	got := map[string]string{}
	for _, w := range leoWs {
		got[w.Slug] = w.Role
	}
	if got["acme-a"] != "integrator" {
		t.Fatalf("leo acme-a role = %q, want integrator", got["acme-a"])
	}
	if got["acme-b"] != "integrator" {
		t.Fatalf("leo acme-b role = %q, want integrator (grant, never logged in)", got["acme-b"])
	}
	if len(leoWs) != 2 {
		t.Fatalf("leo accessible count = %d, want 2: %+v", len(leoWs), leoWs)
	}

	// The other user sees ONLY workspace A as a member — never Léo's grant
	// in B. Cross-user isolation.
	otherWs, err := store.ListAccessibleWorkspaces(ctx, otherID, "other@acme-a.test")
	if err != nil {
		t.Fatalf("list accessible (other): %v", err)
	}
	if len(otherWs) != 1 || otherWs[0].Slug != "acme-a" || otherWs[0].Role != "member" {
		t.Fatalf("other accessible = %+v, want [{acme-a member}]", otherWs)
	}
}

// TestIntegratorExcludedFromMemberList covers the members-listing hygiene:
// integrator rows are hidden from GET /v1/workspace/members.
func TestIntegratorExcludedFromMemberList(t *testing.T) {
	env := setupIntegratorEnv(t)
	ctx := context.Background()
	store := auth.NewStore(env.pool)
	mstore := &members.PgMemberStore{Pool: env.pool}

	leoID := seedUser(ctx, t, store, "https://idp.example", "leo-sub", "leo@vernayo.com")
	clientID := seedUser(ctx, t, store, "https://idp.example", "client-sub", "client@acme-a.test")

	if err := store.EnsureMemberWithRole(ctx, env.wsA, leoID, "integrator"); err != nil {
		t.Fatalf("ensure integrator: %v", err)
	}
	if err := store.EnsureMemberWithRole(ctx, env.wsA, clientID, "owner"); err != nil {
		t.Fatalf("ensure owner: %v", err)
	}

	list, err := mstore.ListMembers(ctx, env.wsA)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	for _, m := range list {
		if m.UserID == leoID {
			t.Fatalf("integrator %s must be excluded from the client member list", leoID)
		}
		if m.Role == "integrator" {
			t.Fatalf("found integrator-role row in member list: %+v", m)
		}
	}
	// Sanity: the client owner IS listed (the filter is integrator-only).
	var sawClient bool
	for _, m := range list {
		if m.UserID == clientID {
			sawClient = true
		}
	}
	if !sawClient {
		t.Fatalf("expected client owner %s in member list, got %+v", clientID, list)
	}

	// LookupRole must still resolve the integrator for authorization — the
	// exclusion is presentation-only.
	role, found, err := mstore.LookupRole(ctx, env.wsA, leoID)
	if err != nil || !found {
		t.Fatalf("lookup integrator role: found=%v err=%v", found, err)
	}
	if role != rbac.RoleIntegrator {
		t.Fatalf("lookup role = %v, want RoleIntegrator", role)
	}
}

func readRole(ctx context.Context, t *testing.T, pool *pgxpool.Pool, wsID, userID uuid.UUID) string {
	t.Helper()
	var role string
	if err := pool.QueryRow(ctx,
		`SELECT role FROM core.workspace_members WHERE workspace_id = $1 AND user_id = $2`,
		wsID, userID,
	).Scan(&role); err != nil {
		t.Fatalf("read role: %v", err)
	}
	return role
}
