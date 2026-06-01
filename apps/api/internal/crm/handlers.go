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

	"github.com/gbconsult/lecrm/apps/api/capability"
	"github.com/gbconsult/lecrm/apps/api/internal/domain"
	"github.com/gbconsult/lecrm/apps/api/internal/jobs"
	"github.com/gbconsult/lecrm/apps/api/internal/rbac"
	"github.com/gbconsult/lecrm/apps/api/internal/sqlcgen"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

const (
	defaultPageLimit int32 = 50
	maxBodySize      int64 = 1 << 20
)

type Handler struct {
	Pool      *pgxpool.Pool
	Logger    *slog.Logger
	JobRunner jobs.JobRunner // optional — when nil, task reminders are skipped (v0 placeholder)

	// svc is the capability layer; it is lazily initialized via getSvc.
	svc *capability.Service
}

// getSvc returns the capability.Service for this handler, creating it lazily.
// All CRM handlers that delegate to the capability layer call this instead of
// accessing h.svc directly.
func (h *Handler) getSvc() *capability.Service {
	if h.svc == nil {
		h.svc = capability.New(h.Pool, h.Logger)
	}
	return h.svc
}

// principalFrom builds a capability.Principal from the request context. It
// reads the workspace (for WorkspaceID and Schema) and the RBAC principal
// (for Role and ActorType). Called once per handler invocation.
func principalFrom(r *http.Request) (capability.Principal, bool, *http.Request) {
	ws, err := workspace.WorkspaceFromContext(r.Context())
	if err != nil {
		return capability.Principal{}, false, r
	}

	var role capability.Role
	var actorType string
	var isServiceToken bool
	var scopes []string

	if p, ok := rbac.PrincipalFromContext(r.Context()); ok {
		// Map rbac.Role → capability.Role (same integer ordering).
		role = capability.Role(p.Role)
		actorType = p.ActorType
		isServiceToken = p.IsServiceToken
	}
	// actorType defaults to human_api when not set by service token.
	if actorType == "" {
		actorType = capability.ActorTypeHumanAPI
	}

	return capability.Principal{
		WorkspaceID:    ws.ID,
		Schema:         ws.RoleName,
		Role:           role,
		ActorType:      actorType,
		Scopes:         scopes,
		IsServiceToken: isServiceToken,
	}, true, r
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/contacts", h.ListContacts)
	r.Post("/v1/contacts", h.CreateContact)
	// Static /export and /import segments are registered as their own routes;
	// chi prioritises them over the /{id} wildcard so these words are never
	// parsed as UUIDs.
	r.Get("/v1/contacts/export", h.ExportContacts)
	r.Post("/v1/import/contacts/analyze", h.ImportAnalyze)
	r.Post("/v1/import/contacts/preview", h.ImportPreview)
	r.Post("/v1/import/contacts/commit", h.ImportCommit)
	r.Get("/v1/contacts/{id}", h.GetContact)
	r.Put("/v1/contacts/{id}", h.UpdateContact)
	r.Delete("/v1/contacts/{id}", h.DeleteContact)

	r.Get("/v1/companies", h.ListCompanies)
	r.Post("/v1/companies", h.CreateCompany)
	r.Get("/v1/companies/export", h.ExportCompanies)
	r.Post("/v1/import/companies/analyze", h.ImportAnalyze)
	r.Post("/v1/import/companies/preview", h.ImportPreview)
	r.Post("/v1/import/companies/commit", h.ImportCommit)
	r.Get("/v1/companies/{id}", h.GetCompany)
	r.Put("/v1/companies/{id}", h.UpdateCompany)
	r.Delete("/v1/companies/{id}", h.DeleteCompany)

	r.Get("/v1/deals", h.ListDeals)
	r.Post("/v1/deals", h.CreateDeal)
	r.Get("/v1/deals/export", h.ExportDeals)
	r.Post("/v1/import/deals/analyze", h.ImportAnalyze)
	r.Post("/v1/import/deals/preview", h.ImportPreview)
	r.Post("/v1/import/deals/commit", h.ImportCommit)
	r.Get("/v1/deals/{id}", h.GetDeal)
	r.Put("/v1/deals/{id}", h.UpdateDeal)
	r.Delete("/v1/deals/{id}", h.DeleteDeal)
	r.Patch("/v1/deals/{id}/stage", h.TransitionDealStage)

	r.Get("/v1/pipeline/stages", h.ListPipelineStages)

	// Dedup / merge (tasket 20260601-110828-76e8).
	r.Get("/v1/dedup/contacts", h.ListContactDuplicates)
	r.Get("/v1/dedup/companies", h.ListCompanyDuplicates)
	r.Post("/v1/dedup/contacts/merge", h.MergeContacts)
	r.Post("/v1/dedup/companies/merge", h.MergeCompanies)
	r.Post("/v1/dedup/contacts/distinct", h.MarkContactsDistinct)
	r.Post("/v1/dedup/companies/distinct", h.MarkCompaniesDistinct)
}

