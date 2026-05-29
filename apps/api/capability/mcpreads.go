// Package capability – MCP read projection (ADR-012 §1, §10 Increment 1.2).
//
// This file is the single home for the CRM reads the MCP adapter
// (apps/mcp) exposes to AI agents. It used to live as a divergent second
// implementation in apps/mcp/internal/store/store.go; that copy is
// deleted and the MCP binary now *links* this layer as a library (the
// separate-binary topology of ADR-009 §4.2 is preserved — apps/mcp still
// builds and deploys on its own).
//
// These projections deliberately differ from the REST result types
// (ContactResult/DealResult): they fold in each entity's custom-property
// bag (ADR-010 §5), a deal's stage name, and per-stage deal counts —
// the agent-facing shape — and they page on an opaque row-id cursor
// rather than the REST keyset cursor. Keeping them as their own types is
// what lets the MCP wire contract stay byte-for-byte stable while the SQL
// lives in exactly one place.
//
// DB-level read-only guarantee: every method here runs through
// (*Service).readTx, which — when the Principal carries a ReadRole —
// issues `SET LOCAL ROLE workspace_<id>_ro` inside a read-only
// transaction (see Principal.ReadRole and ReadTxAsRole). The MCP adapter
// builds such a Principal via MCPReadPrincipal, so a read-only-scoped
// token can never write, enforced by Postgres rather than by Go.
package capability

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MCP read pagination bounds. Mirror the values the MCP store used before
// consolidation so the tool contract is unchanged.
const (
	mcpDefaultLimit = 50
	mcpMaxLimit     = 200
)

// MCPPage is an opaque-cursor pagination request for the MCP read tools.
// Cursor is the id of the last row from the previous page (keyset
// pagination on (created_at, id)); uuid.Nil requests the first page.
type MCPPage struct {
	Limit  int
	Cursor uuid.UUID
}

func (p MCPPage) limit() int {
	if p.Limit <= 0 {
		return mcpDefaultLimit
	}
	if p.Limit > mcpMaxLimit {
		return mcpMaxLimit
	}
	return p.Limit
}

// MCPContact is a contact plus its custom-property bag (agent view).
type MCPContact struct {
	ID         uuid.UUID      `json:"id"`
	FirstName  string         `json:"first_name"`
	LastName   string         `json:"last_name"`
	Email      *string        `json:"email"`
	Phone      *string        `json:"phone"`
	CompanyID  *string        `json:"company_id"`
	Properties map[string]any `json:"custom_properties,omitempty"`
}

// MCPContacts is one page of contacts.
type MCPContacts struct {
	Data       []MCPContact `json:"data"`
	NextCursor *string      `json:"next_cursor"`
	HasMore    bool         `json:"has_more"`
}

// MCPDeal is a deal plus stage name and custom-property bag (agent view).
type MCPDeal struct {
	ID         uuid.UUID      `json:"id"`
	Title      string         `json:"title"`
	Amount     *float64       `json:"amount"`
	Currency   *string        `json:"currency"`
	StageID    *string        `json:"stage_id"`
	StageName  *string        `json:"stage_name"`
	ContactID  *string        `json:"contact_id"`
	CompanyID  *string        `json:"company_id"`
	Properties map[string]any `json:"custom_properties,omitempty"`
}

// MCPDeals is one page of deals.
type MCPDeals struct {
	Data       []MCPDeal `json:"data"`
	NextCursor *string   `json:"next_cursor"`
	HasMore    bool      `json:"has_more"`
}

// MCPStage is a pipeline stage with the count of deals currently in it.
type MCPStage struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	OrderIndex int32     `json:"order_index"`
	DealCount  int64     `json:"deal_count"`
}

// MCPSchemaName derives the per-workspace schema name from a workspace
// UUID, matching core.lecrm_provision_workspace (workspace_<hex> where
// <hex> is the dashless lowercase UUID).
func MCPSchemaName(ws uuid.UUID) string {
	return "workspace_" + strings.ReplaceAll(ws.String(), "-", "")
}

// MCPReadRole derives the constrained per-workspace RO role name: the
// schema name with a `_ro` suffix (migration 0013). Both identifiers
// contain only [0-9a-f_] by construction, so they are safe to interpolate
// after pgx.Identifier sanitisation.
func MCPReadRole(ws uuid.UUID) string {
	return MCPSchemaName(ws) + "_ro"
}

