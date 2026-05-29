package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// resourceWorkspaceSchema is the URI of the self-describing custom-property
// schema Resource (ADR-012 §5/§9). It is the leCRM differentiator: a generic
// CRM MCP exposes a fixed schema, whereas this returns the *connecting*
// workspace's actual custom-property definitions so an LLM uses real fields.
const resourceWorkspaceSchema = "lecrm://workspace/schema"

// resourceCatalog returns the resources/list payload. The description is
// written for the LLM — it tells the model to read the schema before setting
// or interpreting custom properties (ADR-012 §2: the description IS the
// interface).
func (s *Server) resourceCatalog() []resourceDef {
	return []resourceDef{
		{
			URI:  resourceWorkspaceSchema,
			Name: "workspace-schema",
			Description: "This workspace's custom-property schema: the custom fields defined on contacts and deals " +
				"(key, type, allowed values for enums, and whether required). Read it before setting or interpreting " +
				"custom properties so you use real fields with valid values. The schema is workspace-specific — different " +
				"workspaces define different fields.",
			MimeType: "application/json",
		},
	}
}

// dispatchResource resolves a resources/read URI to its contents, scoped to
// the caller's workspace. Unknown URIs return an error the JSON-RPC layer maps
// to an invalid-params response. The body is compact JSON (no indent) for
// token efficiency.
func (s *Server) dispatchResource(ctx context.Context, ws uuid.UUID, uri string) (resourceContents, error) {
	switch uri {
	case resourceWorkspaceSchema:
		schema, err := s.reader.WorkspaceSchema(ctx, ws)
		if err != nil {
			return resourceContents{}, err
		}
		body, err := json.Marshal(schema)
		if err != nil {
			return resourceContents{}, err
		}
		return resourceContents{URI: uri, MimeType: "application/json", Text: string(body)}, nil
	default:
		return resourceContents{}, fmt.Errorf("unknown resource %q", uri)
	}
}
