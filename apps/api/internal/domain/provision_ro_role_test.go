//go:build integration

// Smoke tests for the per-workspace RO role created by migration
// 0013_workspace_ro_role.sql. Verifies:
//
//   - The RO role exists after provisioning.
//   - lecrm_cube_reader can SET ROLE to it and SELECT from deals.
//   - The RO role CANNOT INSERT, UPDATE, or DELETE on deals.
//   - The RO role CANNOT read another workspace's schema.

package domain_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// allMigrationPaths returns every migration including 0013. The
// existing `migrationPaths` helper in provision_test.go stops at 0008;
// keeping a separate helper avoids breaking the older test that asserts
// only on Sprint-4 schema.
func allMigrationPaths(t *testing.T) []string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	migrationsDir := filepath.Join(repoRoot, "packages", "db", "migrations")

	files := []string{
		"0001_init.sql",
		"0002_identity.sql",
		"0003_metadata_engine.sql",
		"0004_workspaces_admin_email_registry.sql",
		"0005_slug_tombstoning.sql",
		"0006_security_definer_hardening.sql",
		"0007_session_revocations.sql",
		"0008_crm_entities.sql",
		"0009_metadata_json_type.sql",
		"0010_pgcrypto_to_core_schema.sql",
		"0011_external_sync.sql",
		"0012_email_suppression.sql",
		"0013_workspace_ro_role.sql",
	}

	paths := make([]string, len(files))
	for i, f := range files {
		p := filepath.Join(migrationsDir, f)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("migration %s not found at %s: %v", f, p, err)
		}
		paths[i] = p
	}
	return paths
}

func roRoleNameOf(workspaceID uuid.UUID) string {
	return "workspace_" + strings.ToLower(strings.ReplaceAll(workspaceID.String(), "-", "")) + "_ro"
}

func schemaNameOf(workspaceID uuid.UUID) string {
	return "workspace_" + strings.ToLower(strings.ReplaceAll(workspaceID.String(), "-", ""))
}

func setupCubeReaderPassword(ctx context.Context, t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	// The migration creates lecrm_cube_reader with PASSWORD NULL. To
	// connect, we need to set one. This is the deploy-time step
	// documented in 0013_workspace_ro_role.sql.
	pw := "cube-reader-test-password-" + uuid.NewString()
	if _, err := conn.Exec(ctx, fmt.Sprintf("ALTER ROLE lecrm_cube_reader WITH PASSWORD %s",
		pgQuote(pw))); err != nil {
		t.Fatalf("set lecrm_cube_reader password: %v", err)
	}
	return pw
}

func pgQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func startPostgresWithMigrations(t *testing.T) (string, func()) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	cleanup := func() { cancel() }

	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("lecrm"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.WithInitScripts(allMigrationPaths(t)...),
	)
	if err != nil {
		cleanup()
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() {
		if err := ctr.Terminate(context.Background()); err != nil {
			t.Logf("terminate: %v", err)
		}
		cleanup()
	})

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	return connStr, cleanup
}

func TestROBeforeAndAfterProvision(t *testing.T) {
	connStr, _ := startPostgresWithMigrations(t)
	ctx := context.Background()

	conn := connectWithRetry(ctx, t, connStr, 15*time.Second)
	defer func() { _ = conn.Close(ctx) }()

	// lecrm_cube_reader must already exist (created at migration time).
	var exists bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = 'lecrm_cube_reader')`,
	).Scan(&exists); err != nil {
		t.Fatalf("check lecrm_cube_reader: %v", err)
	}
	if !exists {
		t.Fatal("lecrm_cube_reader role missing after migration 0013")
	}

	wsID := uuid.New()
	if _, err := conn.Exec(ctx, "SELECT core.lecrm_provision_workspace($1)", wsID); err != nil {
		t.Fatalf("provision: %v", err)
	}

	roName := roRoleNameOf(wsID)
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)`, roName,
	).Scan(&exists); err != nil {
		t.Fatalf("check RO role: %v", err)
	}
	if !exists {
		t.Fatalf("RO role %s missing after provision", roName)
	}

	// The RO role must be NOLOGIN — only reachable via SET ROLE.
	var canLogin bool
	if err := conn.QueryRow(ctx,
		`SELECT rolcanlogin FROM pg_roles WHERE rolname = $1`, roName,
	).Scan(&canLogin); err != nil {
		t.Fatalf("check rolcanlogin: %v", err)
	}
	if canLogin {
		t.Errorf("RO role %s has LOGIN — must be NOLOGIN", roName)
	}

	// lecrm_cube_reader must be a member of the new RO role.
	var member bool
	if err := conn.QueryRow(ctx, `
		SELECT pg_has_role('lecrm_cube_reader', $1, 'MEMBER')
	`, roName).Scan(&member); err != nil {
		t.Fatalf("check membership: %v", err)
	}
	if !member {
		t.Errorf("lecrm_cube_reader is not a member of %s", roName)
	}
}

// connectAsCubeReader opens a pgx connection authenticated as
// lecrm_cube_reader. It needs a password to be set on the role first.
func connectAsCubeReader(ctx context.Context, t *testing.T, baseConnStr, password string) *pgx.Conn {
	t.Helper()
	cfg, err := pgx.ParseConfig(baseConnStr)
	if err != nil {
		t.Fatalf("parse conn str: %v", err)
	}
	cfg.User = "lecrm_cube_reader"
	cfg.Password = password
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect as lecrm_cube_reader: %v", err)
	}
	return conn
}

