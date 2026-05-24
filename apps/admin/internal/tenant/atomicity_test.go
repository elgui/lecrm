//go:build integration

package tenant_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/admin/internal/tenant"
	"github.com/gbconsult/lecrm/apps/admin/internal/tenant/templates"
)

// TestAtomicityPoisonFixture covers AC-T1 — the single most important AC
// of Story 8.1. We poison the registry with a row that occupies the
// target slug under a foreign UUID. When the SECURITY DEFINER wrapper
// tries to INSERT its own row for the target slug, the UNIQUE (slug)
// constraint fires and the entire wrapper transaction rolls back. We
// then assert that NO partial state survived: no workspace_<test_uuid>
// role, no workspace_<test_uuid> schema, no river_<test_uuid> schema, no
// audit_log row bound to the test UUID.
//
// Léo's panic scenario (partial provisioning during the first Design
// Partner demo) is exactly the failure mode this test guards against.
func TestAtomicityPoisonFixture(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()
	slug := uniqueSlug(t, conn)

	// Poison: insert a registry row holding the slug under a fake UUID.
	// The fake UUID's role/schema do NOT exist (we only touch the
	// registry table) — that's fine, the UNIQUE (slug) constraint alone
	// will block the wrapper.
	poisonID := uuid.New() // UUIDv4 deliberately so it can't collide with our wrapper's v7 mint.
	poisonRole := "workspace_" + strings.ReplaceAll(poisonID.String(), "-", "")
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.workspaces (id, slug, role_name, admin_email, creator_email)
		VALUES ($1, $2, $3, 'poison@example.com', 'poison@example.com')
	`, poisonID, slug, poisonRole); err != nil {
		t.Fatalf("poison insert: %v", err)
	}
	t.Cleanup(func() {
		// Drop the poison row directly; uniqueSlug's cleanup runs first
		// and may find this row instead of a real tenant.
		_, _ = conn.Exec(context.Background(), `DELETE FROM core.workspaces WHERE id = $1`, poisonID)
	})

	// Call Create. We expect a slug-conflict error (the default path
	// fails loud with AC-F2 — it pre-checks core.workspaces). To
	// actually exercise the WRAPPER's failure path we need to bypass
	// the Go pre-check; --upsert is exactly that flag (it looks up the
	// existing UUID and calls the wrapper with it).
	//
	// But --upsert with the poison row in place becomes a successful
	// no-op (the wrapper sees its own poison UUID and ON CONFLICT (id)
	// DO NOTHING returns success). That's NOT the poison path.
	//
	// To force the wrapper's INSERT to fail on UNIQUE (slug), we must
	// pass a FRESH UUID for the same slug. We do that by calling the
	// wrapper directly (callers normally do this via Create).
	freshID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("mint UUIDv7: %v", err)
	}

	var roleName string
	err = conn.QueryRow(ctx,
		`SELECT core.lecrm_provision_workspace_with_registry($1, $2, $3, $4, $5)`,
		freshID, slug, "ci@example.com", "ci@example.com", templates.GBConsultDefaultName,
	).Scan(&roleName)
	if err == nil {
		t.Fatal("expected wrapper to fail under slug-conflict poison; got success")
	}

	// Atomicity assertions: NOTHING the wrapper would have created for
	// freshID survives in the database.
	freshRole := "workspace_" + strings.ReplaceAll(freshID.String(), "-", "")
	freshRiver := "river_" + strings.ReplaceAll(freshID.String(), "-", "")

	var freshRegistry int
	_ = conn.QueryRow(ctx, `SELECT count(*) FROM core.workspaces WHERE id = $1`, freshID).Scan(&freshRegistry)
	if freshRegistry != 0 {
		t.Errorf("registry row for failed UUID survived: count=%d", freshRegistry)
	}

	var roleExists bool
	_ = conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, freshRole).Scan(&roleExists)
	if roleExists {
		t.Errorf("role %s survived a failed wrapper call (atomicity broken)", freshRole)
	}

	var workspaceSchemaExists, riverSchemaExists bool
	_ = conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)`,
		freshRole).Scan(&workspaceSchemaExists)
	_ = conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)`,
		freshRiver).Scan(&riverSchemaExists)
	if workspaceSchemaExists {
		t.Errorf("workspace schema %s survived (atomicity broken)", freshRole)
	}
	if riverSchemaExists {
		t.Errorf("river schema %s survived (atomicity broken)", freshRiver)
	}

	var auditRows int
	_ = conn.QueryRow(ctx,
		`SELECT count(*) FROM core.audit_log WHERE workspace_id = $1 AND event = 'workspace.provisioned'`,
		freshID).Scan(&auditRows)
	if auditRows != 0 {
		t.Errorf("audit rows for failed UUID survived: count=%d", auditRows)
	}
}

// TestAtomicityCreateBypass exercises the same poison via the Create
// entry point with --upsert disabled — should hit the Go-side pre-check
// (AC-F2) before the wrapper, no role/schema attempted.
func TestAtomicityCreateBypass(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()
	slug := uniqueSlug(t, conn)

	poisonID := uuid.New()
	poisonRole := "workspace_" + strings.ReplaceAll(poisonID.String(), "-", "")
	if _, err := conn.Exec(ctx, `
		INSERT INTO core.workspaces (id, slug, role_name, admin_email, creator_email)
		VALUES ($1, $2, $3, 'poison@example.com', 'poison@example.com')
	`, poisonID, slug, poisonRole); err != nil {
		t.Fatalf("poison insert: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), `DELETE FROM core.workspaces WHERE id = $1`, poisonID)
	})

	_, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:       slug,
		AdminEmail: "ci@example.com",
		Template:   templates.GBConsultDefaultName,
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected slug-conflict from pre-check")
	}
	se, ok := err.(*tenant.StructErr)
	if !ok || se.Kind != tenant.ErrKindSlugConflict {
		t.Fatalf("expected slug_conflict, got %T %v", err, err)
	}
}
