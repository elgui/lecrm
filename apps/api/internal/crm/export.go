package crm

// CSV export — Sprint 9 feature 8 (tasket 20260525-1008).
//
// Per-tenant data export. Every row is read inside the workspace's own
// schema (search_path set by readTx), so a tenant can only ever stream
// its own data — the sovereignty pitch made concrete (ADR-009 §1). Custom
// properties stored as JSONB are flattened into additional columns,
// prefixed `cf_`, so a workspace's bespoke fields travel with the export.
//
// Routes are registered in handlers.go RegisterRoutes alongside the other
// CRUD endpoints, so the same RBAC guard (reads require member+) applies.

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gbconsult/lecrm/apps/api/internal/sqlcgen"
)

// exportPageSize bounds each DB round-trip while draining a full entity
// list. Export is unbounded by design (a tenant exports everything), so we
// page rather than load-all to keep peak memory flat on large workspaces.
const exportPageSize int32 = 500

// customPropPrefix namespaces flattened JSONB columns so they never
// collide with a fixed column header.
const customPropPrefix = "cf_"

// csvFilename builds the Content-Disposition filename, e.g.
// contacts_2026-05-25.csv.
func csvFilename(entity string, now time.Time) string {
	return fmt.Sprintf("%s_%s.csv", entity, now.Format("2006-01-02"))
}

// customPropertyColumns returns the sorted union of custom-property keys
// across all rows. Sorting keeps the column order stable between exports
// (and therefore diff-able), which a timestamp-ordered map iteration would
// not.
func customPropertyColumns(props map[uuid.UUID]map[string]any) []string {
	seen := map[string]struct{}{}
	for _, p := range props {
		for k := range p {
			seen[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(seen))
	for k := range seen {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}

// formatCSVValue renders a JSONB scalar for a CSV cell. Booleans and
// numbers get canonical forms; nil becomes empty; everything else is
// stringified. Composite values (arrays/objects) fall back to a Go literal
// — rare for custom properties, which are scalar-typed by the metadata
// validator, but we never want a panic to abort a tenant's whole export.
func formatCSVValue(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	default:
		return fmt.Sprintf("%v", t)
	}
}

// loadCustomProperties returns parent_id → property map for every record of
// parentType in the current workspace schema. One query, no N+1. Companies
// carry no custom properties (metadata only defines contact/deal), so
// callers pass through an empty map for them.
func loadCustomProperties(ctx context.Context, tx pgx.Tx, parentType string) (map[uuid.UUID]map[string]any, error) {
	out := map[uuid.UUID]map[string]any{}
	rows, err := tx.Query(ctx,
		`SELECT parent_id, data FROM objects WHERE object_type = 'custom_properties' AND parent_type = $1`,
		parentType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var data map[string]any
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		out[id] = data
	}
	return out, rows.Err()
}

// customCells maps a row's properties onto the (ordered) custom columns,
// emitting "" for any column the row doesn't define.
func customCells(props map[string]any, cols []string) []string {
	cells := make([]string, len(cols))
	for i, c := range cols {
		if props != nil {
			cells[i] = formatCSVValue(props[c])
		}
	}
	return cells
}

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func floatOrEmpty(p *float64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatFloat(*p, 'f', -1, 64)
}

func timeOrEmpty(p *time.Time) string {
	if p == nil {
		return ""
	}
	return p.UTC().Format(time.RFC3339)
}

// writeCSVAttachment sets the streaming-download headers shared by every
// export endpoint.
func writeCSVAttachment(w http.ResponseWriter, entity string, now time.Time) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", csvFilename(entity, now)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// ExportContacts streams every contact in the workspace as CSV, with each
// custom property as a cf_-prefixed column.
func (h *Handler) ExportContacts(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}

	var (
		rows  []sqlcgen.Contact
		props map[uuid.UUID]map[string]any
	)
	err := readTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		var cursorTS pgtype.Timestamptz
		var cursorID uuid.UUID
		for {
			page, e := q.ListContacts(r.Context(), sqlcgen.ListContactsParams{
				CursorCreatedAt: cursorTS, CursorID: cursorID, PageLimit: exportPageSize,
			})
			if e != nil {
				return e
			}
			rows = append(rows, page...)
			if int32(len(page)) < exportPageSize {
				break
			}
			last := page[len(page)-1]
			cursorTS = last.CreatedAt
			cursorID = last.ID
		}
		var e error
		props, e = loadCustomProperties(r.Context(), tx, entityTypeContact)
		return e
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "export contacts", "err", err)
		writeErr(w, http.StatusInternalServerError, "export contacts failed")
		return
	}

	cols := customPropertyColumns(props)
	writeCSVAttachment(w, "contacts", time.Now())
	cw := csv.NewWriter(w)
	header := append([]string{"id", "first_name", "last_name", "email", "phone", "company_id", "owner_id", "created_at", "updated_at"}, prefixCols(cols)...)
	_ = cw.Write(header)
	for _, c := range rows {
		resp := contactFromRow(c)
		rec := []string{
			resp.ID.String(), resp.FirstName, resp.LastName,
			strOrEmpty(resp.Email), strOrEmpty(resp.Phone),
			strOrEmpty(resp.CompanyID), strOrEmpty(resp.OwnerID),
			resp.CreatedAt.UTC().Format(time.RFC3339), resp.UpdatedAt.UTC().Format(time.RFC3339),
		}
		rec = append(rec, customCells(props[resp.ID], cols)...)
		_ = cw.Write(rec)
	}
	cw.Flush()
}

