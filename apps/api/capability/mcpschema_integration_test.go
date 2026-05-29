//go:build integration

// Integration tests for the self-describing workspace schema MCP Resource
// (ADR-012 §5/§9), exercised against a real Postgres. They cover what the unit
// tests cannot: that MCPWorkspaceSchema reads the connecting workspace's own
// custom_property_definitions through the per-workspace read-only role, that a
// second workspace sees only its own definitions (no cross-workspace leak),
// and that a workspace with no definitions returns a valid empty schema.
//
// Run (docker required):
//
//	~/.local/go/bin/go -C apps/api test -tags integration -count 1 -race -v \
//	    -run TestSchema ./capability
package capability

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// seedDefinition inserts one custom_property_definition into a workspace
// schema. allowed is marshalled to jsonb (nil → SQL NULL).
func (e *intentEnv) seedDefinition(t *testing.T, ws uuid.UUID, parentType, key, propType string, allowed []string, required bool) {
	t.Helper()
	var allowedJSON any
	if len(allowed) > 0 {
		b, err := json.Marshal(allowed)
		if err != nil {
			t.Fatalf("marshal allowed: %v", err)
		}
		allowedJSON = b
	}
	if _, err := e.pool.Exec(context.Background(),
		`INSERT INTO `+pgIdent(e.schema(ws))+`.custom_property_definitions
		   (parent_type, property_key, property_type, allowed_values, required)
		 VALUES ($1, $2, $3, $4, $5)`,
		parentType, key, propType, allowedJSON, required); err != nil {
		t.Fatalf("seed definition %s/%s: %v", parentType, key, err)
	}
}

func TestSchema_ReturnsOwnDefinitions(t *testing.T) {
	e := setupIntentEnv(t)
	ctx := context.Background()

	e.seedDefinition(t, e.wsA, "contact", "lead_score", "number", nil, false)
	e.seedDefinition(t, e.wsA, "contact", "cms", "enum", []string{"wordpress", "shopify"}, true)
	e.seedDefinition(t, e.wsA, "deal", "renewal", "boolean", nil, false)

	schema, err := e.svc.MCPWorkspaceSchema(ctx, MCPReadPrincipal(e.wsA))
	if err != nil {
		t.Fatalf("MCPWorkspaceSchema: %v", err)
	}
	if schema.WorkspaceID != e.wsA.String() {
		t.Fatalf("workspace_id = %q, want %q", schema.WorkspaceID, e.wsA)
	}
	if len(schema.Contact) != 2 {
		t.Fatalf("want 2 contact defs, got %d: %+v", len(schema.Contact), schema.Contact)
	}
	if len(schema.Deal) != 1 {
		t.Fatalf("want 1 deal def, got %d: %+v", len(schema.Deal), schema.Deal)
	}
	// Ordered by property_key: cms before lead_score.
	if schema.Contact[0].Key != "cms" || schema.Contact[1].Key != "lead_score" {
		t.Fatalf("contact defs not ordered by key: %+v", schema.Contact)
	}
	cms := schema.Contact[0]
	if cms.Type != "enum" || !cms.Required || len(cms.AllowedValues) != 2 {
		t.Fatalf("cms def wrong: %+v", cms)
	}
}

func TestSchema_PerWorkspaceIsolation(t *testing.T) {
	e := setupIntentEnv(t)
	ctx := context.Background()

	e.seedDefinition(t, e.wsA, "contact", "a_only", "string", nil, false)
	e.seedDefinition(t, e.wsB, "contact", "b_only", "string", nil, false)

	a, err := e.svc.MCPWorkspaceSchema(ctx, MCPReadPrincipal(e.wsA))
	if err != nil {
		t.Fatalf("schema A: %v", err)
	}
	b, err := e.svc.MCPWorkspaceSchema(ctx, MCPReadPrincipal(e.wsB))
	if err != nil {
		t.Fatalf("schema B: %v", err)
	}
	if len(a.Contact) != 1 || a.Contact[0].Key != "a_only" {
		t.Fatalf("ws A leaked or missing its def: %+v", a.Contact)
	}
	if len(b.Contact) != 1 || b.Contact[0].Key != "b_only" {
		t.Fatalf("ws B leaked or missing its def: %+v", b.Contact)
	}
	// Neither workspace must see the other's field.
	for _, d := range a.Contact {
		if d.Key == "b_only" {
			t.Fatal("ws A leaked ws B's schema")
		}
	}
}

func TestSchema_EmptyWorkspaceReturnsEmptyShape(t *testing.T) {
	e := setupIntentEnv(t)
	ctx := context.Background()

	schema, err := e.svc.MCPWorkspaceSchema(ctx, MCPReadPrincipal(e.wsA))
	if err != nil {
		t.Fatalf("MCPWorkspaceSchema: %v", err)
	}
	if schema.Contact == nil || schema.Deal == nil {
		t.Fatalf("empty schema groups must be non-nil slices: %+v", schema)
	}
	if len(schema.Contact) != 0 || len(schema.Deal) != 0 {
		t.Fatalf("fresh workspace must have no definitions: %+v", schema)
	}
}
