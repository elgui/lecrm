// Package capability – MCP self-describing workspace schema (ADR-012 §5, §9).
//
// A generic CRM MCP server exposes a fixed schema. leCRM has a per-workspace
// metadata engine (ADR-010): each workspace defines its own custom-property
// definitions on contacts and deals. Exposing the *connecting* workspace's
// actual definitions lets an LLM discover and use the real fields (e.g. that
// this workspace tracks `cms`, `geo`, `lead_score` on contacts) instead of
// guessing — the differentiator a closed CRM structurally cannot offer.
//
// This is the read backing the `lecrm://workspace/schema` MCP Resource. Like
// the other MCP reads it runs through (*Service).readTx, so the per-workspace
// read-only role is assumed and Postgres enforces SELECT-only access. The
// search_path is pinned to the workspace schema inside the transaction, so the
// query reads exactly the caller's own custom_property_definitions and can
// never see another workspace's schema.
package capability

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// MCPPropertyDef is one custom-property definition in the agent-facing
// schema view. It is serialised compactly for LLM consumption: allowed_values
// and required are omitted when empty/false so the payload carries only the
// fields that constrain the property.
type MCPPropertyDef struct {
	Key           string   `json:"key"`
	Type          string   `json:"type"`
	AllowedValues []string `json:"allowed_values,omitempty"`
	Required      bool     `json:"required,omitempty"`
}

// MCPWorkspaceSchema is the self-describing custom-property schema for one
// workspace, grouped by parent type (ADR-012 §5). Empty groups serialise as
// empty arrays (never null), so an agent always receives a valid, complete
// shape — a workspace with no custom properties returns `{"contact":[],"deal":[]}`.
type MCPWorkspaceSchema struct {
	WorkspaceID string           `json:"workspace_id"`
	Contact     []MCPPropertyDef `json:"contact"`
	Deal        []MCPPropertyDef `json:"deal"`
}

// MCPWorkspaceSchema returns the connecting workspace's custom-property
// definitions for contacts and deals. Scoped to the Principal's workspace via
// readTx (RO role + search_path), so it can never leak another workspace's
// schema.
func (s *Service) MCPWorkspaceSchema(ctx context.Context, p Principal) (MCPWorkspaceSchema, error) {
	if err := authorize(p, RoleMember); err != nil {
		return MCPWorkspaceSchema{}, err
	}
	out := MCPWorkspaceSchema{
		WorkspaceID: p.WorkspaceID.String(),
		Contact:     []MCPPropertyDef{},
		Deal:        []MCPPropertyDef{},
	}
	err := s.readTx(ctx, p, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx,
			`SELECT parent_type, property_key, property_type, allowed_values, required
			   FROM custom_property_definitions
			  ORDER BY parent_type, property_key`)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var parentType string
			var def MCPPropertyDef
			var allowedRaw []byte
			if e := rows.Scan(&parentType, &def.Key, &def.Type, &allowedRaw, &def.Required); e != nil {
				return e
			}
			if len(allowedRaw) > 0 {
				if e := json.Unmarshal(allowedRaw, &def.AllowedValues); e != nil {
					return e
				}
			}
			switch parentType {
			case "contact":
				out.Contact = append(out.Contact, def)
			case "deal":
				out.Deal = append(out.Deal, def)
			}
		}
		return rows.Err()
	})
	if err != nil {
		return MCPWorkspaceSchema{}, err
	}
	return out, nil
}