// --- transaction helpers (kept for export.go and other local users) ---

func readTx(ctx context.Context, pool *pgxpool.Pool, schema string, fn func(pgx.Tx) error) error {
	return capability.ReadTx(ctx, pool, schema, fn)
}

func writeTx(ctx context.Context, pool *pgxpool.Pool, schema string, fn func(pgx.Tx) error) error {
	return capability.WriteTx(ctx, pool, schema, fn)
}

// --- response types (kept for export.go and handlers_test.go) ---

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

// --- pgtype conversion helpers (kept for export.go and handlers_test.go) ---

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

// --- cursor helpers (kept for handlers_test.go) ---

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

// capErr maps capability layer errors to HTTP responses. Returns true if the
// error was handled (caller should return immediately), false if it was not a
// recognized sentinel.
func capErr(w http.ResponseWriter, err error, entityLabel string) bool {
	switch {
	case errors.Is(err, capability.ErrUnauthenticated):
		writeErr(w, http.StatusUnauthorized, "authentication required")
		return true
	case errors.Is(err, capability.ErrForbidden):
		writeErr(w, http.StatusForbidden, "insufficient role")
		return true
	case errors.Is(err, capability.ErrNotFound):
		writeErr(w, http.StatusNotFound, entityLabel+" not found")
		return true
	case errors.Is(err, capability.ErrBadStage):
		writeErr(w, http.StatusBadRequest, "stage not found")
		return true
	}
	var ve *capability.ValidationError
	if errors.As(err, &ve) {
		writeErr(w, http.StatusBadRequest, ve.Msg)
		return true
	}
	return false
}

// contactFromRow is kept for export.go which iterates sqlcgen rows directly.
func contactFromRow(c sqlcgen.Contact) contactResp {
	return contactResp{
		ID: c.ID, FirstName: c.FirstName, LastName: c.LastName,
		Email: textPtr(c.Email), Phone: textPtr(c.Phone),
		CompanyID: uuidPtr(c.CompanyID), OwnerID: uuidPtr(c.OwnerID),
		CreatedAt: c.CreatedAt.Time, UpdatedAt: c.UpdatedAt.Time,
	}
}

func companyFromRow(c sqlcgen.Company) companyResp {
	return companyResp{
		ID: c.ID, Name: c.Name,
		Domain: textPtr(c.Domain), Industry: textPtr(c.Industry), Size: textPtr(c.Size),
		OwnerID:   uuidPtr(c.OwnerID),
		CreatedAt: c.CreatedAt.Time, UpdatedAt: c.UpdatedAt.Time,
	}
}

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

var errNotFound = errors.New("not found")

// deleteRow is kept for anc_handlers.go tasks/notes which still do direct DB
// work (out of scope for this increment).
func deleteRow(ctx context.Context, tx pgx.Tx, table string, id uuid.UUID) error {
	tag, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNotFound
	}
	return nil
}

// ========= CONTACTS =========

func (h *Handler) ListContacts(w http.ResponseWriter, r *http.Request) {
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	result, err := h.getSvc().ListContacts(r.Context(), p, r.URL.Query().Get("cursor"))
	if err != nil {
		if capErr(w, err, "contacts") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "list contacts", "err", err)
		writeErr(w, http.StatusInternalServerError, "list contacts failed")
		return
	}
	writeJSON(w, http.StatusOK, listResp{
		Data:       result.Data,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	})
}

func (h *Handler) GetContact(w http.ResponseWriter, r *http.Request) {
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	result, err := h.getSvc().GetContact(r.Context(), p, id)
	if err != nil {
		if capErr(w, err, "contact") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "get contact", "err", err)
		writeErr(w, http.StatusInternalServerError, "get contact failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type createContactReq struct {
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	Email     *string `json:"email"`
	Phone     *string `json:"phone"`
	CompanyID *string `json:"company_id"`
}

func (h *Handler) CreateContact(w http.ResponseWriter, r *http.Request) {
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	var body createContactReq
	if !decodeBody(w, r, &body) {
		return
	}
	// Validate early (pre-capability) so we get a fast 400 before checking idempotency.
	email := ""
	if body.Email != nil {
		email = *body.Email
	}
	if err := (domain.CreateContactInput{FirstName: body.FirstName, LastName: body.LastName, Email: email}).Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	idemKey, ok := readIdempotencyKey(w, r)
	if !ok {
		return
	}
	result, err := h.getSvc().CreateContact(r.Context(), p, capability.CreateContactInput{
		FirstName: body.FirstName,
		LastName:  body.LastName,
		Email:     body.Email,
		Phone:     body.Phone,
		CompanyID: body.CompanyID,
	}, idemKey)
	if err != nil {
		if capErr(w, err, "contact") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "create contact", "err", err)
		writeErr(w, http.StatusInternalServerError, "create contact failed")
		return
	}
	if result.Replayed {
		writeReplay(w, result.Status, result.Body)
		return
	}
	writeRaw(w, result.Status, result.Body)
}

