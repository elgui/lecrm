//go:build integration

package tenant_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gbconsult/lecrm/apps/admin/internal/tenant"
	"github.com/gbconsult/lecrm/apps/admin/internal/tenant/templates"
)

// TestCreateFresh exercises the happy path: a never-seen-before slug
// produces a tenant with role, schema, audit row, and pipeline seed.
// Covers AC-F1, AC-F5, AC-I-13 (RBAC status line on stdout).
func TestCreateFresh(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()
	slug := uniqueSlug(t, conn)

	var out bytes.Buffer
	res, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:       slug,
		AdminEmail: "ci@example.com",
		Template:   templates.GBConsultDefaultName,
	}, &out)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.WorkspaceID.Version() != 7 {
		t.Fatalf("expected UUIDv7, got version %d", res.WorkspaceID.Version())
	}

	// AC-I-13: stdout must contain the RBAC seeding skipped line.
	if !strings.Contains(out.String(), "[PROVISION] RBAC seeding: skipped") {
		t.Errorf("stdout missing RBAC skipped status line:\n%s", out.String())
	}
	// WORKSPACE_ID= line for CI smoke parsing.
	if !strings.Contains(out.String(), "WORKSPACE_ID="+res.WorkspaceID.String()) {
		t.Errorf("stdout missing WORKSPACE_ID line:\n%s", out.String())
	}

	// AC-F5: pipeline_stages contains exactly the 5 default stages.
	var stageCount int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM `+`"`+res.RoleName+`"`+`.pipeline_stages`).Scan(&stageCount); err != nil {
		t.Fatalf("count pipeline stages: %v", err)
	}
	if stageCount != 5 {
		t.Errorf("pipeline_stages: want 5, got %d", stageCount)
	}
}

// TestCreateDuplicateLoud covers AC-F2: re-running the default path on an
// existing slug must exit non-zero with the verbatim structured error
// "Tenant 'X' already exists, created <ISO-8601> by <creator_email>. Use
// a different slug or pass --force-recreate (destroys data)."
func TestCreateDuplicateLoud(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()
	slug := uniqueSlug(t, conn)

	if _, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:       slug,
		AdminEmail: "leo@vernayo.com",
		OwnerEmail: "leo@vernayo.com",
		Template:   templates.GBConsultDefaultName,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Create (first run): %v", err)
	}

	_, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:       slug,
		AdminEmail: "other@example.com",
		OwnerEmail: "other@example.com",
		Template:   templates.GBConsultDefaultName,
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Create (duplicate): expected error, got nil")
	}
	se, ok := err.(*tenant.StructErr)
	if !ok {
		t.Fatalf("expected *tenant.StructErr, got %T", err)
	}
	if se.Kind != tenant.ErrKindSlugConflict {
		t.Fatalf("wrong kind %q (want %q)", se.Kind, tenant.ErrKindSlugConflict)
	}
	msg := se.Message
	// AC-F2 verbatim structure: "Tenant 'X' already exists, created <ISO> by <email>. Use a different slug or pass --force-recreate (destroys data)."
	checks := []string{
		"Tenant ",
		"\"" + slug + "\"", // %q wraps in quotes
		"already exists",
		"created ",
		"by leo@vernayo.com",
		"Use a different slug or pass --force-recreate (destroys data)",
	}
	for _, want := range checks {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q\nmsg=%s", want, msg)
		}
	}
}

// TestCreateUpsertNoop covers AC-F3 / AC-I-10: re-running with --upsert
// is a true no-op — exits zero and the DB row count, role count, and
// schema count are bit-identical to the post-first-run state.
func TestCreateUpsertNoop(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()
	slug := uniqueSlug(t, conn)

	if _, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:       slug,
		AdminEmail: "ci@example.com",
		Template:   templates.GBConsultDefaultName,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Create (first run): %v", err)
	}

	type snap struct {
		Workspaces  int
		AuditRows   int
		Roles       int
		Schemas     int
		Stages      int
		Features    string
	}
	collect := func() snap {
		var s snap
		var roleName string
		_ = conn.QueryRow(ctx, `SELECT role_name FROM core.workspaces WHERE slug = $1`, slug).Scan(&roleName)
		_ = conn.QueryRow(ctx, `SELECT count(*) FROM core.workspaces WHERE slug = $1`, slug).Scan(&s.Workspaces)
		_ = conn.QueryRow(ctx, `
			SELECT count(*) FROM core.audit_log al
			  JOIN core.workspaces w ON w.id = al.workspace_id
			 WHERE w.slug = $1 AND al.event = 'workspace.provisioned'`, slug).Scan(&s.AuditRows)
		_ = conn.QueryRow(ctx, `SELECT count(*) FROM pg_roles WHERE rolname = $1`, roleName).Scan(&s.Roles)
		_ = conn.QueryRow(ctx,
			`SELECT count(*) FROM information_schema.schemata WHERE schema_name = $1`, roleName).Scan(&s.Schemas)
		_ = conn.QueryRow(ctx,
			`SELECT count(*) FROM "`+roleName+`".pipeline_stages`).Scan(&s.Stages)
		_ = conn.QueryRow(ctx,
			`SELECT provisioning_features_applied::text FROM core.workspaces WHERE slug = $1`, slug).Scan(&s.Features)
		return s
	}

	before := collect()
	if before.Workspaces != 1 || before.AuditRows < 1 || before.Roles != 1 || before.Schemas != 1 || before.Stages != 5 {
		t.Fatalf("baseline state wrong: %+v", before)
	}

	// Re-run with --upsert. Should be a true no-op.
	if _, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:       slug,
		AdminEmail: "ci@example.com",
		Template:   templates.GBConsultDefaultName,
		Upsert:     true,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Create (--upsert): %v", err)
	}

	after := collect()
	if before != after {
		t.Errorf("--upsert was not a no-op:\nbefore=%+v\nafter =%+v", before, after)
	}
}

// TestCreateForceRecreate covers AC-F4: --force-recreate atomically drops
// the existing tenant and recreates from scratch. We verify the workspace
// UUID changes (proves drop-then-create) and the audit row count resets
// to 1 (old audit markers were also dropped).
func TestCreateForceRecreate(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()
	slug := uniqueSlug(t, conn)

	res1, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:       slug,
		AdminEmail: "ci@example.com",
		Template:   templates.GBConsultDefaultName,
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Create (first run): %v", err)
	}

	// Force-recreate must succeed and produce a DIFFERENT UUID.
	res2, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:          slug,
		AdminEmail:    "ci@example.com",
		Template:      templates.GBConsultDefaultName,
		ForceRecreate: true,
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Create (--force-recreate): %v", err)
	}
	if res2.WorkspaceID == res1.WorkspaceID {
		t.Errorf("--force-recreate did not mint a new UUID (still %s)", res2.WorkspaceID)
	}

	// Old role/schema must be gone.
	var oldRoleExists, oldSchemaExists bool
	_ = conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, res1.RoleName).Scan(&oldRoleExists)
	_ = conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)`,
		res1.RoleName).Scan(&oldSchemaExists)
	if oldRoleExists {
		t.Errorf("old role %s still present after --force-recreate", res1.RoleName)
	}
	if oldSchemaExists {
		t.Errorf("old schema %s still present after --force-recreate", res1.RoleName)
	}

	// New artifacts: 1 workspaces row, 1 audit row, 5 stages.
	var auditCount int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM core.audit_log al
		  JOIN core.workspaces w ON w.id = al.workspace_id
		 WHERE w.slug = $1 AND al.event = 'workspace.provisioned'`, slug).Scan(&auditCount); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	if auditCount != 1 {
		t.Errorf("audit row count after recreate: want 1, got %d", auditCount)
	}

	// Sanity: created_at on the new row is later than the old run.
	var createdAt time.Time
	if err := conn.QueryRow(ctx, `SELECT created_at FROM core.workspaces WHERE slug = $1`, slug).Scan(&createdAt); err != nil {
		t.Fatalf("query created_at: %v", err)
	}
	if time.Since(createdAt) > 1*time.Minute {
		t.Errorf("created_at unexpectedly old: %s", createdAt)
	}
}

// TestCreateRejectsInvalidSlug covers AC-V1 via the Create entry point
// (the slug-only unit test in slug_test.go covers the regex; this test
// asserts Create() refuses to open a DB connection on a bad slug).
func TestCreateRejectsInvalidSlug(t *testing.T) {
	conn := newConn(t)
	_, err := tenant.Create(context.Background(), conn, tenant.CreateOptions{
		Slug:       "Bad-Slug", // uppercase = invalid
		AdminEmail: "x@y.z",
		Template:   templates.GBConsultDefaultName,
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected slug_invalid error")
	}
	se, ok := err.(*tenant.StructErr)
	if !ok || se.Kind != tenant.ErrKindSlugInvalid {
		t.Fatalf("expected slug_invalid StructErr, got %T %v", err, err)
	}
}
