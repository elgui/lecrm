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

// migrationPaths returns absolute paths to the given migration filenames,
// navigating from this source file to the repository root.
func migrationPaths(t *testing.T, names ...string) []string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile is apps/migrate/internal/provision/provision_test.go
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	paths := make([]string, len(names))
	for i, name := range names {
		p := filepath.Join(repoRoot, "packages", "db", "migrations", name)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("migration not found at %s: %v", p, err)
		}
		paths[i] = p
	}
	return paths
}

func startPostgres(ctx context.Context, t *testing.T) string {
	t.Helper()

	testcontainers.SkipIfProviderIsNotHealthy(t)

	migrations := migrationPaths(t,
		"0001_init.sql",
		"0002_identity.sql",
		"0003_metadata_engine.sql",
	)

	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("lecrm"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.WithInitScripts(migrations...),
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

	// ADR-010 §3: objects table + both indexes
	assertTableExists(ctx, t, conn, result.RoleName, "objects")
	assertIndexExists(ctx, t, conn, result.RoleName, "objects_type_parent_idx")
	assertIndexExists(ctx, t, conn, result.RoleName, "objects_data_gin_idx")

	// ADR-010 §4: custom_property_definitions table + unique constraint
	assertTableExists(ctx, t, conn, result.RoleName, "custom_property_definitions")
	assertUniqueConstraintExists(ctx, t, conn, result.RoleName, "custom_property_definitions",
		[]string{"parent_type", "property_key"})

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

	// Tables and indexes must still exist after idempotent re-invocation.
	assertTableExists(ctx, t, conn, result2.RoleName, "objects")
	assertIndexExists(ctx, t, conn, result2.RoleName, "objects_type_parent_idx")
	assertIndexExists(ctx, t, conn, result2.RoleName, "objects_data_gin_idx")
	assertTableExists(ctx, t, conn, result2.RoleName, "custom_property_definitions")
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

func assertTableExists(ctx context.Context, t *testing.T, conn *pgx.Conn, schemaName, tableName string) {
	t.Helper()
	var count int
	err := conn.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema = $1 AND table_name = $2 AND table_type = 'BASE TABLE'`,
		schemaName, tableName,
	).Scan(&count)
	if err != nil {
		t.Fatalf("assertTableExists %s.%s: %v", schemaName, tableName, err)
	}
	if count != 1 {
		t.Errorf("table %s.%s not found", schemaName, tableName)
	}
}

func assertIndexExists(ctx context.Context, t *testing.T, conn *pgx.Conn, schemaName, indexName string) {
	t.Helper()
	var count int
	err := conn.QueryRow(ctx,
		`SELECT count(*) FROM pg_indexes
		 WHERE schemaname = $1 AND indexname = $2`,
		schemaName, indexName,
	).Scan(&count)
	if err != nil {
		t.Fatalf("assertIndexExists %s.%s: %v", schemaName, indexName, err)
	}
	if count != 1 {
		t.Errorf("index %s.%s not found", schemaName, indexName)
	}
}

// assertUniqueConstraintExists checks that a UNIQUE constraint spanning the given columns
// exists on the named table (schema-qualified). Column order must match the constraint definition.
func assertUniqueConstraintExists(ctx context.Context, t *testing.T, conn *pgx.Conn, schemaName, tableName string, cols []string) {
	t.Helper()
	// Build an ordered array of column names for this table's unique constraints and compare.
	rows, err := conn.Query(ctx, `
		SELECT array_agg(a.attname ORDER BY x.ordinality)
		FROM   pg_constraint c
		JOIN   pg_class      r ON r.oid = c.conrelid
		JOIN   pg_namespace  n ON n.oid = r.relnamespace
		CROSS  JOIN LATERAL unnest(c.conkey) WITH ORDINALITY x(attnum, ordinality)
		JOIN   pg_attribute  a ON a.attrelid = r.oid AND a.attnum = x.attnum
		WHERE  c.contype = 'u'
		  AND  n.nspname  = $1
		  AND  r.relname  = $2
		GROUP  BY c.oid`, schemaName, tableName)
	if err != nil {
		t.Fatalf("assertUniqueConstraintExists query for %s.%s: %v", schemaName, tableName, err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var colNames []string
		if err := rows.Scan(&colNames); err != nil {
			t.Fatalf("scan unique constraint cols: %v", err)
		}
		if len(colNames) == len(cols) {
			match := true
			for i, c := range cols {
				if colNames[i] != c {
					match = false
					break
				}
			}
			if match {
				found = true
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("assertUniqueConstraintExists rows: %v", err)
	}
	if !found {
		t.Errorf("unique constraint on %s.%s%v not found", schemaName, tableName, cols)
	}
}
