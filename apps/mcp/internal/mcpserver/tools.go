package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/capability"
	"github.com/gbconsult/lecrm/apps/mcp/internal/store"
)

// toolName enumerates the MCP tools the adapter exposes (ADR-011/012 — the
// AI-native interface seam). The first six are read-only primitives; the
// last three are the read-write *intent composites* (ADR-012 §3), each
// encapsulating one user story as a single safe, idempotent, auditable call.
// The write tools are advertised and dispatched only when a write surface is
// configured (Server.writer != nil); otherwise the server is read-only.
const (
	toolReadContact        = "read_contact"
	toolListContacts       = "list_contacts"
	toolReadDeal           = "read_deal"
	toolListDeals          = "list_deals"
	toolListPipelineStages = "list_pipeline_stages"
	toolSearchContacts     = "search_contacts"

	toolAdvanceDeal    = "advance_deal"
	toolLogInteraction = "log_interaction"
	toolCaptureLead    = "capture_lead"
)

// toolCatalog returns the tools/list payload. Schemas are valid JSON Schema
// objects so MCP clients can render argument forms, and descriptions are
// written for the LLM — the description IS the interface (ADR-012 §2). The
// three write tools are appended only when a write surface is configured.
func (s *Server) toolCatalog() []toolDef {
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
	tools := []toolDef{
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
	if s.writer != nil {
		tools = append(tools, writeToolDefs()...)
	}
	return tools
}

// writeToolDefs returns the three intent write tools' definitions. Each
// description tells the model exactly when to reach for the tool, what it
// composes, and that it supports dry_run / idempotency_key — because for an
// LLM the description is the contract (ADR-012 §2).
func writeToolDefs() []toolDef {
	// Common safety controls every write tool accepts (ADR-012 §6).
	common := map[string]any{
		"dry_run": map[string]any{
			"type":        "boolean",
			"description": "When true, returns a preview of the would-be effect (a diff) and mutates nothing. Destructive variants return a confirmation_token to echo on the real call.",
		},
		"idempotency_key": map[string]any{
			"type":        "string",
			"description": "Optional. A stable key for this logical intent; a duplicate call within 24h replays the cached result instead of acting twice. Use it when retrying.",
		},
		"confirmation_token": map[string]any{
			"type":        "string",
			"description": "Echo the token from a prior dry_run to authorise a destructive effect (e.g. advancing a deal with mark_closed_at).",
		},
	}
	merge := func(extra map[string]any) map[string]any {
		props := map[string]any{}
		for k, v := range common {
			props[k] = v
		}
		for k, v := range extra {
			props[k] = v
		}
		return props
	}

	return []toolDef{
		{
			Name: toolAdvanceDeal,
			Description: "Move a deal to a new pipeline stage and record the change on its timeline — the conversational way to operate the pipeline (e.g. \"mark the Acme deal won, they signed today\"). " +
				"`deal` is a deal UUID or a fuzzy title match; `to_stage` is a stage UUID or a fuzzy stage name (e.g. \"gagné\" matches \"Gagné / Perdu\"). " +
				"Set `mark_closed_at` to close the deal (\"today\"/\"now\" or YYYY-MM-DD) — this is destructive and requires the dry_run→confirmation_token handshake. Add an optional `note` for context.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": merge(map[string]any{
					"deal":           map[string]any{"type": "string", "description": "Deal UUID, or a fuzzy substring of the deal title."},
					"to_stage":       map[string]any{"type": "string", "description": "Target pipeline stage: a stage UUID or a fuzzy stage name."},
					"note":           map[string]any{"type": "string", "description": "Optional note recorded with the stage change."},
					"mark_closed_at": map[string]any{"type": "string", "description": "Optional. Close the deal as of this date (\"today\", \"now\", or YYYY-MM-DD). Destructive — needs confirmation."},
				}),
				"required": []string{"deal", "to_stage"},
			},
		},
		{
			Name: toolLogInteraction,
			Description: "Record an interaction (call, email, meeting, chat) against a contact or company and append it to the timeline. " +
				"`contact_or_company` is a UUID, an email address, or a name — if it names a person who isn't in the CRM yet, the contact is created. " +
				"`summary` is the interaction text; `outcome` is an optional disposition (e.g. \"left voicemail\", \"interested\", \"not a fit\").",
			InputSchema: map[string]any{
				"type": "object",
				"properties": merge(map[string]any{
					"contact_or_company": map[string]any{"type": "string", "description": "Contact/company UUID, an email address, or a name. A missing person is upserted."},
					"summary":            map[string]any{"type": "string", "description": "What happened in the interaction."},
					"outcome":            map[string]any{"type": "string", "description": "Optional disposition / result of the interaction."},
				}),
				"required": []string{"contact_or_company", "summary"},
			},
		},
		{
			Name: toolCaptureLead,
			Description: "Turn a conversation into a CRM lead: upsert a contact (deduplicated by email), optionally attach a company, and open a deal in the first pipeline stage. " +
				"This is the same capability the chatboting connector uses for inbound candidates — exposed for any LLM frontend. " +
				"`name` is the lead's full name; `email` (optional) is used to dedupe; `company` (optional) is found or created and linked; `source` records where the lead came from (e.g. \"website-chat\", \"voice\").",
			InputSchema: map[string]any{
				"type": "object",
				"properties": merge(map[string]any{
					"name":    map[string]any{"type": "string", "description": "The lead's full name."},
					"email":   map[string]any{"type": "string", "description": "Optional. Used to deduplicate against existing contacts."},
					"company": map[string]any{"type": "string", "description": "Optional company name; found-or-created and linked."},
					"source":  map[string]any{"type": "string", "description": "Where the lead came from (channel/campaign)."},
				}),
				"required": []string{"name", "source"},
			},
		},
	}
}

