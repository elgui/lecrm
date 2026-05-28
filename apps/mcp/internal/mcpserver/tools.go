package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/mcp/internal/store"
)

// toolName enumerates the MCP tools the adapter exposes (ADR-011 — the
// AI-native interface seam). All are read-only.
const (
	toolReadContact        = "read_contact"
	toolListContacts       = "list_contacts"
	toolReadDeal           = "read_deal"
	toolListDeals          = "list_deals"
	toolListPipelineStages = "list_pipeline_stages"
	toolSearchContacts     = "search_contacts"
)

// toolCatalog returns the tools/list payload. Schemas are minimal but
// valid JSON Schema objects so MCP clients can render argument forms.
func toolCatalog() []toolDef {
	idProp := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string", "description": "entity UUID"},
		},
		"required": []string{"id"},
	}
	pageProps := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"cursor": map[string]any{"type": "string", "description": "opaque pagination cursor from a prior page"},
			"limit":  map[string]any{"type": "integer", "description": "page size (default 50, max 200)"},
		},
	}
	return []toolDef{
		{Name: toolReadContact, Description: "Read one contact with its custom properties.", InputSchema: idProp},
		{Name: toolListContacts, Description: "List contacts, newest first, with cursor pagination.", InputSchema: pageProps},
		{Name: toolReadDeal, Description: "Read one deal with stage info and custom properties.", InputSchema: idProp},
		{Name: toolListDeals, Description: "List deals, newest first, with cursor pagination.", InputSchema: pageProps},
		{Name: toolListPipelineStages, Description: "List all pipeline stages with deal counts.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}},
		{Name: toolSearchContacts, Description: "Full-text search contacts by name or email.", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"query": map[string]any{"type": "string"}},
			"required":   []string{"query"},
		}},
	}
}

// dispatchTool routes a tools/call to the store and returns the encoded
// result. A returned error is surfaced to the client as an isError tool
// result (not a transport-level JSON-RPC error) per MCP semantics.
func (s *Server) dispatchTool(ctx context.Context, ws uuid.UUID, name string, args json.RawMessage) (any, error) {
	switch name {
	case toolReadContact:
		id, err := argID(args)
		if err != nil {
			return nil, err
		}
		c, err := s.reader.ReadContact(ctx, ws, id)
		if err != nil {
			return nil, mapNotFound(err, "contact")
		}
		return c, nil

	case toolListContacts:
		p, err := argPage(args)
		if err != nil {
			return nil, err
		}
		return s.reader.ListContacts(ctx, ws, p)

	case toolReadDeal:
		id, err := argID(args)
		if err != nil {
			return nil, err
		}
		d, err := s.reader.ReadDeal(ctx, ws, id)
		if err != nil {
			return nil, mapNotFound(err, "deal")
		}
		return d, nil

	case toolListDeals:
		p, err := argPage(args)
		if err != nil {
			return nil, err
		}
		return s.reader.ListDeals(ctx, ws, p)

	case toolListPipelineStages:
		stages, err := s.reader.ListPipelineStages(ctx, ws)
		if err != nil {
			return nil, err
		}
		return map[string]any{"data": stages}, nil

	case toolSearchContacts:
		q, err := argQuery(args)
		if err != nil {
			return nil, err
		}
		hits, err := s.reader.SearchContacts(ctx, ws, q)
		if err != nil {
			return nil, err
		}
		return map[string]any{"data": hits}, nil

	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
}

func mapNotFound(err error, kind string) error {
	if err == store.ErrNotFound {
		return fmt.Errorf("%s not found", kind)
	}
	return err
}

// --- argument decoding ---

func argID(raw json.RawMessage) (uuid.UUID, error) {
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return uuid.Nil, fmt.Errorf("invalid arguments: %w", err)
	}
	id, err := uuid.Parse(a.ID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid id: %q", a.ID)
	}
	return id, nil
}

func argPage(raw json.RawMessage) (store.Page, error) {
	var a struct {
		Cursor string `json:"cursor"`
		Limit  int    `json:"limit"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return store.Page{}, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	p := store.Page{Limit: a.Limit}
	if a.Cursor != "" {
		c, err := uuid.Parse(a.Cursor)
		if err != nil {
			return store.Page{}, fmt.Errorf("invalid cursor: %q", a.Cursor)
		}
		p.Cursor = c
	}
	return p, nil
}

func argQuery(raw json.RawMessage) (string, error) {
	var a struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Query == "" {
		return "", fmt.Errorf("query is required")
	}
	return a.Query, nil
}
