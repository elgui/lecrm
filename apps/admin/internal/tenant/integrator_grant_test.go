//go:build integration

package tenant_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gbconsult/lecrm/apps/admin/internal/integrator"
	"github.com/gbconsult/lecrm/apps/admin/internal/tenant"
	"github.com/gbconsult/lecrm/apps/admin/internal/tenant/templates"
)

// grantEmails returns the integrator-grant emails recorded for a slug.
func grantEmails(t *testing.T, conn *pgx.Conn, slug string) []string {
	t.Helper()
	rows, err := conn.Query(context.Background(),
		`SELECT g.email
		   FROM core.integrator_grants g
		   JOIN core.workspaces w ON w.id = g.workspace_id
		  WHERE w.slug = $1
		  ORDER BY g.email`, slug)
	if err != nil {
		t.Fatalf("query grants for %s: %v", slug, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			t.Fatalf("scan grant: %v", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate grants: %v", err)
	}
	return out
}

// TestProvisionAutoGrantsIntegrator asserts that provisioning with
// --owner-email writes a matching core.integrator_grants row in the same
// flow as provisioning, before the integrator has ever logged in.
func TestProvisionAutoGrantsIntegrator(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()
	slug := uniqueSlug(t, conn)

	var out bytes.Buffer
	res, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:          slug,
		AdminEmail:    "contact@client.example",
		OwnerEmail:    "Leo@Vernayo.com", // mixed case — uniqueness is case-insensitive
		OperatorEmail: "ops@gbconsult.me",
		Template:      templates.GBConsultDefaultName,
	}, &out)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Stdout advertises the auto-grant.
	if !strings.Contains(out.String(), "[PROVISION] integrator grant: Leo@Vernayo.com") {
		t.Errorf("stdout missing integrator-grant line:\n%s", out.String())
	}

	emails := grantEmails(t, conn, slug)
	if len(emails) != 1 || emails[0] != "Leo@Vernayo.com" {
		t.Fatalf("expected exactly one grant for Leo@Vernayo.com, got %v", emails)
	}

	// The grant points at the provisioned workspace UUID.
	var gotID uuid.UUID
	if err := conn.QueryRow(ctx,
		`SELECT workspace_id FROM core.integrator_grants WHERE lower(email) = lower($1)`,
		"leo@vernayo.com").Scan(&gotID); err != nil {
		t.Fatalf("lookup grant workspace_id: %v", err)
	}
	if gotID != res.WorkspaceID {
		t.Errorf("grant workspace_id %s != provisioned %s", gotID, res.WorkspaceID)
	}
}

// TestProvisionWithoutOwnerEmailNoGrant asserts the client's own admin email
// is NOT auto-granted integrator access when --owner-email is omitted.
func TestProvisionWithoutOwnerEmailNoGrant(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()
	slug := uniqueSlug(t, conn)

	if _, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:       slug,
		AdminEmail: "admin@client.example",
		Template:   templates.GBConsultDefaultName,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if emails := grantEmails(t, conn, slug); len(emails) != 0 {
		t.Fatalf("expected no integrator grants, got %v", emails)
	}
}

// TestProvisionUpsertGrantIdempotent asserts re-provisioning with --upsert
// does not duplicate the integrator grant.
func TestProvisionUpsertGrantIdempotent(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()
	slug := uniqueSlug(t, conn)

	mk := func(upsert bool) {
		if _, err := tenant.Create(ctx, conn, tenant.CreateOptions{
			Slug:       slug,
			AdminEmail: "contact@client.example",
			OwnerEmail: "leo@vernayo.com",
			Template:   templates.GBConsultDefaultName,
			Upsert:     upsert,
		}, &bytes.Buffer{}); err != nil {
			t.Fatalf("Create(upsert=%v): %v", upsert, err)
		}
	}
	mk(false)
	mk(true)

	if emails := grantEmails(t, conn, slug); len(emails) != 1 {
		t.Fatalf("expected exactly one grant after upsert re-run, got %v", emails)
	}
}

// TestIntegratorGrantRevokeListRoundTrip exercises the standalone CLI-backed
// package for a tenant the integrator did not provision.
func TestIntegratorGrantRevokeListRoundTrip(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()
	slug := uniqueSlug(t, conn)

	// Provision WITHOUT owner-email so no auto-grant exists yet.
	if _, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:       slug,
		AdminEmail: "admin@client.example",
		Template:   templates.GBConsultDefaultName,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// grant
	if err := integrator.Grant(ctx, conn, integrator.GrantOptions{
		Slug:      slug,
		Email:     "leo@vernayo.com",
		GrantedBy: "ops@gbconsult.me",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if emails := grantEmails(t, conn, slug); len(emails) != 1 || emails[0] != "leo@vernayo.com" {
		t.Fatalf("after grant, want [leo@vernayo.com], got %v", emails)
	}

	// grant is idempotent (case-insensitive)
	if err := integrator.Grant(ctx, conn, integrator.GrantOptions{
		Slug:  slug,
		Email: "LEO@vernayo.com",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Grant (repeat): %v", err)
	}
	if emails := grantEmails(t, conn, slug); len(emails) != 1 {
		t.Fatalf("repeat grant duplicated row: %v", emails)
	}

	// list (filtered by slug) shows the grant
	var listOut bytes.Buffer
	if err := integrator.List(ctx, conn, integrator.ListOptions{Slug: slug}, &listOut); err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(listOut.String(), "leo@vernayo.com") || !strings.Contains(listOut.String(), slug) {
		t.Errorf("list output missing grant:\n%s", listOut.String())
	}

	// revoke (case-insensitive) removes it
	if err := integrator.Revoke(ctx, conn, integrator.RevokeOptions{
		Slug:  slug,
		Email: "Leo@Vernayo.com",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if emails := grantEmails(t, conn, slug); len(emails) != 0 {
		t.Fatalf("after revoke, want none, got %v", emails)
	}

	// revoke again is a no-op (idempotent), not an error
	var revOut bytes.Buffer
	if err := integrator.Revoke(ctx, conn, integrator.RevokeOptions{
		Slug:  slug,
		Email: "leo@vernayo.com",
	}, &revOut); err != nil {
		t.Fatalf("Revoke (repeat): %v", err)
	}
	if !strings.Contains(revOut.String(), "nothing to revoke") {
		t.Errorf("repeat revoke should report nothing to revoke:\n%s", revOut.String())
	}
}

// TestIntegratorGrantUnknownSlug asserts a loud tenant_not_found error.
func TestIntegratorGrantUnknownSlug(t *testing.T) {
	conn := newConn(t)
	err := integrator.Grant(context.Background(), conn, integrator.GrantOptions{
		Slug:  "no-such-tenant-xyz",
		Email: "leo@vernayo.com",
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for unknown slug")
	}
	var se *tenant.StructErr
	if !errors.As(err, &se) || se.Kind != tenant.ErrKindTenantNotFound {
		t.Fatalf("expected tenant_not_found StructErr, got %T %v", err, err)
	}
}
