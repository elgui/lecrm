//go:build integration

package config_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gbconsult/lecrm/apps/admin/internal/config"
)

// TestReplayEndToEnd provisions two tenants, applies a methodology config
// to the source, replays it onto the destination, and verifies the
// destination ends up with identical methodology config AND the matching
// custom_property_definitions rows.
func TestReplayEndToEnd(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()

	srcSlug := uniqueSlug(t)
	dstSlug := uniqueSlug(t)
	provisionTenant(t, conn, srcSlug)
	provisionTenant(t, conn, dstSlug)

	// Apply the default template to source.
	var applyOut bytes.Buffer
	applyRes, err := config.Apply(ctx, conn, config.ApplyOptions{
		Slug:     srcSlug,
		Template: "gbconsult-default",
	}, &applyOut)
	if err != nil {
		t.Fatalf("Apply to source: %v", err)
	}
	if applyRes.VersionSeq != 1 {
		t.Fatalf("Apply version_seq: got %d, want 1", applyRes.VersionSeq)
	}

	// Show source config — baseline for comparison.
	var showOut bytes.Buffer
	srcCfg, err := config.Show(ctx, conn, config.ShowOptions{Slug: srcSlug}, &showOut)
	if err != nil {
		t.Fatalf("Show source: %v", err)
	}

	// Replay source → destination.
	var replayOut bytes.Buffer
	replayRes, err := config.Replay(ctx, conn, config.ReplayOptions{
		SrcSlug: srcSlug,
		DstSlug: dstSlug,
	}, &replayOut)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if replayRes.VersionSeq != 1 {
		t.Fatalf("Replay version_seq: got %d, want 1", replayRes.VersionSeq)
	}

	// Show destination config.
	var dstShowOut bytes.Buffer
	dstCfg, err := config.Show(ctx, conn, config.ShowOptions{Slug: dstSlug}, &dstShowOut)
	if err != nil {
		t.Fatalf("Show destination: %v", err)
	}

	// Methodology content must be identical (ignore version_seq which resets).
	srcCfg.VersionSeq = 0
	dstCfg.VersionSeq = 0
	srcJSON, _ := json.Marshal(srcCfg)
	dstJSON, _ := json.Marshal(dstCfg)
	if string(srcJSON) != string(dstJSON) {
		t.Errorf("replayed config differs from source:\n  src: %s\n  dst: %s", srcJSON, dstJSON)
	}

	// Diff should report zero differences.
	var diffOut bytes.Buffer
	diffRes, err := config.Diff(ctx, conn, config.DiffOptions{
		SlugA: srcSlug,
		SlugB: dstSlug,
	}, &diffOut)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(diffRes.Entries) != 0 {
		t.Errorf("Diff reported %d entries, want 0:\n%s", len(diffRes.Entries), diffOut.String())
	}

	// Verify custom_property_definitions were provisioned on destination.
	dstRef, err := config.ResolveSlug(ctx, conn, dstSlug)
	if err != nil {
		t.Fatalf("ResolveSlug dst: %v", err)
	}
	srcRef, err := config.ResolveSlug(ctx, conn, srcSlug)
	if err != nil {
		t.Fatalf("ResolveSlug src: %v", err)
	}

	srcProps := countProperties(t, conn, srcRef.RoleName)
	dstProps := countProperties(t, conn, dstRef.RoleName)
	if srcProps != dstProps {
		t.Errorf("custom_property_definitions count: src=%d dst=%d", srcProps, dstProps)
	}
	if dstProps == 0 {
		t.Error("destination has zero custom_property_definitions — provisioning failed")
	}

	// Spot-check a specific property exists on destination.
	assertPropertyExists(t, conn, dstRef.RoleName, "deal", "contact_name", "string")
	assertPropertyExists(t, conn, dstRef.RoleName, "deal", "outcome", "enum")
	assertPropertyExists(t, conn, dstRef.RoleName, "deal", "proposal_amount", "number")
}

