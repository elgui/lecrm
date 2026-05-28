//go:build integration

package config_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/gbconsult/lecrm/apps/admin/internal/audit"
	"github.com/gbconsult/lecrm/apps/admin/internal/config"
)

// TestApplyEmitsAuditRow verifies that the Phase 2 Apply path now
// writes a core.audit_log entry (Phase 3 done-criterion #3) with
// actor_type=human_api and operator_email surfaced from env.
func TestApplyEmitsAuditRow(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()

	t.Setenv(config.OperatorEmailEnv, "leo@vernayo.com")

	slug := uniqueSlug(t)
	provisionTenant(t, conn, slug)

	var buf bytes.Buffer
	if _, err := config.Apply(ctx, conn, config.ApplyOptions{
		Slug:     slug,
		Template: "gbconsult-default",
	}, &buf); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	entries, err := audit.Query(ctx, conn, audit.Filter{
		Slug:  slug,
		Event: "config.template.applied",
	})
	if err != nil {
		t.Fatalf("audit.Query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 audit row for config.template.applied, got %d", len(entries))
	}
	e := entries[0]
	if e.ActorType != "human_api" {
		t.Errorf("actor_type: got %q want human_api", e.ActorType)
	}
	if e.Payload["operator_email"] != "leo@vernayo.com" {
		t.Errorf("operator_email: got %v want leo@vernayo.com", e.Payload["operator_email"])
	}
	if e.Payload["template"] != "gbconsult-default" {
		t.Errorf("template: got %v want gbconsult-default", e.Payload["template"])
	}
	if e.Payload["slug"] != slug {
		t.Errorf("slug: got %v want %s", e.Payload["slug"], slug)
	}
}

// TestReplayEmitsAuditRow verifies Replay writes an audit row attributed
// to the destination tenant referencing the source slug.
func TestReplayEmitsAuditRow(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()

	_ = os.Unsetenv(config.OperatorEmailEnv) // operator unset → "unknown"

	srcSlug := uniqueSlug(t)
	dstSlug := uniqueSlug(t)
	provisionTenant(t, conn, srcSlug)
	provisionTenant(t, conn, dstSlug)

	var buf bytes.Buffer
	if _, err := config.Apply(ctx, conn, config.ApplyOptions{
		Slug:     srcSlug,
		Template: "gbconsult-default",
	}, &buf); err != nil {
		t.Fatalf("Apply src: %v", err)
	}
	buf.Reset()
	if _, err := config.Replay(ctx, conn, config.ReplayOptions{
		SrcSlug: srcSlug,
		DstSlug: dstSlug,
	}, &buf); err != nil {
		t.Fatalf("Replay: %v", err)
	}

	entries, err := audit.Query(ctx, conn, audit.Filter{
		Slug:  dstSlug,
		Event: "config.template.replayed",
	})
	if err != nil {
		t.Fatalf("audit.Query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 replay audit row on dst, got %d", len(entries))
	}
	e := entries[0]
	if e.ActorType != "human_api" {
		t.Errorf("actor_type: got %q want human_api", e.ActorType)
	}
	if e.Payload["src_slug"] != srcSlug {
		t.Errorf("src_slug: got %v want %s", e.Payload["src_slug"], srcSlug)
	}
	if e.Payload["operator_email"] != "unknown" {
		t.Errorf("operator_email: got %v want unknown", e.Payload["operator_email"])
	}
}

// TestAuditQueryUnknownSlug verifies the resolver returns ErrUnknownSlug.
func TestAuditQueryUnknownSlug(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()
	_, err := audit.Query(ctx, conn, audit.Filter{Slug: "no-such-tenant-xyz"})
	if err != audit.ErrUnknownSlug {
		t.Fatalf("want ErrUnknownSlug, got %v", err)
	}
}