// MCPReadPrincipal builds the Principal an MCP read tool acts under: it
// scopes to the workspace schema and pins reads to the workspace's
// read-only role, so the DB enforces SELECT-only access. Role is
// RoleMember (reads require RoleMember+); ActorType is the MCP agent.
func MCPReadPrincipal(ws uuid.UUID) Principal {
	return Principal{
		WorkspaceID: ws,
		Schema:      MCPSchemaName(ws),
		ReadRole:    MCPReadRole(ws),
		Role:        RoleMember,
		ActorType:   ActorTypeMCPAgent,
	}
}

// MCPReadContact reads one contact with its custom properties.
func (s *Service) MCPReadContact(ctx context.Context, p Principal, id uuid.UUID) (MCPContact, error) {
	if err := authorize(p, RoleMember); err != nil {
		return MCPContact{}, err
	}
	var c MCPContact
	err := s.readTx(ctx, p, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT id, first_name, last_name, email, phone, company_id::text
			   FROM contacts WHERE id = $1`, id)
		if e := row.Scan(&c.ID, &c.FirstName, &c.LastName, &c.Email, &c.Phone, &c.CompanyID); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return e
		}
		props, e := mcpCustomProperties(ctx, tx, "contact", c.ID)
		if e != nil {
			return e
		}
		c.Properties = props
		return nil
	})
	return c, err
}

// MCPListContacts lists contacts, newest first, with cursor pagination.
func (s *Service) MCPListContacts(ctx context.Context, p Principal, page MCPPage) (MCPContacts, error) {
	if err := authorize(p, RoleMember); err != nil {
		return MCPContacts{}, err
	}
	limit := page.limit()
	var out MCPContacts
	err := s.readTx(ctx, p, func(tx pgx.Tx) error {
		// Keyset pagination on (created_at, id). The cursor row supplies
		// the boundary; uuid.Nil → NULL → scan from the newest row.
		rows, e := tx.Query(ctx,
			`SELECT id, first_name, last_name, email, phone, company_id::text
			   FROM contacts
			  WHERE ($1::uuid IS NULL OR (created_at, id) <
			         (SELECT created_at, id FROM contacts WHERE id = $1))
			  ORDER BY created_at DESC, id DESC
			  LIMIT $2`, mcpCursorArg(page.Cursor), limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var c MCPContact
			if e := rows.Scan(&c.ID, &c.FirstName, &c.LastName, &c.Email, &c.Phone, &c.CompanyID); e != nil {
				return e
			}
			out.Data = append(out.Data, c)
		}
		return rows.Err()
	})
	if err != nil {
		return MCPContacts{}, err
	}
	paginateContacts(&out, limit)
	return out, nil
}

// MCPReadDeal reads one deal with stage info and custom properties.
func (s *Service) MCPReadDeal(ctx context.Context, p Principal, id uuid.UUID) (MCPDeal, error) {
	if err := authorize(p, RoleMember); err != nil {
		return MCPDeal{}, err
	}
	var d MCPDeal
	err := s.readTx(ctx, p, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT d.id, d.title, d.amount, d.currency, d.stage_id::text,
			        s.name, d.contact_id::text, d.company_id::text
			   FROM deals d
			   LEFT JOIN pipeline_stages s ON s.id = d.stage_id
			  WHERE d.id = $1`, id)
		if e := row.Scan(&d.ID, &d.Title, &d.Amount, &d.Currency, &d.StageID,
			&d.StageName, &d.ContactID, &d.CompanyID); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return e
		}
		props, e := mcpCustomProperties(ctx, tx, "deal", d.ID)
		if e != nil {
			return e
		}
		d.Properties = props
		return nil
	})
	return d, err
}