type updateContactReq struct {
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	Email     *string `json:"email"`
	Phone     *string `json:"phone"`
	CompanyID *string `json:"company_id"`
}

func (h *Handler) UpdateContact(w http.ResponseWriter, r *http.Request) {
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
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
	result, err := h.getSvc().UpdateContact(r.Context(), p, id, capability.UpdateContactInput{
		FirstName: body.FirstName,
		LastName:  body.LastName,
		Email:     body.Email,
		Phone:     body.Phone,
		CompanyID: body.CompanyID,
	})
	if err != nil {
		if capErr(w, err, "contact") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "update contact", "err", err)
		writeErr(w, http.StatusInternalServerError, "update contact failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// DeleteContact does a HARD delete. Soft-delete (deleted_at filter) was
// considered for tasket 20260525-1003 but rejected for v0: the only
// caller today is the React admin UI and ADR-009 does not require a
// recover path. The audit_log row is the durable trail. If a recover
// requirement appears, add `deleted_at` columns + a filtered read view.
func (h *Handler) DeleteContact(w http.ResponseWriter, r *http.Request) {
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.getSvc().DeleteContact(r.Context(), p, id); err != nil {
		if capErr(w, err, "contact") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "delete contact", "err", err)
		writeErr(w, http.StatusInternalServerError, "delete contact failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ========= COMPANIES =========

func (h *Handler) ListCompanies(w http.ResponseWriter, r *http.Request) {
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	result, err := h.getSvc().ListCompanies(r.Context(), p, r.URL.Query().Get("cursor"))
	if err != nil {
		if capErr(w, err, "companies") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "list companies", "err", err)
		writeErr(w, http.StatusInternalServerError, "list companies failed")
		return
	}
	writeJSON(w, http.StatusOK, listResp{
		Data:       result.Data,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	})
}

func (h *Handler) GetCompany(w http.ResponseWriter, r *http.Request) {
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	result, err := h.getSvc().GetCompany(r.Context(), p, id)
	if err != nil {
		if capErr(w, err, "company") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "get company", "err", err)
		writeErr(w, http.StatusInternalServerError, "get company failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type createCompanyReq struct {
	Name     string  `json:"name"`
	Domain   *string `json:"domain"`
	Industry *string `json:"industry"`
	Size     *string `json:"size"`
}

func (h *Handler) CreateCompany(w http.ResponseWriter, r *http.Request) {
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
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
	idemKey, ok := readIdempotencyKey(w, r)
	if !ok {
		return
	}
	result, err := h.getSvc().CreateCompany(r.Context(), p, capability.CreateCompanyInput{
		Name:     body.Name,
		Domain:   body.Domain,
		Industry: body.Industry,
		Size:     body.Size,
	}, idemKey)
	if err != nil {
		if capErr(w, err, "company") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "create company", "err", err)
		writeErr(w, http.StatusInternalServerError, "create company failed")
		return
	}
	if result.Replayed {
		writeReplay(w, result.Status, result.Body)
		return
	}
	writeRaw(w, result.Status, result.Body)
}

type updateCompanyReq struct {
	Name     string  `json:"name"`
	Domain   *string `json:"domain"`
	Industry *string `json:"industry"`
	Size     *string `json:"size"`
}

func (h *Handler) UpdateCompany(w http.ResponseWriter, r *http.Request) {
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
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
	result, err := h.getSvc().UpdateCompany(r.Context(), p, id, capability.UpdateCompanyInput{
		Name:     body.Name,
		Domain:   body.Domain,
		Industry: body.Industry,
		Size:     body.Size,
	})
	if err != nil {
		if capErr(w, err, "company") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "update company", "err", err)
		writeErr(w, http.StatusInternalServerError, "update company failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) DeleteCompany(w http.ResponseWriter, r *http.Request) {
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.getSvc().DeleteCompany(r.Context(), p, id); err != nil {
		if capErr(w, err, "company") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "delete company", "err", err)
		writeErr(w, http.StatusInternalServerError, "delete company failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ========= DEALS =========

func (h *Handler) ListDeals(w http.ResponseWriter, r *http.Request) {
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	result, err := h.getSvc().ListDeals(r.Context(), p, r.URL.Query().Get("cursor"))
	if err != nil {
		if capErr(w, err, "deals") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "list deals", "err", err)
		writeErr(w, http.StatusInternalServerError, "list deals failed")
		return
	}
	writeJSON(w, http.StatusOK, listResp{
		Data:       result.Data,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	})
}

func (h *Handler) GetDeal(w http.ResponseWriter, r *http.Request) {
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	result, err := h.getSvc().GetDeal(r.Context(), p, id)
	if err != nil {
		if capErr(w, err, "deal") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "get deal", "err", err)
		writeErr(w, http.StatusInternalServerError, "get deal failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
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
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
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
	idemKey, ok := readIdempotencyKey(w, r)
	if !ok {
		return
	}
	result, err := h.getSvc().CreateDeal(r.Context(), p, capability.CreateDealInput{
		Title:             body.Title,
		Amount:            body.Amount,
		Currency:          body.Currency,
		StageID:           body.StageID,
		ContactID:         body.ContactID,
		CompanyID:         body.CompanyID,
		ExpectedCloseDate: body.ExpectedCloseDate,
	}, idemKey)
	if err != nil {
		if capErr(w, err, "deal") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "create deal", "err", err)
		writeErr(w, http.StatusInternalServerError, "create deal failed")
		return
	}
	if result.Replayed {
		writeReplay(w, result.Status, result.Body)
		return
	}
	writeRaw(w, result.Status, result.Body)
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
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
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
	result, err := h.getSvc().UpdateDeal(r.Context(), p, id, capability.UpdateDealInput{
		Title:             body.Title,
		Amount:            body.Amount,
		Currency:          body.Currency,
		StageID:           body.StageID,
		ContactID:         body.ContactID,
		CompanyID:         body.CompanyID,
		ExpectedCloseDate: body.ExpectedCloseDate,
	})
	if err != nil {
		if capErr(w, err, "deal") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "update deal", "err", err)
		writeErr(w, http.StatusInternalServerError, "update deal failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) DeleteDeal(w http.ResponseWriter, r *http.Request) {
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.getSvc().DeleteDeal(r.Context(), p, id); err != nil {
		if capErr(w, err, "deal") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "delete deal", "err", err)
		writeErr(w, http.StatusInternalServerError, "delete deal failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ========= PIPELINE STAGES =========

type pipelineStageResp struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	OrderIndex int32     `json:"order_index"`
	CreatedAt  time.Time `json:"created_at"`
}

func (h *Handler) ListPipelineStages(w http.ResponseWriter, r *http.Request) {
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	stages, err := h.getSvc().ListPipelineStages(r.Context(), p)
	if err != nil {
		if capErr(w, err, "pipeline stages") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "list pipeline stages", "err", err)
		writeErr(w, http.StatusInternalServerError, "list pipeline stages failed")
		return
	}
	// Convert to local pipelineStageResp to match existing wire format.
	out := make([]pipelineStageResp, len(stages))
	for i, s := range stages {
		out[i] = pipelineStageResp{
			ID:         s.ID,
			Name:       s.Name,
			OrderIndex: s.OrderIndex,
			CreatedAt:  s.CreatedAt,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

type transitionDealStageReq struct {
	StageID string `json:"stage_id"`
}

// TransitionDealStage moves a deal to a new pipeline stage and atomically
// writes a `stage_change` activity row into objects. Same-stage PATCHes
// short-circuit (idempotent — no activity row).
func (h *Handler) TransitionDealStage(w http.ResponseWriter, r *http.Request) {
	p, ok, r := principalFrom(r)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	dealID, ok := parseID(w, r)
	if !ok {
		return
	}
	var body transitionDealStageReq
	if !decodeBody(w, r, &body) {
		return
	}
	newStageID, err := uuid.Parse(body.StageID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid stage_id")
		return
	}
	result, err := h.getSvc().TransitionDealStage(r.Context(), p, dealID, newStageID)
	if err != nil {
		if capErr(w, err, "deal") {
			return
		}
		h.Logger.ErrorContext(r.Context(), "transition deal stage", "err", err)
		writeErr(w, http.StatusInternalServerError, "transition deal stage failed")
		return
	}
	_ = result.Unchanged // both branches return the deal; tracked for tests via DB assertions
	writeJSON(w, http.StatusOK, result.Deal)
}