func TestROCanSelectCannotWrite(t *testing.T) {
	connStr, _ := startPostgresWithMigrations(t)
	ctx := context.Background()

	admin := connectWithRetry(ctx, t, connStr, 15*time.Second)
	defer func() { _ = admin.Close(ctx) }()

	cubePw := setupCubeReaderPassword(ctx, t, admin)

	wsID := uuid.New()
	if _, err := admin.Exec(ctx, "SELECT core.lecrm_provision_workspace($1)", wsID); err != nil {
		t.Fatalf("provision: %v", err)
	}
	schema := schemaNameOf(wsID)
	roName := roRoleNameOf(wsID)

	// Seed a row in the workspace.deals table as the superuser so the
	// RO test has something to SELECT against.
	if _, err := admin.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s.deals (title) VALUES ('seed deal')`, pgx.Identifier{schema}.Sanitize()),
	); err != nil {
		t.Fatalf("seed deal: %v", err)
	}

	cube := connectAsCubeReader(ctx, t, connStr, cubePw)
	defer func() { _ = cube.Close(ctx) }()

	tx, err := cube.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// SET ROLE to the per-workspace RO role.
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL ROLE %s", pgx.Identifier{roName}.Sanitize())); err != nil {
		t.Fatalf("SET LOCAL ROLE %s: %v", roName, err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL search_path TO %s", pgx.Identifier{schema}.Sanitize())); err != nil {
		t.Fatalf("SET search_path: %v", err)
	}

	// SELECT must succeed.
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM deals").Scan(&count); err != nil {
		t.Fatalf("SELECT as RO: %v", err)
	}
	if count != 1 {
		t.Errorf("count: got %d want 1", count)
	}

	// INSERT must fail with permission-denied.
	if _, err := tx.Exec(ctx, "INSERT INTO deals (title) VALUES ('write-attempt')"); err == nil {
		t.Error("RO role INSERT succeeded — must be denied")
	}
	// Failed exec aborts the transaction; explicit rollback is the
	// cleanup. The deferred Rollback above is the safety net.
	_ = tx.Rollback(ctx)

	// Same role, fresh transaction — UPDATE must also fail.
	tx2, err := cube.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	defer tx2.Rollback(ctx) //nolint:errcheck
	if _, err := tx2.Exec(ctx, fmt.Sprintf("SET LOCAL ROLE %s", pgx.Identifier{roName}.Sanitize())); err != nil {
		t.Fatalf("SET LOCAL ROLE tx2: %v", err)
	}
	if _, err := tx2.Exec(ctx, fmt.Sprintf("SET LOCAL search_path TO %s", pgx.Identifier{schema}.Sanitize())); err != nil {
		t.Fatalf("SET search_path tx2: %v", err)
	}
	if _, err := tx2.Exec(ctx, "UPDATE deals SET title = 'updated'"); err == nil {
		t.Error("RO role UPDATE succeeded — must be denied")
	}
	_ = tx2.Rollback(ctx)

	// DELETE also must fail.
	tx3, err := cube.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx3: %v", err)
	}
	defer tx3.Rollback(ctx) //nolint:errcheck
	if _, err := tx3.Exec(ctx, fmt.Sprintf("SET LOCAL ROLE %s", pgx.Identifier{roName}.Sanitize())); err != nil {
		t.Fatalf("SET LOCAL ROLE tx3: %v", err)
	}
	if _, err := tx3.Exec(ctx, fmt.Sprintf("SET LOCAL search_path TO %s", pgx.Identifier{schema}.Sanitize())); err != nil {
		t.Fatalf("SET search_path tx3: %v", err)
	}
	if _, err := tx3.Exec(ctx, "DELETE FROM deals"); err == nil {
		t.Error("RO role DELETE succeeded — must be denied")
	}
}

func TestROCannotReadOtherWorkspace(t *testing.T) {
	connStr, _ := startPostgresWithMigrations(t)
	ctx := context.Background()

	admin := connectWithRetry(ctx, t, connStr, 15*time.Second)
	defer func() { _ = admin.Close(ctx) }()

	cubePw := setupCubeReaderPassword(ctx, t, admin)

	wsA := uuid.New()
	wsB := uuid.New()
	for _, ws := range []uuid.UUID{wsA, wsB} {
		if _, err := admin.Exec(ctx, "SELECT core.lecrm_provision_workspace($1)", ws); err != nil {
			t.Fatalf("provision %s: %v", ws, err)
		}
	}

	// Seed a row in workspace B's deals.
	schemaB := schemaNameOf(wsB)
	if _, err := admin.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s.deals (title) VALUES ('b-only')`, pgx.Identifier{schemaB}.Sanitize()),
	); err != nil {
		t.Fatalf("seed B: %v", err)
	}

	roA := roRoleNameOf(wsA)

	cube := connectAsCubeReader(ctx, t, connStr, cubePw)
	defer func() { _ = cube.Close(ctx) }()

	tx, err := cube.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL ROLE %s", pgx.Identifier{roA}.Sanitize())); err != nil {
		t.Fatalf("SET LOCAL ROLE %s: %v", roA, err)
	}

	// Workspace A's RO role MUST NOT be able to SELECT from workspace B's schema.
	if _, err := tx.Exec(ctx, fmt.Sprintf(
		"SELECT count(*) FROM %s.deals", pgx.Identifier{schemaB}.Sanitize()),
	); err == nil {
		t.Error("workspace A RO role read workspace B schema — must be denied")
	}
}
