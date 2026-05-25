package crm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gbconsult/lecrm/apps/api/internal/domain"
	"github.com/gbconsult/lecrm/apps/api/internal/sqlcgen"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

const (
	defaultPageLimit int32 = 50
	maxBodySize      int64 = 1 << 20
)

type Handler struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/contacts", h.ListContacts)
	r.Post("/v1/contacts", h.CreateContact)
	r.Get("/v1/contacts/{id}", h.GetContact)
	r.Put("/v1/contacts/{id}", h.UpdateContact)
	r.Delete("/v1/contacts/{id}", h.DeleteContact)

	r.Get("/v1/companies", h.ListCompanies)
	r.Post("/v1/companies", h.CreateCompany)
	r.Get("/v1/companies/{id}", h.GetCompany)
	r.Put("/v1/companies/{id}", h.UpdateCompany)
	r.Delete("/v1/companies/{id}", h.DeleteCompany)

	r.Get("/v1/deals", h.ListDeals)
	r.Post("/v1/deals", h.CreateDeal)
	r.Get("/v1/deals/{id}", h.GetDeal)
	r.Put("/v1/deals/{id}", h.UpdateDeal)
	r.Delete("/v1/deals/{id}", h.DeleteDeal)
}

// --- transaction helpers ---

func readTx(ctx context.Context, pool *pgxpool.Pool, schema string, fn func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+pgx.Identifier{schema}.Sanitize()); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func writeTx(ctx context.Context, pool *pgxpool.Pool, schema string, fn func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+pgx.Identifier{schema}.Sanitize()); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// --- response types ---

