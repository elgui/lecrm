// Package store provides read-only CRM access for the MCP adapter.
//
// Every query runs inside a READ-ONLY transaction that first issues
// `SET LOCAL ROLE workspace_<hex>_ro` so the constrained per-workspace
// role (created by migration 0013) is the effective principal. The
// pool's login user is `lecrm_cube_reader` (ADR-009 §9): a NOLOGIN-free
// role with membership in every workspace_<id>_ro role and NO write
// privileges of its own. This is the same SET-ROLE-per-query pattern
// Cube.dev uses, reused here so the MCP surface inherits the exact
// "SELECT-only, single-workspace" guarantee without a second role.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a single-row read matches nothing.
var ErrNotFound = errors.New("not found")

// Reader is the read surface the MCP tools depend on. The interface
// seam lets the JSON-RPC layer be unit-tested with a fake; the pgx
// implementation below is exercised by integration tests against a
// real Postgres.
type Reader interface {
	ReadContact(ctx context.Context, ws uuid.UUID, id uuid.UUID) (Contact, error)
	ListContacts(ctx context.Context, ws uuid.UUID, p Page) (Contacts, error)
	ReadDeal(ctx context.Context, ws uuid.UUID, id uuid.UUID) (Deal, error)
	ListDeals(ctx context.Context, ws uuid.UUID, p Page) (Deals, error)
	ListPipelineStages(ctx context.Context, ws uuid.UUID) ([]Stage, error)
	SearchContacts(ctx context.Context, ws uuid.UUID, query string) ([]Contact, error)
}

// Page is an opaque-cursor pagination request. Cursor is the id of the
// last row from the previous page (keyset pagination on created_at,id).
type Page struct {
	Limit  int
	Cursor uuid.UUID // uuid.Nil for the first page
}

const (
	defaultLimit = 50
	maxLimit     = 200
)

func (p Page) limit() int {
	if p.Limit <= 0 {
		return defaultLimit
	}
	if p.Limit > maxLimit {
		return maxLimit
	}
	return p.Limit
}

// Contact is a CRM contact plus its custom properties.
type Contact struct {
	ID         uuid.UUID      `json:"id"`
	FirstName  string         `json:"first_name"`
	LastName   string         `json:"last_name"`
	Email      *string        `json:"email"`
	Phone      *string        `json:"phone"`
	CompanyID  *string        `json:"company_id"`
	Properties map[string]any `json:"custom_properties,omitempty"`
}

// Contacts is one page of contacts.
type Contacts struct {
	Data       []Contact `json:"data"`
	NextCursor *string   `json:"next_cursor"`
	HasMore    bool      `json:"has_more"`
}

// Deal is a CRM deal plus stage info and custom properties.
type Deal struct {
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

// Deals is one page of deals.
type Deals struct {
	Data       []Deal  `json:"data"`
	NextCursor *string `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
}

// Stage is a pipeline stage with the count of deals currently in it.
type Stage struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	OrderIndex int32     `json:"order_index"`
	DealCount  int64     `json:"deal_count"`
}

// PG is the production Reader backed by a pgx pool connected as
// lecrm_cube_reader.
type PG struct {
	Pool *pgxpool.Pool
}

// SchemaName derives the per-workspace schema name from a workspace
// UUID, matching core.lecrm_provision_workspace
// (workspace_<hex> where <hex> is the dashless lowercase UUID).
func SchemaName(ws uuid.UUID) string {
	hex := strings.ReplaceAll(ws.String(), "-", "")
	return "workspace_" + hex
}

// RoleName derives the constrained per-workspace RO role name. It is the
// schema name with a `_ro` suffix (migration 0013).
//
// Both identifiers contain only [0-9a-f_] characters by construction
// (uuid.String is hex+dashes; we strip the dashes), so they are safe to
// interpolate after pgx.Identifier sanitisation.
func RoleName(ws uuid.UUID) string {
	return SchemaName(ws) + "_ro"
}

// withWorkspace opens a read-only transaction, assumes the workspace RO
// role, pins the search_path to the workspace schema, and runs fn.
//
// Both SET LOCAL statements are scoped to the transaction, so the
// connection reverts to the lecrm_cube_reader login defaults on
// commit/rollback — no role or search_path leakage across pooled
// connections. The explicit search_path is required: `SET ROLE` changes
// privileges but does NOT apply the target role's ALTER ROLE search_path
// (that only happens at login), so unqualified table names would
// otherwise resolve against the reader's default path.
func (s *PG) withWorkspace(ctx context.Context, ws uuid.UUID, fn func(pgx.Tx) error) error {
	if ws == uuid.Nil {
		return errors.New("store: nil workspace id")
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	role := pgx.Identifier{RoleName(ws)}.Sanitize()
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+role); err != nil {
		return fmt.Errorf("set role: %w", err)
	}
	schema := pgx.Identifier{SchemaName(ws)}.Sanitize()
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+schema); err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PG) ReadContact(ctx context.Context, ws, id uuid.UUID) (Contact, error) {
	var c Contact
	err := s.withWorkspace(ctx, ws, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT id, first_name, last_name, email, phone, company_id::text
			   FROM contacts WHERE id = $1`, id)
		if e := row.Scan(&c.ID, &c.FirstName, &c.LastName, &c.Email, &c.Phone, &c.CompanyID); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return e
		}
		props, e := customProperties(ctx, tx, "contact", c.ID)
		if e != nil {
			return e
		}
		c.Properties = props
		return nil
	})
	return c, err
}

