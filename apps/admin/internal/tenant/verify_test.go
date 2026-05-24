//go:build integration

package tenant_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gbconsult/lecrm/apps/admin/internal/tenant"
	"github.com/gbconsult/lecrm/apps/admin/internal/tenant/templates"
)

// TestVerifyInvariants exercises AC-I-01..AC-I-13 (AC-I-14 deferred) by
// provisioning a tenant and running the verify subcommand. The check
// passes only when all 14 invariants report [OK] and the exit-equivalent
// returns Failed == 0.
func TestVerifyInvariants(t *testing.T) {
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

	var out bytes.Buffer
	res, err := tenant.Verify(ctx, conn, tenant.VerifyOptions{
		Slug:        slug,
		AllFailures: true,
	}, &out)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Failed != 0 {
		t.Errorf("verify reported %d/%d failures:\n%s", res.Failed, res.Total, out.String())
	}
	if res.Total != 14 {
		t.Errorf("expected 14 invariants total, got %d", res.Total)
	}

	// AC-VFY-4: every line must start with [OK] or [FAIL] + INV-XX.
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	for _, line := range lines {
		if !(strings.HasPrefix(line, "[OK] INV-") || strings.HasPrefix(line, "[FAIL] INV-")) {
			t.Errorf("non-conforming verify line: %q", line)
		}
	}

	// AC-VFY-1 / D12: INV-05 line uses operator vocabulary ("Tenant"),
	// not the internal "workspace" name.
	wantInv05 := "[OK] INV-05 Tenant registry row exists"
	if !strings.Contains(out.String(), wantInv05) {
		t.Errorf("verify output missing %q (D12 vocabulary check)\n%s", wantInv05, out.String())
	}
}

// TestVerifyMissingSlug confirms verify errors loudly when the tenant
// doesn't exist (operator typo, wrong host, etc.).
func TestVerifyMissingSlug(t *testing.T) {
	conn := newConn(t)
	_, err := tenant.Verify(context.Background(), conn, tenant.VerifyOptions{
		Slug: "no-such-tenant-xyz",
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for unknown slug")
	}
	se, ok := err.(*tenant.StructErr)
	if !ok || se.Kind != tenant.ErrKindSlugConflict {
		t.Fatalf("expected slug_conflict StructErr, got %T %v", err, err)
	}
}