type contactResp struct {
	ID        uuid.UUID `json:"id"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Email     *string   `json:"email"`
	Phone     *string   `json:"phone"`
	CompanyID *string   `json:"company_id"`
	OwnerID   *string   `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type companyResp struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Domain    *string   `json:"domain"`
	Industry  *string   `json:"industry"`
	Size      *string   `json:"size"`
	OwnerID   *string   `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type dealResp struct {
	ID                uuid.UUID  `json:"id"`
	Title             string     `json:"title"`
	Amount            *float64   `json:"amount"`
	Currency          *string    `json:"currency"`
	StageID           *string    `json:"stage_id"`
	ContactID         *string    `json:"contact_id"`
	CompanyID         *string    `json:"company_id"`
	OwnerID           *string    `json:"owner_id"`
	ExpectedCloseDate *string    `json:"expected_close_date"`
	ClosedAt          *time.Time `json:"closed_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type listResp struct {
	Data       any     `json:"data"`
	NextCursor *string `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
}

// --- pgtype conversion helpers ---

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func uuidPtr(u uuid.NullUUID) *string {
	if !u.Valid {
		return nil
	}
	s := u.UUID.String()
	return &s
}

func datePtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format("2006-01-02")
	return &s
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func numPtr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	return &f.Float64
}

func toText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func toNullUUID(s *string) uuid.NullUUID {
	if s == nil {
		return uuid.NullUUID{}
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: id, Valid: true}
}

func toNumeric(f *float64) pgtype.Numeric {
	if f == nil {
		return pgtype.Numeric{}
	}
	var n pgtype.Numeric
	_ = n.Scan(strconv.FormatFloat(*f, 'f', -1, 64))
	return n
}

func toDate(s *string) pgtype.Date {
	if s == nil {
		return pgtype.Date{}
	}
	t, err := time.Parse("2006-01-02", *s)
	if err != nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: t, Valid: true}
}

// --- cursor helpers ---

type cursor struct {
	T  time.Time `json:"t"`
	ID uuid.UUID `json:"id"`
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	b, _ := json.Marshal(cursor{T: t, ID: id})
	return base64.URLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (pgtype.Timestamptz, uuid.UUID, error) {
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return pgtype.Timestamptz{}, uuid.Nil, err
	}
	var c cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return pgtype.Timestamptz{}, uuid.Nil, err
	}
	return pgtype.Timestamptz{Time: c.T, Valid: true}, c.ID, nil
}

// --- JSON helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) ws(w http.ResponseWriter, r *http.Request) (*workspace.Context, bool) {
	ws, err := workspace.WorkspaceFromContext(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return nil, false
	}
	return ws, true
}

// ========= CONTACTS =========

func contactFromRow(c sqlcgen.Contact) contactResp {
	return contactResp{
		ID: c.ID, FirstName: c.FirstName, LastName: c.LastName,
		Email: textPtr(c.Email), Phone: textPtr(c.Phone),
		CompanyID: uuidPtr(c.CompanyID), OwnerID: uuidPtr(c.OwnerID),
		CreatedAt: c.CreatedAt.Time, UpdatedAt: c.UpdatedAt.Time,
	}
}

func (h *Handler) ListContacts(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	cursorTS, cursorID, _ := decodeCursor(r.URL.Query().Get("cursor"))
	limit := defaultPageLimit

	var rows []sqlcgen.Contact
	err := readTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		rows, e = sqlcgen.New(tx).ListContacts(r.Context(), sqlcgen.ListContactsParams{
			CursorCreatedAt: cursorTS, CursorID: cursorID, PageLimit: limit + 1,
		})
		return e
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "list contacts", "err", err)
		writeErr(w, http.StatusInternalServerError, "list contacts failed")
		return
	}

	hasMore := int32(len(rows)) > limit
	if hasMore {
		rows = rows[:limit]
	}
	out := make([]contactResp, len(rows))
	for i, c := range rows {
		out[i] = contactFromRow(c)
	}
	var next *string
	if hasMore {
		last := rows[len(rows)-1]
		s := encodeCursor(last.CreatedAt.Time, last.ID)
		next = &s
	}
	writeJSON(w, http.StatusOK, listResp{Data: out, NextCursor: next, HasMore: hasMore})
}

func (h *Handler) GetContact(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var row sqlcgen.Contact
	err := readTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).GetContact(r.Context(), id)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "contact not found")
		return
	}
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "get contact", "err", err)
		writeErr(w, http.StatusInternalServerError, "get contact failed")
		return
	}
	writeJSON(w, http.StatusOK, contactFromRow(row))
}

type createContactReq struct {
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	Email     *string `json:"email"`
	Phone     *string `json:"phone"`
	CompanyID *string `json:"company_id"`
}

func (h *Handler) CreateContact(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	var body createContactReq
	if !decodeBody(w, r, &body) {
		return
	}
	email := ""
	if body.Email != nil {
		email = *body.Email
	}
	if err := (domain.CreateContactInput{FirstName: body.FirstName, LastName: body.LastName, Email: email}).Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	var row sqlcgen.Contact
	err := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).CreateContact(r.Context(), sqlcgen.CreateContactParams{
			FirstName: body.FirstName, LastName: body.LastName,
			Email: toText(body.Email), Phone: toText(body.Phone),
			CompanyID: toNullUUID(body.CompanyID),
		})
		return e
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "create contact", "err", err)
		writeErr(w, http.StatusInternalServerError, "create contact failed")
		return
	}
	writeJSON(w, http.StatusCreated, contactFromRow(row))
}

type updateContactReq struct {
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	Email     *string `json:"email"`
	Phone     *string `json:"phone"`
	CompanyID *string `json:"company_id"`
}

func (h *Handler) UpdateContact(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body updateContactReq
	if !decodeBody(w, r, &body) {
		return
	}
	var row sqlcgen.Contact
	err := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).UpdateContact(r.Context(), sqlcgen.UpdateContactParams{
			ID: id, FirstName: body.FirstName, LastName: body.LastName,
			Email: toText(body.Email), Phone: toText(body.Phone),
			CompanyID: toNullUUID(body.CompanyID),
		})
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "contact not found")
		return
	}
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "update contact", "err", err)
		writeErr(w, http.StatusInternalServerError, "update contact failed")
		return
	}
	writeJSON(w, http.StatusOK, contactFromRow(row))
}

func (h *Handler) DeleteContact(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	err := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		return sqlcgen.New(tx).DeleteContact(r.Context(), id)
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "delete contact", "err", err)
		writeErr(w, http.StatusInternalServerError, "delete contact failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ========= COMPANIES =========

func companyFromRow(c sqlcgen.Company) companyResp {
	return companyResp{
		ID: c.ID, Name: c.Name,
		Domain: textPtr(c.Domain), Industry: textPtr(c.Industry), Size: textPtr(c.Size),
		OwnerID:   uuidPtr(c.OwnerID),
		CreatedAt: c.CreatedAt.Time, UpdatedAt: c.UpdatedAt.Time,
	}
}

func (h *Handler) ListCompanies(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	cursorTS, cursorID, _ := decodeCursor(r.URL.Query().Get("cursor"))
	limit := defaultPageLimit

	var rows []sqlcgen.Company
	err := readTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		rows, e = sqlcgen.New(tx).ListCompanies(r.Context(), sqlcgen.ListCompaniesParams{
			CursorCreatedAt: cursorTS, CursorID: cursorID, PageLimit: limit + 1,
		})
		return e
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "list companies", "err", err)
		writeErr(w, http.StatusInternalServerError, "list companies failed")
		return
	}

	hasMore := int32(len(rows)) > limit
	if hasMore {
		rows = rows[:limit]
	}
	out := make([]companyResp, len(rows))
	for i, c := range rows {
		out[i] = companyFromRow(c)
	}
	var next *string
	if hasMore {
		last := rows[len(rows)-1]
		s := encodeCursor(last.CreatedAt.Time, last.ID)
		next = &s
	}
	writeJSON(w, http.StatusOK, listResp{Data: out, NextCursor: next, HasMore: hasMore})
}

func (h *Handler) GetCompany(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var row sqlcgen.Company
	err := readTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).GetCompany(r.Context(), id)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "company not found")
		return
	}
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "get company", "err", err)
		writeErr(w, http.StatusInternalServerError, "get company failed")
		return
	}
	writeJSON(w, http.StatusOK, companyFromRow(row))
}

type createCompanyReq struct {
	Name     string  `json:"name"`
	Domain   *string `json:"domain"`
	Industry *string `json:"industry"`
	Size     *string `json:"size"`
}

func (h *Handler) CreateCompany(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	var body createCompanyReq
	if !decodeBody(w, r, &body) {
		return
	}
	size := ""
	if body.Size != nil {
		size = *body.Size
	}
	if err := (domain.CreateCompanyInput{Name: body.Name, Size: size}).Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	var row sqlcgen.Company
	err := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).CreateCompany(r.Context(), sqlcgen.CreateCompanyParams{
			Name: body.Name, Domain: toText(body.Domain),
			Industry: toText(body.Industry), Size: toText(body.Size),
		})
		return e
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "create company", "err", err)
		writeErr(w, http.StatusInternalServerError, "create company failed")
		return
	}
	writeJSON(w, http.StatusCreated, companyFromRow(row))
}

type updateCompanyReq struct {
	Name     string  `json:"name"`
	Domain   *string `json:"domain"`
	Industry *string `json:"industry"`
	Size     *string `json:"size"`
}

func (h *Handler) UpdateCompany(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body updateCompanyReq
	if !decodeBody(w, r, &body) {
		return
	}
	var row sqlcgen.Company
	err := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).UpdateCompany(r.Context(), sqlcgen.UpdateCompanyParams{
			ID: id, Name: body.Name,
			Domain: toText(body.Domain), Industry: toText(body.Industry), Size: toText(body.Size),
		})
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "company not found")
		return
	}
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "update company", "err", err)
		writeErr(w, http.StatusInternalServerError, "update company failed")
		return
	}
	writeJSON(w, http.StatusOK, companyFromRow(row))
}

func (h *Handler) DeleteCompany(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	err := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		return sqlcgen.New(tx).DeleteCompany(r.Context(), id)
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "delete company", "err", err)
		writeErr(w, http.StatusInternalServerError, "delete company failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ========= DEALS =========

func dealFromRow(d sqlcgen.Deal) dealResp {
	return dealResp{
		ID: d.ID, Title: d.Title,
		Amount: numPtr(d.Amount), Currency: textPtr(d.Currency),
		StageID: uuidPtr(d.StageID), ContactID: uuidPtr(d.ContactID),
		CompanyID: uuidPtr(d.CompanyID), OwnerID: uuidPtr(d.OwnerID),
		ExpectedCloseDate: datePtr(d.ExpectedCloseDate),
		ClosedAt:          tsPtr(d.ClosedAt),
		CreatedAt:         d.CreatedAt.Time, UpdatedAt: d.UpdatedAt.Time,
	}
}

func (h *Handler) ListDeals(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	cursorTS, cursorID, _ := decodeCursor(r.URL.Query().Get("cursor"))
	limit := defaultPageLimit

	var rows []sqlcgen.Deal
	err := readTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		rows, e = sqlcgen.New(tx).ListDeals(r.Context(), sqlcgen.ListDealsParams{
			CursorCreatedAt: cursorTS, CursorID: cursorID, PageLimit: limit + 1,
		})
		return e
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "list deals", "err", err)
		writeErr(w, http.StatusInternalServerError, "list deals failed")
		return
	}

	hasMore := int32(len(rows)) > limit
	if hasMore {
		rows = rows[:limit]
	}
	out := make([]dealResp, len(rows))
	for i, d := range rows {
		out[i] = dealFromRow(d)
	}
	var next *string
	if hasMore {
		last := rows[len(rows)-1]
		s := encodeCursor(last.CreatedAt.Time, last.ID)
		next = &s
	}
	writeJSON(w, http.StatusOK, listResp{Data: out, NextCursor: next, HasMore: hasMore})
}

func (h *Handler) GetDeal(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var row sqlcgen.Deal
	err := readTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).GetDeal(r.Context(), id)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "deal not found")
		return
	}
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "get deal", "err", err)
		writeErr(w, http.StatusInternalServerError, "get deal failed")
		return
	}
	writeJSON(w, http.StatusOK, dealFromRow(row))
}

type createDealReq struct {
	Title             string   `json:"title"`
	Amount            *float64 `json:"amount"`
	Currency          *string  `json:"currency"`
	StageID           *string  `json:"stage_id"`
	ContactID         *string  `json:"contact_id"`
	CompanyID         *string  `json:"company_id"`
	ExpectedCloseDate *string  `json:"expected_close_date"`
}

func (h *Handler) CreateDeal(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	var body createDealReq
	if !decodeBody(w, r, &body) {
		return
	}
	cur := ""
	if body.Currency != nil {
		cur = *body.Currency
	}
	if err := (domain.CreateDealInput{Title: body.Title, Currency: cur}).Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	var row sqlcgen.Deal
	err := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).CreateDeal(r.Context(), sqlcgen.CreateDealParams{
			Title: body.Title, Amount: toNumeric(body.Amount),
			Currency: toText(body.Currency), StageID: toNullUUID(body.StageID),
			ContactID: toNullUUID(body.ContactID), CompanyID: toNullUUID(body.CompanyID),
			ExpectedCloseDate: toDate(body.ExpectedCloseDate),
		})
		return e
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "create deal", "err", err)
		writeErr(w, http.StatusInternalServerError, "create deal failed")
		return
	}
	writeJSON(w, http.StatusCreated, dealFromRow(row))
}

type updateDealReq struct {
	Title             string   `json:"title"`
	Amount            *float64 `json:"amount"`
	Currency          *string  `json:"currency"`
	StageID           *string  `json:"stage_id"`
	ContactID         *string  `json:"contact_id"`
	CompanyID         *string  `json:"company_id"`
	ExpectedCloseDate *string  `json:"expected_close_date"`
}

func (h *Handler) UpdateDeal(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body updateDealReq
	if !decodeBody(w, r, &body) {
		return
	}
	var row sqlcgen.Deal
	err := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).UpdateDeal(r.Context(), sqlcgen.UpdateDealParams{
			ID: id, Title: body.Title, Amount: toNumeric(body.Amount),
			Currency: toText(body.Currency), StageID: toNullUUID(body.StageID),
			ContactID: toNullUUID(body.ContactID), CompanyID: toNullUUID(body.CompanyID),
			ExpectedCloseDate: toDate(body.ExpectedCloseDate),
		})
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "deal not found")
		return
	}
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "update deal", "err", err)
		writeErr(w, http.StatusInternalServerError, "update deal failed")
		return
	}
	writeJSON(w, http.StatusOK, dealFromRow(row))
}

func (h *Handler) DeleteDeal(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	err := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		return sqlcgen.New(tx).DeleteDeal(r.Context(), id)
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "delete deal", "err", err)
		writeErr(w, http.StatusInternalServerError, "delete deal failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
