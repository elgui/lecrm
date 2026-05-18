package provision_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/gbconsult/lecrm/apps/migrate/internal/provision"
)

// connectWithRetry retries pgx.Connect until it succeeds or maxWait elapses.
// Postgres briefly resets connections while applying init scripts and restarting.
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

// initSQLPath returns the absolute path to 0001_init.sql, navigating
// from this source file to the repository root.
func initSQLPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile is apps/migrate/internal/provision/provision_test.go
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	p := filepath.Join(repoRoot, "packages", "db", "migrations", "0001_init.sql")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("init SQL not found at %s: %v", p, err)
	}
	return p
}

func startPostgres(ctx context.Context, t *testing.T) string {
	t.Helper()

	testcontainers.SkipIfProviderIsNotHealthy(t)

	initSQL := initSQLPath(t)

	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("lecrm"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.WithInitScripts(initSQL),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := ctr.Terminate(context.Background()); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	return connStr
}

// TestProvisionWorkspace_FreshAndIdempotent is the end-to-end ADR-009
// §2.1 validation: spin up Postgres 17 with 0001_init.sql, provision
// "acme", assert all invariants, re-invoke and assert idempotency.
func TestProvisionWorkspace_FreshAndIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	connStr := startPostgres(ctx, t)

	// Retry for up to 15 s: postgres briefly resets connections while applying
	// init scripts and restarting, even after testcontainers signals "ready".
	conn, err := connectWithRetry(ctx, connStr, 15*time.Second)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// --- Fresh provisioning ---
	result, err := provision.Run(ctx, conn, "acme", logger)
	if err != nil {
		t.Fatalf("provision.Run (fresh): %v", err)
	}

	if result.Slug != "acme" {
		t.Errorf("result.Slug = %q; want %q", result.Slug, "acme")
	}
	if !result.IsNew {
		t.Error("result.IsNew = false; want true for fresh provisioning")
	}

	// role_name must be workspace_<hex-uuid> (all lowercase, no dashes)
	idHex := strings.ReplaceAll(result.WorkspaceID.String(), "-", "")
	expectedRole := "workspace_" + idHex
	if result.RoleName != expectedRole {
		t.Errorf("RoleName = %q; want %q", result.RoleName, expectedRole)
	}

	// core.workspaces row
	assertWorkspacesRow(ctx, t, conn, result)

	// Postgres role attributes
	assertRoleExists(ctx, t, conn, result.RoleName)

	// Workspace schema
	assertSchemaExists(ctx, t, conn, result.RoleName)

	// river_<hex> schema
	riverSchema := "river_" + idHex
	assertSchemaExists(ctx, t, conn, riverSchema)

	// --- Idempotent re-invocation ---
	result2, err := provision.Run(ctx, conn, "acme", logger)
	if err != nil {
		t.Fatalf("provision.Run (re-invoke): %v", err)
	}
	if result2.IsNew {
		t.Error("result2.IsNew = true; want false on re-invocation")
	}
	if result2.WorkspaceID != result.WorkspaceID {
		t.Errorf("re-invocation changed WorkspaceID: got %s want %s", result2.WorkspaceID, result.WorkspaceID)
	}
	if result2.RoleName != result.RoleName {
		t.Errorf("re-invocation changed RoleName: got %q want %q", result2.RoleName, result.RoleName)
	}
}

func assertWorkspacesRow(ctx context.Context, t *testing.T, conn *pgx.Conn, r provision.Result) {
	t.Helper()
	var slug, roleName string
	err := conn.QueryRow(ctx,
		"SELECT slug, role_name FROM core.workspaces WHERE id = $1", r.WorkspaceID,
	).Scan(&slug, &roleName)
	if err != nil {
		t.Fatalf("query core.workspaces: %v", err)
	}
	if slug != r.Slug {
		t.Errorf("workspaces.slug = %q; want %q", slug, r.Slug)
	}
	if roleName != r.RoleName {
		t.Errorf("workspaces.role_name = %q; want %q", roleName, r.RoleName)
	}
}

func assertRoleExists(ctx context.Context, t *testing.T, conn *pgx.Conn, roleName string) {
	t.Helper()
	var connLimit int
	var rolConfig []string
	err := conn.QueryRow(ctx,
		"SELECT rolconnlimit, rolconfig FROM pg_roles WHERE rolname = $1", roleName,
	).Scan(&connLimit, &rolConfig)
	if err != nil {
		t.Fatalf("pg_roles lookup for %q: %v", roleName, err)
	}
	if connLimit != 10 {
		t.Errorf("role %q CONNECTION LIMIT = %d; want 10", roleName, connLimit)
	}

	// search_path must be in rolconfig
	foundSearchPath := false
	for _, cfg := range rolConfig {
		if strings.HasPrefix(cfg, "search_path=") {
			if strings.Contains(cfg, roleName) {
				foundSearchPath = true
			}
		}
	}
	if !foundSearchPath {
		t.Errorf("role %q rolconfig %v does not contain search_path=%s,…", roleName, rolConfig, roleName)
	}
}

func assertSchemaExists(ctx context.Context, t *testing.T, conn *pgx.Conn, schemaName string) {
	t.Helper()
	var owner string
	err := conn.QueryRow(ctx,
		"SELECT schema_owner FROM information_schema.schemata WHERE schema_name = $1",
		schemaName,
	).Scan(&owner)
	if err != nil {
		t.Fatalf("schema %q not found: %v", schemaName, err)
	}
	// workspace and river schemas are owned by the workspace role
	// river schema uses the workspace role as AUTHORIZATION per §2.1 step 5
	if schemaName != "core" && schemaName != "public" {
		wsRole := schemaName
		if strings.HasPrefix(schemaName, "river_") {
			wsRole = fmt.Sprintf("workspace_%s", strings.TrimPrefix(schemaName, "river_"))
		}
		if owner != wsRole {
			t.Errorf("schema %q owner = %q; want %q", schemaName, owner, wsRole)
		}
	}
}
