//go:build integration

package tenant_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// dsn returns the DSN to use for the integration suite. Tests skip when
// the env var is unset so unit suites (`go test ./...`) without the
// integration tag and without Postgres still pass.
func dsn(t *testing.T) string {
	t.Helper()
	d := os.Getenv("LECRM_PROVISIONER_DSN")
	if d == "" {
		d = os.Getenv("DATABASE_URL")
	}
	if d == "" {
		t.Skip("LECRM_PROVISIONER_DSN not set — skipping integration test")
	}
	return d
}

// newConn opens a fresh pgx.Conn against the integration DSN and ensures
// the Story 8.1 schema is applied. Tests register a t.Cleanup for the
// close so they never leak connections.
func newConn(t *testing.T) *pgx.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn(t))
	if err != nil {
		t.Fatalf("pgx.Connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	ensureMigrationsApplied(t, conn)
	return conn
}

// ensureMigrationsApplied applies any missing migrations from
// packages/db/migrations against the test DB. The CI workflow already
// runs the migrator before this suite, but local invocations need a
// safety net.
func ensureMigrationsApplied(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	ctx := context.Background()

	// Check whether the Story 8.1 wrapper exists. If yes, migrations are
	// good; if not, walk packages/db/migrations and apply each file.
	var exists bool
	err := conn.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM pg_proc p
		    JOIN pg_namespace n ON n.oid = p.pronamespace
		   WHERE n.nspname = 'core' AND p.proname = 'lecrm_provision_workspace_with_registry'
		)
	`).Scan(&exists)
	if err != nil {
		t.Fatalf("probe wrapper: %v", err)
	}
	if exists {
		return
	}

	dir := migrationsDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir %s: %v", dir, err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := conn.Exec(ctx, string(body)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(f), err)
		}
	}
}

// migrationsDir resolves packages/db/migrations relative to this test
// file (apps/admin/internal/tenant/), regardless of where `go test` is
// invoked from.
func migrationsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile = .../apps/admin/internal/tenant/testhelper_test.go
	// repo root = ../../../..
	repo := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	dir := filepath.Join(repo, "packages", "db", "migrations")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("migrations dir %s: %v", dir, err)
	}
	return dir
}

// uniqueSlug returns a deterministic-but-random slug for this test, and
// registers a t.Cleanup that drops every artifact the wrapper created
// (workspaces row, audit rows, schemas, role). Cleanup is best-effort so
// a failing test still surfaces the original assertion.
func uniqueSlug(t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand: %v", err)
	}
	slug := "test-" + hex.EncodeToString(buf) // "test-" + 8 hex = 13 chars
	t.Cleanup(func() { dropBySlug(t, conn, slug) })
	return slug
}

// dropBySlug tears down everything Story 8.1 may have created for a slug.
// Safe to call when nothing was provisioned (every step uses IF EXISTS).
func dropBySlug(t *testing.T, conn *pgx.Conn, slug string) {
	t.Helper()
	ctx := context.Background()

	var id uuid.UUID
	err := conn.QueryRow(ctx, `SELECT id FROM core.workspaces WHERE slug = $1`, slug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return
	}
	if err != nil {
		t.Logf("cleanup: lookup %s: %v", slug, err)
		return
	}
	roleName := "workspace_" + strings.ReplaceAll(id.String(), "-", "")
	riverName := "river_" + strings.ReplaceAll(id.String(), "-", "")

	mustExec(t, conn, `DELETE FROM core.audit_log WHERE workspace_id = $1`, id)
	mustExec(t, conn, `DELETE FROM core.workspaces WHERE id = $1`, id)
	mustExec(t, conn, fmt.Sprintf(`DROP SCHEMA IF EXISTS %q CASCADE`, roleName))
	mustExec(t, conn, fmt.Sprintf(`DROP SCHEMA IF EXISTS %q CASCADE`, riverName))
	mustExec(t, conn, fmt.Sprintf(`DROP ROLE IF EXISTS %q`, roleName))
}

// mustExec runs an SQL statement and only logs (does not fail) on error.
// Used from cleanup paths where surfacing a cleanup failure would mask
// the test's real failure.
func mustExec(t *testing.T, conn *pgx.Conn, sql string, args ...any) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), sql, args...); err != nil {
		t.Logf("cleanup exec %q: %v", sql, err)
	}
}