// dispatchTool routes a tools/call to the read or write surface and returns
// the encoded result. A returned error is surfaced to the client as an
// isError tool result (not a transport-level JSON-RPC error) per MCP
// semantics — so a read-only-scope denial, a validation failure, or a
// "confirmation required" all reach the agent as a readable message.
func (s *Server) dispatchTool(ctx context.Context, ws uuid.UUID, scopes []string, name string, args json.RawMessage) (any, error) {
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

	case toolAdvanceDeal:
		return s.dispatchAdvanceDeal(ctx, ws, scopes, args)
	case toolLogInteraction:
		return s.dispatchLogInteraction(ctx, ws, scopes, args)
	case toolCaptureLead:
		return s.dispatchCaptureLead(ctx, ws, scopes, args)

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

// --- write tool dispatch ---

// errWriteDisabled is returned when a write tool is called on a read-only
// deployment (no write surface configured).
const errWriteDisabled = writeDisabledError("this MCP server is read-only; write tools are not enabled")

type writeDisabledError string

func (e writeDisabledError) Error() string { return string(e) }

func (s *Server) dispatchAdvanceDeal(ctx context.Context, ws uuid.UUID, scopes []string, raw json.RawMessage) (any, error) {
	if s.writer == nil {
		return nil, errWriteDisabled
	}
	var a struct {
		writeCommonArgs
		Deal         string          `json:"deal"`
		ToStage      string          `json:"to_stage"`
		Note         *string         `json:"note"`
		MarkClosedAt json.RawMessage `json:"mark_closed_at"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	in := capability.AdvanceDealInput{
		Deal:         a.Deal,
		ToStage:      a.ToStage,
		Note:         a.Note,
		MarkClosedAt: parseFlexClosedAt(a.MarkClosedAt),
	}
	res, err := s.writer.AdvanceDeal(ctx, ws, scopes, in, a.opts())
	if err != nil {
		return nil, err
	}
	return writeResultOut(res)
}

func (s *Server) dispatchLogInteraction(ctx context.Context, ws uuid.UUID, scopes []string, raw json.RawMessage) (any, error) {
	if s.writer == nil {
		return nil, errWriteDisabled
	}
	var a struct {
		writeCommonArgs
		ContactOrCompany string  `json:"contact_or_company"`
		Summary          string  `json:"summary"`
		Outcome          *string `json:"outcome"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	in := capability.LogInteractionInput{
		ContactOrCompany: a.ContactOrCompany,
		Summary:          a.Summary,
		Outcome:          a.Outcome,
	}
	res, err := s.writer.LogInteraction(ctx, ws, scopes, in, a.opts())
	if err != nil {
		return nil, err
	}
	return writeResultOut(res)
}

func (s *Server) dispatchCaptureLead(ctx context.Context, ws uuid.UUID, scopes []string, raw json.RawMessage) (any, error) {
	if s.writer == nil {
		return nil, errWriteDisabled
	}
	var a struct {
		writeCommonArgs
		Name    string  `json:"name"`
		Email   *string `json:"email"`
		Company *string `json:"company"`
		Source  string  `json:"source"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	in := capability.CaptureLeadInput{
		Name:    a.Name,
		Email:   a.Email,
		Company: a.Company,
		Source:  a.Source,
	}
	res, err := s.writer.CaptureLead(ctx, ws, scopes, in, a.opts())
	if err != nil {
		return nil, err
	}
	return writeResultOut(res)
}

// writeCommonArgs are the cross-cutting safety controls every write tool
// accepts (ADR-012 §6). Embedded into each tool's argument struct.
type writeCommonArgs struct {
	DryRun            bool   `json:"dry_run"`
	ConfirmationToken string `json:"confirmation_token"`
	IdempotencyKey    string `json:"idempotency_key"`
}

func (w writeCommonArgs) opts() capability.WriteOptions {
	return capability.WriteOptions{
		DryRun:            w.DryRun,
		ConfirmationToken: w.ConfirmationToken,
		IdempotencyKey:    w.IdempotencyKey,
	}
}

// writeResultOut maps a capability WriteResult to the value the JSON-RPC layer
// encodes: a Preview for a dry-run, otherwise the canonical mutation body
// (identical bytes on a fresh write and on an idempotent replay).
func writeResultOut(res capability.WriteResult) (any, error) {
	if res.Preview != nil {
		return res.Preview, nil
	}
	if len(res.Body) == 0 {
		return map[string]any{"ok": true}, nil
	}
	var data any
	if err := json.Unmarshal(res.Body, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// parseFlexClosedAt normalises the polymorphic mark_closed_at argument: an
// LLM may send a boolean (true → "now"), a string ("today"/YYYY-MM-DD), or
// omit it. Returns nil when absent/false/null so AdvanceDeal stays
// non-destructive unless a close was actually requested.
func parseFlexClosedAt(raw json.RawMessage) *string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		if !b {
			return nil
		}
		now := "now"
		return &now
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return &s
	}
	return nil
}

// --- argument decoding ---

// decodeArgs unmarshals tool arguments, tolerating an absent/empty arguments
// object (treated as all-zero).
func decodeArgs(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

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