// TestReplayIdempotent verifies that replaying twice doesn't duplicate
// custom_property_definitions rows (UPSERT semantics).
func TestReplayIdempotent(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()

	srcSlug := uniqueSlug(t)
	dstSlug := uniqueSlug(t)
	provisionTenant(t, conn, srcSlug)
	provisionTenant(t, conn, dstSlug)

	var buf bytes.Buffer
	if _, err := config.Apply(ctx, conn, config.ApplyOptions{
		Slug:     srcSlug,
		Template: "gbconsult-default",
	}, &buf); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// First replay.
	buf.Reset()
	if _, err := config.Replay(ctx, conn, config.ReplayOptions{
		SrcSlug: srcSlug,
		DstSlug: dstSlug,
	}, &buf); err != nil {
		t.Fatalf("Replay 1: %v", err)
	}

	dstRef, err := config.ResolveSlug(ctx, conn, dstSlug)
	if err != nil {
		t.Fatalf("ResolveSlug: %v", err)
	}
	countAfterFirst := countProperties(t, conn, dstRef.RoleName)

	// Second replay.
	buf.Reset()
	res, err := config.Replay(ctx, conn, config.ReplayOptions{
		SrcSlug: srcSlug,
		DstSlug: dstSlug,
	}, &buf)
	if err != nil {
		t.Fatalf("Replay 2: %v", err)
	}
	if res.VersionSeq != 2 {
		t.Errorf("second replay version_seq: got %d, want 2", res.VersionSeq)
	}

	countAfterSecond := countProperties(t, conn, dstRef.RoleName)
	if countAfterFirst != countAfterSecond {
		t.Errorf("property count changed after second replay: %d → %d", countAfterFirst, countAfterSecond)
	}
}

// TestShowVersionSpecific verifies that `config show --version N` retrieves
// the correct historical version after multiple applies.
func TestShowVersionSpecific(t *testing.T) {
	conn := newConn(t)
	ctx := context.Background()

	slug := uniqueSlug(t)
	provisionTenant(t, conn, slug)

	var buf bytes.Buffer
	if _, err := config.Apply(ctx, conn, config.ApplyOptions{
		Slug:     slug,
		Template: "gbconsult-default",
	}, &buf); err != nil {
		t.Fatalf("Apply v1: %v", err)
	}

	buf.Reset()
	if _, err := config.Apply(ctx, conn, config.ApplyOptions{
		Slug:     slug,
		Template: "gbconsult-default",
	}, &buf); err != nil {
		t.Fatalf("Apply v2: %v", err)
	}

	// Latest should be v2.
	buf.Reset()
	latest, err := config.Show(ctx, conn, config.ShowOptions{Slug: slug}, &buf)
	if err != nil {
		t.Fatalf("Show latest: %v", err)
	}
	if latest.VersionSeq != 2 {
		t.Errorf("latest version_seq: got %d, want 2", latest.VersionSeq)
	}

	// Explicit v1 should return v1.
	buf.Reset()
	v1, err := config.Show(ctx, conn, config.ShowOptions{Slug: slug, Version: 1}, &buf)
	if err != nil {
		t.Fatalf("Show v1: %v", err)
	}
	if v1.VersionSeq != 1 {
		t.Errorf("v1 version_seq: got %d, want 1", v1.VersionSeq)
	}
}

func countProperties(t *testing.T, conn *pgx.Conn, roleName string) int {
	t.Helper()
	var n int
	q := `SELECT count(*) FROM "` + roleName + `".custom_property_definitions`
	if err := conn.QueryRow(context.Background(), q).Scan(&n); err != nil {
		t.Fatalf("count custom_property_definitions on %s: %v", roleName, err)
	}
	return n
}

func assertPropertyExists(t *testing.T, conn *pgx.Conn, roleName, parentType, key, propType string) {
	t.Helper()
	q := `SELECT property_type FROM "` + roleName + `".custom_property_definitions
	      WHERE parent_type = $1 AND property_key = $2`
	var got string
	err := conn.QueryRow(context.Background(), q, parentType, key).Scan(&got)
	if err != nil {
		t.Errorf("property %s/%s not found on %s: %v", parentType, key, roleName, err)
		return
	}
	if got != propType {
		t.Errorf("property %s/%s type: got %q, want %q", parentType, key, got, propType)
	}
}
