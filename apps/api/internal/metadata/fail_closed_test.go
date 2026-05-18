//go:build integration

// Fail-closed test for the metadata engine (ADR-009 §7.2 / ADR-010 §TO RESOLVE-2).
//
// Verifies that metadata.Set rejects a write — and does NOT persist any data —
// when the core.audit_log INSERT fails, regardless of the cause.
//
// Run:
//
//	~/.local/go/bin/go -C apps/api test -tags integration -count 1 -v \
//	    -run TestSet_FailClosed ./internal/metadata/
package metadata_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/gbconsult/lecrm/apps/api/internal/metadata"
)

func migrationPath(t *testing.T, filename string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile: apps/api/internal/metadata/fail_closed_test.go
	// Four levels up reaches the repo root (leCRM/).
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	p := filepath.Join(repoRoot, "packages", "db", "migrations", filename)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("migration %s not found at %s: %v", filename, p, err)
	}
	return p
}

// TestSet_FailClosed_AuditWriteRollsBackMetadata simulates an audit write
// failure by dropping core.audit_log before calling Set. The test asserts
// that Set returns an error AND that no row appears in the objects table —
// proving the transaction rolled back.
func TestSet_FailClosed_AuditWriteRollsBackMetadata(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("lecrm"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.WithInitScripts(
			migrationPath(t, "0001_init.sql"),
			migrationPath(t, "0003_metadata_engine.sql"),
		),
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

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	// Provision a workspace — creates the workspace schema with objects and
	// custom_property_definitions tables (via 0003_metadata_engine.sql).
	wsID := uuid.New()
	var schema string
	if err := pool.QueryRow(ctx, "SELECT core.lecrm_provision_workspace($1)", wsID).Scan(&schema); err != nil {
		t.Fatalf("provision workspace: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO core.workspaces (id, slug, role_name) VALUES ($1, $2, $3)`,
		wsID, "test-ws", schema,
	); err != nil {
		t.Fatalf("insert workspace row: %v", err)
	}

	store := metadata.New(pool, schema, wsID)
	contactID := uuid.New()
	testData := map[string]any{"color": "blue", "priority": "high"}

	// Simulate audit write failure by removing the target table.
	if _, err := pool.Exec(ctx, "DROP TABLE core.audit_log CASCADE"); err != nil {
		t.Fatalf("drop audit_log: %v", err)
	}

	// Set MUST return an error — the audit INSERT cannot succeed.
	err = store.Set(ctx, "contact", contactID, testData)
	if err == nil {
		t.Fatal("Set returned nil; expected error because core.audit_log was dropped")
	}
	t.Logf("Set returned expected error: %v", err)

	// The objects row must NOT exist — the transaction must have rolled back.
	var count int
	countQ := `SELECT COUNT(*) FROM "` + schema + `".objects WHERE parent_type = 'contact' AND parent_id = $1`
	if err := pool.QueryRow(ctx, countQ, contactID).Scan(&count); err != nil {
		t.Fatalf("count objects rows: %v", err)
	}
	if count != 0 {
		t.Errorf("fail-closed VIOLATED: %d objects row(s) persisted for contact %s; expected 0 (transaction should have rolled back)",
			count, contactID)
	}
}