// MCPListDeals lists deals, newest first, with cursor pagination.
func (s *Service) MCPListDeals(ctx context.Context, p Principal, page MCPPage) (MCPDeals, error) {
	if err := authorize(p, RoleMember); err != nil {
		return MCPDeals{}, err
	}
	limit := page.limit()
	var out MCPDeals
	err := s.readTx(ctx, p, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx,
			`SELECT d.id, d.title, d.amount, d.currency, d.stage_id::text,
			        s.name, d.contact_id::text, d.company_id::text
			   FROM deals d
			   LEFT JOIN pipeline_stages s ON s.id = d.stage_id
			  WHERE ($1::uuid IS NULL OR (d.created_at, d.id) <
			         (SELECT created_at, id FROM deals WHERE id = $1))
			  ORDER BY d.created_at DESC, d.id DESC
			  LIMIT $2`, mcpCursorArg(page.Cursor), limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var d MCPDeal
			if e := rows.Scan(&d.ID, &d.Title, &d.Amount, &d.Currency, &d.StageID,
				&d.StageName, &d.ContactID, &d.CompanyID); e != nil {
				return e
			}
			out.Data = append(out.Data, d)
		}
		return rows.Err()
	})
	if err != nil {
		return MCPDeals{}, err
	}
	paginateDeals(&out, limit)
	return out, nil
}

// MCPListPipelineStages lists all pipeline stages with deal counts.
func (s *Service) MCPListPipelineStages(ctx context.Context, p Principal) ([]MCPStage, error) {
	if err := authorize(p, RoleMember); err != nil {
		return nil, err
	}
	var out []MCPStage
	err := s.readTx(ctx, p, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx,
			`SELECT s.id, s.name, s.order_index, count(d.id)
			   FROM pipeline_stages s
			   LEFT JOIN deals d ON d.stage_id = s.id
			  GROUP BY s.id, s.name, s.order_index
			  ORDER BY s.order_index`)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var st MCPStage
			if e := rows.Scan(&st.ID, &st.Name, &st.OrderIndex, &st.DealCount); e != nil {
				return e
			}
			out = append(out, st)
		}
		return rows.Err()
	})
	return out, err
}

// MCPSearchContacts runs a full-text search over contact name + email.
func (s *Service) MCPSearchContacts(ctx context.Context, p Principal, query string) ([]MCPContact, error) {
	if err := authorize(p, RoleMember); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	var out []MCPContact
	err := s.readTx(ctx, p, func(tx pgx.Tx) error {
		// websearch_to_tsquery tolerates arbitrary user input (no syntax
		// errors on stray operators).
		rows, e := tx.Query(ctx,
			`SELECT id, first_name, last_name, email, phone, company_id::text
			   FROM contacts
			  WHERE to_tsvector('simple',
			          coalesce(first_name,'') || ' ' ||
			          coalesce(last_name,'')  || ' ' ||
			          coalesce(email,''))
			        @@ websearch_to_tsquery('simple', $1)
			  ORDER BY created_at DESC
			  LIMIT $2`, query, mcpMaxLimit)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var c MCPContact
			if e := rows.Scan(&c.ID, &c.FirstName, &c.LastName, &c.Email, &c.Phone, &c.CompanyID); e != nil {
				return e
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// mcpCustomProperties reads the JSONB custom-property bag for a parent
// record from the objects table (ADR-010 §5 storage convention).
func mcpCustomProperties(ctx context.Context, tx pgx.Tx, parentType string, parentID uuid.UUID) (map[string]any, error) {
	var raw map[string]any
	err := tx.QueryRow(ctx,
		`SELECT data FROM objects
		  WHERE object_type = 'custom_properties' AND parent_type = $1 AND parent_id = $2`,
		parentType, parentID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// mcpCursorArg converts a uuid.Nil cursor into a NULL parameter so the
// keyset predicate selects from the first page.
func mcpCursorArg(c uuid.UUID) *uuid.UUID {
	if c == uuid.Nil {
		return nil
	}
	return &c
}

func paginateContacts(c *MCPContacts, limit int) {
	if len(c.Data) > limit {
		last := c.Data[limit-1]
		c.Data = c.Data[:limit]
		s := last.ID.String()
		c.NextCursor = &s
		c.HasMore = true
	}
	if c.Data == nil {
		c.Data = []MCPContact{}
	}
}

func paginateDeals(d *MCPDeals, limit int) {
	if len(d.Data) > limit {
		last := d.Data[limit-1]
		d.Data = d.Data[:limit]
		s := last.ID.String()
		d.NextCursor = &s
		d.HasMore = true
	}
	if d.Data == nil {
		d.Data = []MCPDeal{}
	}
}
