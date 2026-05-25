//go:build integration

package tenant_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestProvisionRejectsNullUUID(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()

	var roleName string
	err := conn.QueryRow(ctx,
		`SELECT core.lecrm_provision_workspace(NULL::uuid)`,
	).Scan(&roleName)
	if err == nil {
		t.Fatal("expected error for NULL UUID, got success")
	}
	if !strings.Contains(err.Error(), "must not be NULL") {
		t.Errorf("unexpected error message: %v", err)
	}

	// No partial state: no role or schema should exist for a NULL input.
}

func TestProvisionRejectsZeroUUID(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()

	zeroUUID := uuid.UUID{}
	var roleName string
	err := conn.QueryRow(ctx,
		`SELECT core.lecrm_provision_workspace($1)`, zeroUUID,
	).Scan(&roleName)
	if err == nil {
		t.Fatal("expected error for zero UUID, got success")
	}
	if !strings.Contains(err.Error(), "zero UUID") {
		t.Errorf("unexpected error message: %v", err)
	}

	// Verify no partial state: zero UUID role must not exist.
	zeroRole := "workspace_" + strings.ReplaceAll(zeroUUID.String(), "-", "")
	var exists bool
	_ = conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, zeroRole,
	).Scan(&exists)
	if exists {
		t.Errorf("role %s was created despite zero UUID rejection", zeroRole)
		// Clean up if it leaked.
		_, _ = conn.Exec(ctx, "DROP ROLE IF EXISTS "+zeroRole)
	}
}

func TestRegistryRejectsInvalidSlug(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()

	cases := []struct {
		name string
		slug string
	}{
		{"uppercase", "Bad-Slug"},
		{"too-short", "ab"},
		{"starts-with-digit", "1foo"},
		{"starts-with-hyphen", "-foo"},
		{"too-long", "a" + strings.Repeat("b", 32)},
		{"empty", ""},
		{"special-chars", "foo_bar!"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testID, _ := uuid.NewV7()
			var roleName string
			err := conn.QueryRow(ctx,
				`SELECT core.lecrm_provision_workspace_with_registry($1, $2, $3, $4, $5)`,
				testID, tc.slug, "ci@example.com", "ci@example.com", "",
			).Scan(&roleName)
			if err == nil {
				t.Fatalf("expected error for slug %q, got success (role=%s)", tc.slug, roleName)
			}
			if !strings.Contains(err.Error(), "p_slug must match") {
				t.Errorf("unexpected error for slug %q: %v", tc.slug, err)
			}

			// No partial state: the role must not have been created.
			testRole := "workspace_" + strings.ReplaceAll(testID.String(), "-", "")
			var exists bool
			_ = conn.QueryRow(ctx,
				`SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, testRole,
			).Scan(&exists)
			if exists {
				t.Errorf("role %s was created despite slug rejection (DDL ran before validation)", testRole)
				_, _ = conn.Exec(ctx, "DROP ROLE IF EXISTS "+testRole)
			}
		})
	}
}

func TestRegistryRejectsInvalidEmail(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()

	testID, _ := uuid.NewV7()
	slug := uniqueSlug(t, conn)

	var roleName string
	err := conn.QueryRow(ctx,
		`SELECT core.lecrm_provision_workspace_with_registry($1, $2, $3, $4, $5)`,
		testID, slug, "not-an-email", "ci@example.com", "",
	).Scan(&roleName)
	if err == nil {
		t.Fatal("expected error for invalid admin_email, got success")
	}
	if !strings.Contains(err.Error(), "must contain @") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRegistryAcceptsEmptyEmail(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()

	slug := uniqueSlug(t, conn)
	testID, _ := uuid.NewV7()

	var roleName string
	err := conn.QueryRow(ctx,
		`SELECT core.lecrm_provision_workspace_with_registry($1, $2, $3, $4, $5)`,
		testID, slug, "", "", "",
	).Scan(&roleName)
	if err != nil {
		t.Fatalf("bootstrap path (empty emails) should succeed: %v", err)
	}
	if roleName == "" {
		t.Error("expected non-empty role_name")
	}
}

func TestRegistryRejectsUnknownTemplate(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()

	slug := uniqueSlug(t, conn)
	testID, _ := uuid.NewV7()

	var roleName string
	err := conn.QueryRow(ctx,
		`SELECT core.lecrm_provision_workspace_with_registry($1, $2, $3, $4, $5)`,
		testID, slug, "ci@example.com", "ci@example.com", "nonexistent-template",
	).Scan(&roleName)
	if err == nil {
		t.Fatal("expected error for unknown template, got success")
	}
	if !strings.Contains(err.Error(), "unknown template") {
		t.Errorf("unexpected error message: %v", err)
	}

	// No partial state: the role must not have been created.
	testRole := "workspace_" + strings.ReplaceAll(testID.String(), "-", "")
	var exists bool
	_ = conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, testRole,
	).Scan(&exists)
	if exists {
		t.Errorf("role %s was created despite template rejection", testRole)
		_, _ = conn.Exec(ctx, "DROP ROLE IF EXISTS "+testRole)
	}
}

func TestRegistryRejectsNullSlug(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()

	testID, _ := uuid.NewV7()
	var roleName string
	err := conn.QueryRow(ctx,
		`SELECT core.lecrm_provision_workspace_with_registry($1, NULL, $2, $3, $4)`,
		testID, "ci@example.com", "ci@example.com", "",
	).Scan(&roleName)
	if err == nil {
		t.Fatal("expected error for NULL slug, got success")
	}
	if !strings.Contains(err.Error(), "p_slug must match") {
		t.Errorf("unexpected error message: %v", err)
	}
}
