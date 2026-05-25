//go:build integration

package tenant_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/gbconsult/lecrm/apps/admin/internal/tenant"
	"github.com/gbconsult/lecrm/apps/admin/internal/tenant/templates"
)

// TestTombstoneWorkspace exercises the happy path: tombstoning a live
// workspace makes it unavailable for resolution and re-provisioning.
func TestTombstoneWorkspace(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()
	slug := uniqueSlug(t, conn)

	// Provision a tenant first.
	if _, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:       slug,
		AdminEmail: "ci@example.com",
		Template:   templates.GBConsultDefaultName,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Tombstone it.
	var out bytes.Buffer
	_, err := tenant.Tombstone(ctx, conn, tenant.TombstoneOptions{Slug: slug}, &out)
	if err != nil {
		t.Fatalf("Tombstone: %v", err)
	}

	// Verify tombstoned_at is set.
	var tombstonedAtNotNull bool
	if err := conn.QueryRow(ctx,
		`SELECT tombstoned_at IS NOT NULL FROM core.workspaces WHERE slug = $1`, slug).
		Scan(&tombstonedAtNotNull); err != nil {
		t.Fatalf("query tombstoned_at: %v", err)
	}
	if !tombstonedAtNotNull {
		t.Fatal("tombstoned_at should be set after tombstoning")
	}

	// Attempt to re-provision with the same slug — must fail.
	_, err = tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:       slug,
		AdminEmail: "other@example.com",
		Template:   templates.GBConsultDefaultName,
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Create after tombstone: expected error, got nil")
	}
	var se *tenant.StructErr
	if !errors.As(err, &se) {
		t.Fatalf("expected *tenant.StructErr, got %T", err)
	}
	if se.Kind != tenant.ErrKindSlugTombstoned {
		t.Fatalf("expected slug_tombstoned, got %q: %s", se.Kind, se.Message)
	}
}

// TestTombstoneAlreadyTombstoned verifies double-tombstone is rejected.
func TestTombstoneAlreadyTombstoned(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()
	slug := uniqueSlug(t, conn)

	if _, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:       slug,
		AdminEmail: "ci@example.com",
		Template:   templates.GBConsultDefaultName,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := tenant.Tombstone(ctx, conn, tenant.TombstoneOptions{Slug: slug}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Tombstone (first): %v", err)
	}

	_, err := tenant.Tombstone(ctx, conn, tenant.TombstoneOptions{Slug: slug}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Tombstone (second): expected error, got nil")
	}
	var se *tenant.StructErr
	if !errors.As(err, &se) || se.Kind != tenant.ErrKindSlugTombstoned {
		t.Fatalf("expected slug_tombstoned, got %T %v", err, err)
	}
}

// TestCreateRejectsReservedSlug verifies that infrastructure slugs seeded
// in migration 0005 cannot be provisioned.
func TestCreateRejectsReservedSlug(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()

	_, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:       "admin",
		AdminEmail: "ci@example.com",
		Template:   templates.GBConsultDefaultName,
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Create with reserved slug: expected error, got nil")
	}
	var se *tenant.StructErr
	if !errors.As(err, &se) {
		t.Fatalf("expected *tenant.StructErr, got %T", err)
	}
	if se.Kind != tenant.ErrKindSlugReserved {
		t.Fatalf("expected slug_reserved, got %q: %s", se.Kind, se.Message)
	}
}

// TestCreateRejectsReservedSlugWithUpsert verifies --upsert also checks reserved slugs.
func TestCreateRejectsReservedSlugWithUpsert(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()

	_, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:       "staging",
		AdminEmail: "ci@example.com",
		Template:   templates.GBConsultDefaultName,
		Upsert:     true,
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Create --upsert with reserved slug: expected error, got nil")
	}
	var se *tenant.StructErr
	if !errors.As(err, &se) || se.Kind != tenant.ErrKindSlugReserved {
		t.Fatalf("expected slug_reserved, got %T %v", err, err)
	}
}

// TestCreateRejectsReservedSlugWithForceRecreate verifies --force-recreate also checks reserved slugs.
func TestCreateRejectsReservedSlugWithForceRecreate(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()

	_, err := tenant.Create(ctx, conn, tenant.CreateOptions{
		Slug:          "api",
		AdminEmail:    "ci@example.com",
		Template:      templates.GBConsultDefaultName,
		ForceRecreate: true,
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Create --force-recreate with reserved slug: expected error, got nil")
	}
	var se *tenant.StructErr
	if !errors.As(err, &se) || se.Kind != tenant.ErrKindSlugReserved {
		t.Fatalf("expected slug_reserved, got %T %v", err, err)
	}
}