// ExportCompanies streams every company in the workspace as CSV.
func (h *Handler) ExportCompanies(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}

	var rows []sqlcgen.Company
	err := readTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		var cursorTS pgtype.Timestamptz
		var cursorID uuid.UUID
		for {
			page, e := q.ListCompanies(r.Context(), sqlcgen.ListCompaniesParams{
				CursorCreatedAt: cursorTS, CursorID: cursorID, PageLimit: exportPageSize,
			})
			if e != nil {
				return e
			}
			rows = append(rows, page...)
			if int32(len(page)) < exportPageSize {
				break
			}
			last := page[len(page)-1]
			cursorTS = last.CreatedAt
			cursorID = last.ID
		}
		return nil
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "export companies", "err", err)
		writeErr(w, http.StatusInternalServerError, "export companies failed")
		return
	}

	writeCSVAttachment(w, "companies", time.Now())
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "name", "domain", "industry", "size", "owner_id", "created_at", "updated_at"})
	for _, c := range rows {
		resp := companyFromRow(c)
		_ = cw.Write([]string{
			resp.ID.String(), resp.Name,
			strOrEmpty(resp.Domain), strOrEmpty(resp.Industry), strOrEmpty(resp.Size),
			strOrEmpty(resp.OwnerID),
			resp.CreatedAt.UTC().Format(time.RFC3339), resp.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	cw.Flush()
}

// ExportDeals streams every deal in the workspace as CSV, with each custom
// property as a cf_-prefixed column.
func (h *Handler) ExportDeals(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}

	var (
		rows  []sqlcgen.Deal
		props map[uuid.UUID]map[string]any
	)
	err := readTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		var cursorTS pgtype.Timestamptz
		var cursorID uuid.UUID
		for {
			page, e := q.ListDeals(r.Context(), sqlcgen.ListDealsParams{
				CursorCreatedAt: cursorTS, CursorID: cursorID, PageLimit: exportPageSize,
			})
			if e != nil {
				return e
			}
			rows = append(rows, page...)
			if int32(len(page)) < exportPageSize {
				break
			}
			last := page[len(page)-1]
			cursorTS = last.CreatedAt
			cursorID = last.ID
		}
		var e error
		props, e = loadCustomProperties(r.Context(), tx, entityTypeDeal)
		return e
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "export deals", "err", err)
		writeErr(w, http.StatusInternalServerError, "export deals failed")
		return
	}

	cols := customPropertyColumns(props)
	writeCSVAttachment(w, "deals", time.Now())
	cw := csv.NewWriter(w)
	header := append([]string{"id", "title", "amount", "currency", "stage_id", "contact_id", "company_id", "owner_id", "expected_close_date", "closed_at", "created_at", "updated_at"}, prefixCols(cols)...)
	_ = cw.Write(header)
	for _, d := range rows {
		resp := dealFromRow(d)
		rec := []string{
			resp.ID.String(), resp.Title,
			floatOrEmpty(resp.Amount), strOrEmpty(resp.Currency),
			strOrEmpty(resp.StageID), strOrEmpty(resp.ContactID), strOrEmpty(resp.CompanyID), strOrEmpty(resp.OwnerID),
			strOrEmpty(resp.ExpectedCloseDate), timeOrEmpty(resp.ClosedAt),
			resp.CreatedAt.UTC().Format(time.RFC3339), resp.UpdatedAt.UTC().Format(time.RFC3339),
		}
		rec = append(rec, customCells(props[resp.ID], cols)...)
		_ = cw.Write(rec)
	}
	cw.Flush()
}

// prefixCols namespaces custom-property column headers with cf_ so they
// can't collide with fixed columns.
func prefixCols(cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = customPropPrefix + c
	}
	return out
}
