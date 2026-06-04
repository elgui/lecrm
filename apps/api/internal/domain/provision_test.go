//go:build integration

package domain_test

import (
	"context"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// migrationPaths returns the FULL production migration chain (every
// NNNN_*.sql, sorted), not a hardcoded subset. A truncated list silently
// rots: this helper used to stop at 0008, so provisioning could not see
// gen_random_bytes after migration 0010 relocated pgcrypto into the core
// schema (0006 pins the SECURITY DEFINER search_path to core,pg_catalog).
// That stayed invisible until the integration suite first ran in CI.
// Globbing keeps the harness in lockstep with prod as new migrations land.
// The zero-padded NNNN_ prefix makes lexical sort == numeric order, and a
// renumber gap (no 0020) is handled transparently.
func migrationPaths(t *testing.T) []string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
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

func connectWithRetry(ctx context.Context, t *testing.T, connStr string, maxWait time.Duration) *pgx.Conn {
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
			t.Fatalf("context cancelled: %v", ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func TestProvision_CRMEntitiesExist(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("lecrm"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.WithInitScripts(migrationPaths(t)...),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() {
		if err := ctr.Terminate(context.Background()); err != nil {
			t.Logf("terminate: %v", err)
		}
	})

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	conn := connectWithRetry(ctx, t, connStr, 15*time.Second)
	defer func() { _ = conn.Close(ctx) }()

	wsID := uuid.New()
	var roleName string
	if err := conn.QueryRow(ctx, "SELECT core.lecrm_provision_workspace($1)", wsID).Scan(&roleName); err != nil {
		t.Fatalf("provision workspace: %v", err)
	}

	wantTables := []string{"companies", "contacts", "deals", "custom_property_definitions", "objects"}
	for _, table := range wantTables {
		var exists bool
		err := conn.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = $1 AND table_name = $2
			)
		`, roleName, table).Scan(&exists)
		if err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %s.%s does not exist after provisioning", roleName, table)
		}
	}

	expectedColumns := map[string][]string{
		"companies": {"id", "name", "domain", "industry", "size", "owner_id", "created_at", "updated_at"},
		"contacts":  {"id", "first_name", "last_name", "email", "phone", "company_id", "owner_id", "created_at", "updated_at"},
		"deals":     {"id", "title", "amount", "currency", "stage_id", "contact_id", "company_id", "owner_id", "expected_close_date", "closed_at", "created_at", "updated_at"},
	}

	for table, wantCols := range expectedColumns {
		rows, err := conn.Query(ctx, `
			SELECT column_name FROM information_schema.columns
			WHERE table_schema = $1 AND table_name = $2
			ORDER BY ordinal_position
		`, roleName, table)
		if err != nil {
			t.Fatalf("query columns for %s: %v", table, err)
		}

		gotCols := map[string]bool{}
		for rows.Next() {
			var col string
			if err := rows.Scan(&col); err != nil {
				t.Fatalf("scan column for %s: %v", table, err)
			}
			gotCols[col] = true
		}
		rows.Close()

		for _, want := range wantCols {
			if !gotCols[want] {
				t.Errorf("table %s.%s missing column %q", roleName, table, want)
			}
		}
	}

	wantIndexes := []struct {
		table string
		index string
	}{
		{"contacts", "contacts_company_id_idx"},
		{"contacts", "contacts_email_idx"},
		{"deals", "deals_stage_id_idx"},
		{"deals", "deals_contact_id_idx"},
		{"deals", "deals_expected_close_date_idx"},
	}
	for _, wi := range wantIndexes {
		var exists bool
		err := conn.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_indexes
				WHERE schemaname = $1 AND tablename = $2 AND indexname = $3
			)
		`, roleName, wi.table, wi.index).Scan(&exists)
		if err != nil {
			t.Fatalf("check index %s: %v", wi.index, err)
		}
		if !exists {
			t.Errorf("index %s on %s.%s does not exist", wi.index, roleName, wi.table)
		}
	}
}

func TestProvision_CRMEntities_Idempotent(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("lecrm"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.WithInitScripts(migrationPaths(t)...),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() {
		if err := ctr.Terminate(context.Background()); err != nil {
			t.Logf("terminate: %v", err)
		}
	})

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	conn := connectWithRetry(ctx, t, connStr, 15*time.Second)
	defer func() { _ = conn.Close(ctx) }()

	wsID := uuid.New()

	var role1 string
	if err := conn.QueryRow(ctx, "SELECT core.lecrm_provision_workspace($1)", wsID).Scan(&role1); err != nil {
		t.Fatalf("first provision: %v", err)
	}

	var role2 string
	if err := conn.QueryRow(ctx, "SELECT core.lecrm_provision_workspace($1)", wsID).Scan(&role2); err != nil {
		t.Fatalf("second provision (idempotent): %v", err)
	}

	if role1 != role2 {
		t.Errorf("role names differ: %s vs %s", role1, role2)
	}

	var tableCount int
	err = conn.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = $1 AND table_name IN ('companies', 'contacts', 'deals')
	`, role1).Scan(&tableCount)
	if err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tableCount != 3 {
		t.Errorf("expected 3 CRM tables after re-provision, got %d", tableCount)
	}
}