func (s *PG) ListContacts(ctx context.Context, ws uuid.UUID, p Page) (Contacts, error) {
	limit := p.limit()
	var out Contacts
	err := s.withWorkspace(ctx, ws, func(tx pgx.Tx) error {
		// Keyset pagination on (created_at, id). The cursor row supplies
		// the boundary; passing uuid.Nil with a sentinel timestamp scans
		// from the newest row.
		rows, e := tx.Query(ctx,
			`SELECT id, first_name, last_name, email, phone, company_id::text
			   FROM contacts
			  WHERE ($1::uuid IS NULL OR (created_at, id) <
			         (SELECT created_at, id FROM contacts WHERE id = $1))
			  ORDER BY created_at DESC, id DESC
			  LIMIT $2`, cursorArg(p.Cursor), limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var c Contact
			if e := rows.Scan(&c.ID, &c.FirstName, &c.LastName, &c.Email, &c.Phone, &c.CompanyID); e != nil {
				return e
			}
			out.Data = append(out.Data, c)
		}
		return rows.Err()
	})
	if err != nil {
		return Contacts{}, err
	}
	out.paginate(limit)
	return out, nil
}

func (s *PG) ReadDeal(ctx context.Context, ws, id uuid.UUID) (Deal, error) {
	var d Deal
	err := s.withWorkspace(ctx, ws, func(tx pgx.Tx) error {
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
		props, e := customProperties(ctx, tx, "deal", d.ID)
		if e != nil {
			return e
		}
		d.Properties = props
		return nil
	})
	return d, err
}

func (s *PG) ListDeals(ctx context.Context, ws uuid.UUID, p Page) (Deals, error) {
	limit := p.limit()
	var out Deals
	err := s.withWorkspace(ctx, ws, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx,
			`SELECT d.id, d.title, d.amount, d.currency, d.stage_id::text,
			        s.name, d.contact_id::text, d.company_id::text
			   FROM deals d
			   LEFT JOIN pipeline_stages s ON s.id = d.stage_id
			  WHERE ($1::uuid IS NULL OR (d.created_at, d.id) <
			         (SELECT created_at, id FROM deals WHERE id = $1))
			  ORDER BY d.created_at DESC, d.id DESC
			  LIMIT $2`, cursorArg(p.Cursor), limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var d Deal
			if e := rows.Scan(&d.ID, &d.Title, &d.Amount, &d.Currency, &d.StageID,
				&d.StageName, &d.ContactID, &d.CompanyID); e != nil {
				return e
			}
			out.Data = append(out.Data, d)
		}
		return rows.Err()
	})
	if err != nil {
		return Deals{}, err
	}
	out.paginate(limit)
	return out, nil
}

func (s *PG) ListPipelineStages(ctx context.Context, ws uuid.UUID) ([]Stage, error) {
	var out []Stage
	err := s.withWorkspace(ctx, ws, func(tx pgx.Tx) error {
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
			var st Stage
			if e := rows.Scan(&st.ID, &st.Name, &st.OrderIndex, &st.DealCount); e != nil {
				return e
			}
			out = append(out, st)
		}
		return rows.Err()
	})
	return out, err
}

func (s *PG) SearchContacts(ctx context.Context, ws uuid.UUID, query string) ([]Contact, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	var out []Contact
	err := s.withWorkspace(ctx, ws, func(tx pgx.Tx) error {
		// Postgres full-text search over name+email. websearch_to_tsquery
		// tolerates arbitrary user input (no syntax errors on stray
		// operators), and a trailing prefix match handles partial words.
		rows, e := tx.Query(ctx,
			`SELECT id, first_name, last_name, email, phone, company_id::text
			   FROM contacts
			  WHERE to_tsvector('simple',
			          coalesce(first_name,'') || ' ' ||
			          coalesce(last_name,'')  || ' ' ||
			          coalesce(email,''))
			        @@ websearch_to_tsquery('simple', $1)
			  ORDER BY created_at DESC
			  LIMIT $2`, query, maxLimit)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var c Contact
			if e := rows.Scan(&c.ID, &c.FirstName, &c.LastName, &c.Email, &c.Phone, &c.CompanyID); e != nil {
				return e
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// customProperties reads the JSONB custom-property bag for a parent
// record from the objects table (ADR-010 §5 storage convention).
func customProperties(ctx context.Context, tx pgx.Tx, parentType string, parentID uuid.UUID) (map[string]any, error) {
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

// cursorArg converts a uuid.Nil cursor into a NULL parameter so the
// keyset predicate selects from the first page.
func cursorArg(c uuid.UUID) *uuid.UUID {
	if c == uuid.Nil {
		return nil
	}
	return &c
}

func (c *Contacts) paginate(limit int) {
	if len(c.Data) > limit {
		last := c.Data[limit-1]
		c.Data = c.Data[:limit]
		s := last.ID.String()
		c.NextCursor = &s
		c.HasMore = true
	}
	if c.Data == nil {
		c.Data = []Contact{}
	}
}

func (d *Deals) paginate(limit int) {
	if len(d.Data) > limit {
		last := d.Data[limit-1]
		d.Data = d.Data[:limit]
		s := last.ID.String()
		d.NextCursor = &s
		d.HasMore = true
	}
	if d.Data == nil {
		d.Data = []Deal{}
	}
}
