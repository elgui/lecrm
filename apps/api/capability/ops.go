// Package capability – CRM operation methods (ADR-012 §1).
//
// This file adds the domain operations to the Service defined in
// capability.go. Every operation:
//   - Takes a resolved Principal as its first argument.
//   - Enforces RBAC (reads require RoleMember+, writes RoleAdmin+) via authorize().
//   - Runs its DB work inside ReadTx or WriteTx.
//   - Emits a fail-closed audit row (and optionally an activity row) inside the
//     same write transaction (ADR-009 §7.2).
//   - Returns domain result types — never wire formats.
//
// The REST handlers in apps/api/internal/crm/handlers.go become thin
// adapters: decode request → build Principal → call capability op →
// encode domain result to JSON.
package capability

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gbconsult/lecrm/apps/api/internal/domain"
	"github.com/gbconsult/lecrm/apps/api/internal/sqlcgen"
)

// --- domain result types ---

// ContactResult is the canonical domain shape for a contact returned by
// capability ops. REST handlers JSON-encode this directly.
type ContactResult struct {
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

// CompanyResult is the canonical domain shape for a company.
type CompanyResult struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Domain    *string   `json:"domain"`
	Industry  *string   `json:"industry"`
	Size      *string   `json:"size"`
	OwnerID   *string   `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DealResult is the canonical domain shape for a deal.
type DealResult struct {
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

// PipelineStageResult is the canonical domain shape for a pipeline stage.
type PipelineStageResult struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	OrderIndex int32     `json:"order_index"`
	CreatedAt  time.Time `json:"created_at"`
}

// ListResult is the keyset-pagination envelope used by all list operations.
type ListResult[T any] struct {
	Data       []T     `json:"data"`
	NextCursor *string `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
}

// --- cursor helpers (keyset pagination) ---

type cursorVal struct {
	T  time.Time `json:"t"`
	ID uuid.UUID `json:"id"`
}

func encodeCursorVal(t time.Time, id uuid.UUID) string {
	b, _ := json.Marshal(cursorVal{T: t, ID: id})
	return base64.URLEncoding.EncodeToString(b)
}

// DecodeCursor parses a URL-safe base64 keyset cursor into its components.
// Returns zero values (and valid=false in the Timestamptz) on a blank/bad
// input — callers treat that as "start from the beginning".
func DecodeCursor(s string) (pgtype.Timestamptz, uuid.UUID, error) {
	if s == "" {
		return pgtype.Timestamptz{}, uuid.Nil, nil
	}
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return pgtype.Timestamptz{}, uuid.Nil, err
	}
	var c cursorVal
	if err := json.Unmarshal(b, &c); err != nil {
		return pgtype.Timestamptz{}, uuid.Nil, err
	}
	return pgtype.Timestamptz{Time: c.T, Valid: true}, c.ID, nil
}

// --- row → domain converters ---

func contactFromRow(c sqlcgen.Contact) ContactResult {
	return ContactResult{
		ID:        c.ID,
		FirstName: c.FirstName,
		LastName:  c.LastName,
		Email:     textPtr(c.Email),
		Phone:     textPtr(c.Phone),
		CompanyID: uuidPtr(c.CompanyID),
		OwnerID:   uuidPtr(c.OwnerID),
		CreatedAt: c.CreatedAt.Time,
		UpdatedAt: c.UpdatedAt.Time,
	}
}

func companyFromRow(c sqlcgen.Company) CompanyResult {
	return CompanyResult{
		ID:        c.ID,
		Name:      c.Name,
		Domain:    textPtr(c.Domain),
		Industry:  textPtr(c.Industry),
		Size:      textPtr(c.Size),
		OwnerID:   uuidPtr(c.OwnerID),
		CreatedAt: c.CreatedAt.Time,
		UpdatedAt: c.UpdatedAt.Time,
	}
}

func dealFromRow(d sqlcgen.Deal) DealResult {
	return DealResult{
		ID:                d.ID,
		Title:             d.Title,
		Amount:            numPtr(d.Amount),
		Currency:          textPtr(d.Currency),
		StageID:           uuidPtr(d.StageID),
		ContactID:         uuidPtr(d.ContactID),
		CompanyID:         uuidPtr(d.CompanyID),
		OwnerID:           uuidPtr(d.OwnerID),
		ExpectedCloseDate: datePtr(d.ExpectedCloseDate),
		ClosedAt:          tsPtr(d.ClosedAt),
		CreatedAt:         d.CreatedAt.Time,
		UpdatedAt:         d.UpdatedAt.Time,
	}
}

func pipelineStageFromRow(s sqlcgen.PipelineStage) PipelineStageResult {
	return PipelineStageResult{
		ID:         s.ID,
		Name:       s.Name,
		OrderIndex: s.OrderIndex,
		CreatedAt:  s.CreatedAt.Time,
	}
}

// deleteRow executes a DELETE by id and returns ErrNotFound when zero rows
// were affected. Shared by DeleteContact, DeleteCompany, DeleteDeal.
func deleteRow(ctx context.Context, tx pgx.Tx, table string, id uuid.UUID) error {
	tag, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// =========================================================================
// CONTACTS
// =========================================================================

// CreateContactInput is the validated input for CreateContact.
type CreateContactInput struct {
	FirstName string
	LastName  string
	Email     *string
	Phone     *string
	CompanyID *string
}

// UpdateContactInput is the validated input for UpdateContact.
type UpdateContactInput struct {
	FirstName string
	LastName  string
	Email     *string
	Phone     *string
	CompanyID *string
}

// ListContacts returns a keyset-paginated list of contacts for the principal's
// workspace. Reads require RoleMember+.
func (s *Service) ListContacts(ctx context.Context, p Principal, cursorStr string) (ListResult[ContactResult], error) {
	if err := authorize(p, RoleMember); err != nil {
		return ListResult[ContactResult]{}, err
	}
	cursorTS, cursorID, _ := DecodeCursor(cursorStr)
	limit := DefaultPageLimit

	var rows []sqlcgen.Contact
	if err := s.readTx(ctx, p, func(tx pgx.Tx) error {
		var e error
		rows, e = sqlcgen.New(tx).ListContacts(ctx, sqlcgen.ListContactsParams{
			CursorCreatedAt: cursorTS,
			CursorID:        cursorID,
			PageLimit:       limit + 1,
		})
		return e
	}); err != nil {
		return ListResult[ContactResult]{}, err
	}

	hasMore := int32(len(rows)) > limit
	if hasMore {
		rows = rows[:limit]
	}
	out := make([]ContactResult, len(rows))
	for i, c := range rows {
		out[i] = contactFromRow(c)
	}
	var next *string
	if hasMore {
		last := rows[len(rows)-1]
		s := encodeCursorVal(last.CreatedAt.Time, last.ID)
		next = &s
	}
	return ListResult[ContactResult]{Data: out, NextCursor: next, HasMore: hasMore}, nil
}

// GetContact returns a single contact by id. Reads require RoleMember+.
func (s *Service) GetContact(ctx context.Context, p Principal, id uuid.UUID) (ContactResult, error) {
	if err := authorize(p, RoleMember); err != nil {
		return ContactResult{}, err
	}
	var row sqlcgen.Contact
	if err := s.readTx(ctx, p, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).GetContact(ctx, id)
		return e
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ContactResult{}, ErrNotFound
		}
		return ContactResult{}, err
	}
	return contactFromRow(row), nil
}

// CreateContact creates a new contact and returns a MutationResult whose Body
// is the canonical JSON of the created contact. Writes require RoleAdmin+.
// An idempotencyKey of "" means no idempotency check is performed.
func (s *Service) CreateContact(ctx context.Context, p Principal, in CreateContactInput, idempotencyKey string) (MutationResult, error) {
	if err := authorize(p, RoleAdmin); err != nil {
		return MutationResult{}, err
	}
	email := ""
	if in.Email != nil {
		email = *in.Email
	}
	if err := validationErr((domain.CreateContactInput{FirstName: in.FirstName, LastName: in.LastName, Email: email}).Validate()); err != nil {
		return MutationResult{}, err
	}

	// Idempotency check: short-circuit before opening a write tx.
	if idempotencyKey != "" {
		status, body, hit, err := IdempotencyLookup(ctx, s.Pool, p.WorkspaceID, idempotencyKey)
		if err != nil {
			return MutationResult{}, err
		}
		if hit {
			return MutationResult{Status: status, Body: body, Replayed: true}, nil
		}
	}

	const respStatus = 201
	var respBody []byte
	if err := s.writeTx(ctx, p, func(tx pgx.Tx) error {
		row, e := sqlcgen.New(tx).CreateContact(ctx, sqlcgen.CreateContactParams{
			FirstName: in.FirstName,
			LastName:  in.LastName,
			Email:     toText(in.Email),
			Phone:     toText(in.Phone),
			CompanyID: toNullUUID(in.CompanyID),
		})
		if e != nil {
			return e
		}
		respBody, e = json.Marshal(contactFromRow(row))
		if e != nil {
			return e
		}
		if e := EmitAudit(ctx, tx, "contact.created", p.WorkspaceID, p.ActorType, map[string]any{
			"id": row.ID.String(), "email": textPtr(row.Email),
		}); e != nil {
			return e
		}
		if e := s.emitEntityActivity(ctx, tx, p, EntityTypeContact, row.ID, "entity.created", map[string]any{
			"first_name": row.FirstName, "last_name": row.LastName, "email": textPtr(row.Email),
		}); e != nil {
			return e
		}
		if idempotencyKey != "" {
			return IdempotencyStore(ctx, tx, p.WorkspaceID, idempotencyKey, "POST", "/v1/contacts", respStatus, respBody)
		}
		return nil
	}); err != nil {
		return MutationResult{}, err
	}
	return MutationResult{Status: respStatus, Body: respBody}, nil
}

// UpdateContact replaces a contact's fields. Writes require RoleAdmin+.
func (s *Service) UpdateContact(ctx context.Context, p Principal, id uuid.UUID, in UpdateContactInput) (ContactResult, error) {
	if err := authorize(p, RoleAdmin); err != nil {
		return ContactResult{}, err
	}
	email := ""
	if in.Email != nil {
		email = *in.Email
	}
	if err := validationErr((domain.UpdateContactInput{FirstName: in.FirstName, LastName: in.LastName, Email: email}).Validate()); err != nil {
		return ContactResult{}, err
	}
	var row sqlcgen.Contact
	if err := s.writeTx(ctx, p, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).UpdateContact(ctx, sqlcgen.UpdateContactParams{
			ID:        id,
			FirstName: in.FirstName,
			LastName:  in.LastName,
			Email:     toText(in.Email),
			Phone:     toText(in.Phone),
			CompanyID: toNullUUID(in.CompanyID),
		})
		if e != nil {
			return e
		}
		if e := EmitAudit(ctx, tx, "contact.updated", p.WorkspaceID, p.ActorType, map[string]any{
			"id": row.ID.String(), "email": textPtr(row.Email),
		}); e != nil {
			return e
		}
		return s.emitEntityActivity(ctx, tx, p, EntityTypeContact, row.ID, "entity.updated", map[string]any{
			"first_name": row.FirstName, "last_name": row.LastName, "email": textPtr(row.Email),
		})
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ContactResult{}, ErrNotFound
		}
		return ContactResult{}, err
	}
	return contactFromRow(row), nil
}

// DeleteContact hard-deletes a contact. Writes require RoleAdmin+.
func (s *Service) DeleteContact(ctx context.Context, p Principal, id uuid.UUID) error {
	if err := authorize(p, RoleAdmin); err != nil {
		return err
	}
	return s.writeTx(ctx, p, func(tx pgx.Tx) error {
		if e := deleteRow(ctx, tx, "contacts", id); e != nil {
			return e
		}
		if e := EmitAudit(ctx, tx, "contact.deleted", p.WorkspaceID, p.ActorType, map[string]any{"id": id.String()}); e != nil {
			return e
		}
		return s.emitEntityActivity(ctx, tx, p, EntityTypeContact, id, "entity.deleted", map[string]any{"id": id.String()})
	})
}

// =========================================================================
// COMPANIES
// =========================================================================

// CreateCompanyInput is the validated input for CreateCompany.
type CreateCompanyInput struct {
	Name     string
	Domain   *string
	Industry *string
	Size     *string
}

// UpdateCompanyInput is the validated input for UpdateCompany.
type UpdateCompanyInput struct {
	Name     string
	Domain   *string
	Industry *string
	Size     *string
}

// ListCompanies returns a paginated list of companies. Reads require RoleMember+.
func (s *Service) ListCompanies(ctx context.Context, p Principal, cursorStr string) (ListResult[CompanyResult], error) {
	if err := authorize(p, RoleMember); err != nil {
		return ListResult[CompanyResult]{}, err
	}
	cursorTS, cursorID, _ := DecodeCursor(cursorStr)
	limit := DefaultPageLimit

	var rows []sqlcgen.Company
	if err := s.readTx(ctx, p, func(tx pgx.Tx) error {
		var e error
		rows, e = sqlcgen.New(tx).ListCompanies(ctx, sqlcgen.ListCompaniesParams{
			CursorCreatedAt: cursorTS,
			CursorID:        cursorID,
			PageLimit:       limit + 1,
		})
		return e
	}); err != nil {
		return ListResult[CompanyResult]{}, err
	}

	hasMore := int32(len(rows)) > limit
	if hasMore {
		rows = rows[:limit]
	}
	out := make([]CompanyResult, len(rows))
	for i, c := range rows {
		out[i] = companyFromRow(c)
	}
	var next *string
	if hasMore {
		last := rows[len(rows)-1]
		s := encodeCursorVal(last.CreatedAt.Time, last.ID)
		next = &s
	}
	return ListResult[CompanyResult]{Data: out, NextCursor: next, HasMore: hasMore}, nil
}

// GetCompany returns a single company by id. Reads require RoleMember+.
func (s *Service) GetCompany(ctx context.Context, p Principal, id uuid.UUID) (CompanyResult, error) {
	if err := authorize(p, RoleMember); err != nil {
		return CompanyResult{}, err
	}
	var row sqlcgen.Company
	if err := s.readTx(ctx, p, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).GetCompany(ctx, id)
		return e
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CompanyResult{}, ErrNotFound
		}
		return CompanyResult{}, err
	}
	return companyFromRow(row), nil
}

// CreateCompany creates a new company. Writes require RoleAdmin+.
func (s *Service) CreateCompany(ctx context.Context, p Principal, in CreateCompanyInput, idempotencyKey string) (MutationResult, error) {
	if err := authorize(p, RoleAdmin); err != nil {
		return MutationResult{}, err
	}
	size := ""
	if in.Size != nil {
		size = *in.Size
	}
	if err := validationErr((domain.CreateCompanyInput{Name: in.Name, Size: size}).Validate()); err != nil {
		return MutationResult{}, err
	}

	if idempotencyKey != "" {
		status, body, hit, err := IdempotencyLookup(ctx, s.Pool, p.WorkspaceID, idempotencyKey)
		if err != nil {
			return MutationResult{}, err
		}
		if hit {
			return MutationResult{Status: status, Body: body, Replayed: true}, nil
		}
	}

	const respStatus = 201
	var respBody []byte
	if err := s.writeTx(ctx, p, func(tx pgx.Tx) error {
		row, e := sqlcgen.New(tx).CreateCompany(ctx, sqlcgen.CreateCompanyParams{
			Name:     in.Name,
			Domain:   toText(in.Domain),
			Industry: toText(in.Industry),
			Size:     toText(in.Size),
		})
		if e != nil {
			return e
		}
		respBody, e = json.Marshal(companyFromRow(row))
		if e != nil {
			return e
		}
		if e := EmitAudit(ctx, tx, "company.created", p.WorkspaceID, p.ActorType, map[string]any{
			"id": row.ID.String(), "name": row.Name,
		}); e != nil {
			return e
		}
		if e := s.emitEntityActivity(ctx, tx, p, EntityTypeCompany, row.ID, "entity.created", map[string]any{
			"name": row.Name, "domain": textPtr(row.Domain),
		}); e != nil {
			return e
		}
		if idempotencyKey != "" {
			return IdempotencyStore(ctx, tx, p.WorkspaceID, idempotencyKey, "POST", "/v1/companies", respStatus, respBody)
		}
		return nil
	}); err != nil {
		return MutationResult{}, err
	}
	return MutationResult{Status: respStatus, Body: respBody}, nil
}

// UpdateCompany replaces a company's fields. Writes require RoleAdmin+.
func (s *Service) UpdateCompany(ctx context.Context, p Principal, id uuid.UUID, in UpdateCompanyInput) (CompanyResult, error) {
	if err := authorize(p, RoleAdmin); err != nil {
		return CompanyResult{}, err
	}
	var row sqlcgen.Company
	if err := s.writeTx(ctx, p, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).UpdateCompany(ctx, sqlcgen.UpdateCompanyParams{
			ID:       id,
			Name:     in.Name,
			Domain:   toText(in.Domain),
			Industry: toText(in.Industry),
			Size:     toText(in.Size),
		})
		if e != nil {
			return e
		}
		if e := EmitAudit(ctx, tx, "company.updated", p.WorkspaceID, p.ActorType, map[string]any{
			"id": row.ID.String(), "name": row.Name,
		}); e != nil {
			return e
		}
		return s.emitEntityActivity(ctx, tx, p, EntityTypeCompany, row.ID, "entity.updated", map[string]any{
			"name": row.Name, "domain": textPtr(row.Domain),
		})
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CompanyResult{}, ErrNotFound
		}
		return CompanyResult{}, err
	}
	return companyFromRow(row), nil
}

// DeleteCompany hard-deletes a company. Writes require RoleAdmin+.
func (s *Service) DeleteCompany(ctx context.Context, p Principal, id uuid.UUID) error {
	if err := authorize(p, RoleAdmin); err != nil {
		return err
	}
	return s.writeTx(ctx, p, func(tx pgx.Tx) error {
		if e := deleteRow(ctx, tx, "companies", id); e != nil {
			return e
		}
		if e := EmitAudit(ctx, tx, "company.deleted", p.WorkspaceID, p.ActorType, map[string]any{"id": id.String()}); e != nil {
			return e
		}
		return s.emitEntityActivity(ctx, tx, p, EntityTypeCompany, id, "entity.deleted", map[string]any{"id": id.String()})
	})
}

// =========================================================================
// DEALS
// =========================================================================

// CreateDealInput is the validated input for CreateDeal.
type CreateDealInput struct {
	Title             string
	Amount            *float64
	Currency          *string
	StageID           *string
	ContactID         *string
	CompanyID         *string
	ExpectedCloseDate *string
}

// UpdateDealInput is the validated input for UpdateDeal.
type UpdateDealInput struct {
	Title             string
	Amount            *float64
	Currency          *string
	StageID           *string
	ContactID         *string
	CompanyID         *string
	ExpectedCloseDate *string
}

// ListDeals returns a paginated list of deals. Reads require RoleMember+.
func (s *Service) ListDeals(ctx context.Context, p Principal, cursorStr string) (ListResult[DealResult], error) {
	if err := authorize(p, RoleMember); err != nil {
		return ListResult[DealResult]{}, err
	}
	cursorTS, cursorID, _ := DecodeCursor(cursorStr)
	limit := DefaultPageLimit

	var rows []sqlcgen.Deal
	if err := s.readTx(ctx, p, func(tx pgx.Tx) error {
		var e error
		rows, e = sqlcgen.New(tx).ListDeals(ctx, sqlcgen.ListDealsParams{
			CursorCreatedAt: cursorTS,
			CursorID:        cursorID,
			PageLimit:       limit + 1,
		})
		return e
	}); err != nil {
		return ListResult[DealResult]{}, err
	}

	hasMore := int32(len(rows)) > limit
	if hasMore {
		rows = rows[:limit]
	}
	out := make([]DealResult, len(rows))
	for i, d := range rows {
		out[i] = dealFromRow(d)
	}
	var next *string
	if hasMore {
		last := rows[len(rows)-1]
		s := encodeCursorVal(last.CreatedAt.Time, last.ID)
		next = &s
	}
	return ListResult[DealResult]{Data: out, NextCursor: next, HasMore: hasMore}, nil
}

// GetDeal returns a single deal by id. Reads require RoleMember+.
func (s *Service) GetDeal(ctx context.Context, p Principal, id uuid.UUID) (DealResult, error) {
	if err := authorize(p, RoleMember); err != nil {
		return DealResult{}, err
	}
	var row sqlcgen.Deal
	if err := s.readTx(ctx, p, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).GetDeal(ctx, id)
		return e
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DealResult{}, ErrNotFound
		}
		return DealResult{}, err
	}
	return dealFromRow(row), nil
}

// CreateDeal creates a new deal. Writes require RoleAdmin+.
func (s *Service) CreateDeal(ctx context.Context, p Principal, in CreateDealInput, idempotencyKey string) (MutationResult, error) {
	if err := authorize(p, RoleAdmin); err != nil {
		return MutationResult{}, err
	}
	cur := ""
	if in.Currency != nil {
		cur = *in.Currency
	}
	if err := validationErr((domain.CreateDealInput{Title: in.Title, Currency: cur}).Validate()); err != nil {
		return MutationResult{}, err
	}

	if idempotencyKey != "" {
		status, body, hit, err := IdempotencyLookup(ctx, s.Pool, p.WorkspaceID, idempotencyKey)
		if err != nil {
			return MutationResult{}, err
		}
		if hit {
			return MutationResult{Status: status, Body: body, Replayed: true}, nil
		}
	}

	const respStatus = 201
	var respBody []byte
	if err := s.writeTx(ctx, p, func(tx pgx.Tx) error {
		row, e := sqlcgen.New(tx).CreateDeal(ctx, sqlcgen.CreateDealParams{
			Title:             in.Title,
			Amount:            toNumeric(in.Amount),
			Currency:          toText(in.Currency),
			StageID:           toNullUUID(in.StageID),
			ContactID:         toNullUUID(in.ContactID),
			CompanyID:         toNullUUID(in.CompanyID),
			ExpectedCloseDate: toDate(in.ExpectedCloseDate),
		})
		if e != nil {
			return e
		}
		respBody, e = json.Marshal(dealFromRow(row))
		if e != nil {
			return e
		}
		if e := EmitAudit(ctx, tx, "deal.created", p.WorkspaceID, p.ActorType, map[string]any{
			"id": row.ID.String(), "title": row.Title, "stage_id": uuidPtr(row.StageID),
		}); e != nil {
			return e
		}
		if e := s.emitEntityActivity(ctx, tx, p, EntityTypeDeal, row.ID, "entity.created", map[string]any{
			"title": row.Title, "stage_id": uuidPtr(row.StageID), "amount": numPtr(row.Amount),
		}); e != nil {
			return e
		}
		if idempotencyKey != "" {
			return IdempotencyStore(ctx, tx, p.WorkspaceID, idempotencyKey, "POST", "/v1/deals", respStatus, respBody)
		}
		return nil
	}); err != nil {
		return MutationResult{}, err
	}
	return MutationResult{Status: respStatus, Body: respBody}, nil
}

// UpdateDeal replaces a deal's fields. Writes require RoleAdmin+.
func (s *Service) UpdateDeal(ctx context.Context, p Principal, id uuid.UUID, in UpdateDealInput) (DealResult, error) {
	if err := authorize(p, RoleAdmin); err != nil {
		return DealResult{}, err
	}
	var row sqlcgen.Deal
	if err := s.writeTx(ctx, p, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).UpdateDeal(ctx, sqlcgen.UpdateDealParams{
			ID:                id,
			Title:             in.Title,
			Amount:            toNumeric(in.Amount),
			Currency:          toText(in.Currency),
			StageID:           toNullUUID(in.StageID),
			ContactID:         toNullUUID(in.ContactID),
			CompanyID:         toNullUUID(in.CompanyID),
			ExpectedCloseDate: toDate(in.ExpectedCloseDate),
		})
		if e != nil {
			return e
		}
		if e := EmitAudit(ctx, tx, "deal.updated", p.WorkspaceID, p.ActorType, map[string]any{
			"id": row.ID.String(), "title": row.Title, "stage_id": uuidPtr(row.StageID),
		}); e != nil {
			return e
		}
		return s.emitEntityActivity(ctx, tx, p, EntityTypeDeal, row.ID, "entity.updated", map[string]any{
			"title": row.Title, "stage_id": uuidPtr(row.StageID), "amount": numPtr(row.Amount),
		})
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DealResult{}, ErrNotFound
		}
		return DealResult{}, err
	}
	return dealFromRow(row), nil
}

// DeleteDeal hard-deletes a deal. Writes require RoleAdmin+.
func (s *Service) DeleteDeal(ctx context.Context, p Principal, id uuid.UUID) error {
	if err := authorize(p, RoleAdmin); err != nil {
		return err
	}
	return s.writeTx(ctx, p, func(tx pgx.Tx) error {
		if e := deleteRow(ctx, tx, "deals", id); e != nil {
			return e
		}
		if e := EmitAudit(ctx, tx, "deal.deleted", p.WorkspaceID, p.ActorType, map[string]any{"id": id.String()}); e != nil {
			return e
		}
		return s.emitEntityActivity(ctx, tx, p, EntityTypeDeal, id, "entity.deleted", map[string]any{"id": id.String()})
	})
}

// =========================================================================
// PIPELINE STAGES
// =========================================================================

// ListPipelineStages returns all pipeline stages for the workspace. Reads
// require RoleMember+.
func (s *Service) ListPipelineStages(ctx context.Context, p Principal) ([]PipelineStageResult, error) {
	if err := authorize(p, RoleMember); err != nil {
		return nil, err
	}
	var rows []sqlcgen.PipelineStage
	if err := s.readTx(ctx, p, func(tx pgx.Tx) error {
		var e error
		rows, e = sqlcgen.New(tx).ListPipelineStages(ctx)
		return e
	}); err != nil {
		return nil, err
	}
	out := make([]PipelineStageResult, len(rows))
	for i, r := range rows {
		out[i] = pipelineStageFromRow(r)
	}
	return out, nil
}

// =========================================================================
// DEAL STAGE TRANSITION
// =========================================================================

// TransitionDealStageResult is the result of a stage transition.
type TransitionDealStageResult struct {
	Deal      DealResult
	Unchanged bool // true when the deal was already at targetStageID
}

// TransitionDealStage moves a deal to a new pipeline stage, atomically
// writing a stage_change activity row. Same-stage requests short-circuit
// (idempotent). Writes require RoleAdmin+.
func (s *Service) TransitionDealStage(ctx context.Context, p Principal, dealID, newStageID uuid.UUID) (TransitionDealStageResult, error) {
	if err := authorize(p, RoleAdmin); err != nil {
		return TransitionDealStageResult{}, err
	}

	var (
		updated      sqlcgen.Deal
		unchanged    bool
		notFoundDeal bool
		badStage     bool
	)

	err := s.writeTx(ctx, p, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)

		newStage, e := q.GetPipelineStage(ctx, newStageID)
		if errors.Is(e, pgx.ErrNoRows) {
			badStage = true
			return nil
		}
		if e != nil {
			return e
		}

		deal, e := q.GetDeal(ctx, dealID)
		if errors.Is(e, pgx.ErrNoRows) {
			notFoundDeal = true
			return nil
		}
		if e != nil {
			return e
		}

		// Same-stage: idempotent short-circuit.
		if deal.StageID.Valid && deal.StageID.UUID == newStageID {
			updated = deal
			unchanged = true
			return nil
		}

		oldStageID := deal.StageID
		var oldStageName *string
		if oldStageID.Valid {
			old, getErr := q.GetPipelineStage(ctx, oldStageID.UUID)
			if getErr == nil {
				n := old.Name
				oldStageName = &n
			} else if !errors.Is(getErr, pgx.ErrNoRows) {
				return getErr
			}
		}

		updated, e = q.UpdateDealStage(ctx, sqlcgen.UpdateDealStageParams{
			ID:      dealID,
			StageID: uuid.NullUUID{UUID: newStageID, Valid: true},
		})
		if e != nil {
			return e
		}

		// Write the objects-table activity (legacy path kept for UI timeline).
		activity := map[string]any{
			"kind":           "stage_change",
			"subject":        updated.Title,
			"occurred_at":    time.Now().UTC().Format(time.RFC3339),
			"new_stage":      newStageID.String(),
			"new_stage_name": newStage.Name,
		}
		if oldStageID.Valid {
			activity["old_stage"] = oldStageID.UUID.String()
		} else {
			activity["old_stage"] = nil
		}
		if oldStageName != nil {
			activity["old_stage_name"] = *oldStageName
		} else {
			activity["old_stage_name"] = nil
		}
		data, mErr := json.Marshal(activity)
		if mErr != nil {
			return mErr
		}
		if _, e = tx.Exec(ctx,
			`INSERT INTO objects (object_type, parent_type, parent_id, data) VALUES ('activity', 'deal', $1, $2)`,
			// string(data), not data: under pgx's simple query protocol a []byte
			// is sent as a bytea literal and rejected by the jsonb column (22P02).
			dealID, string(data),
		); e != nil {
			return e
		}

		stagePayload := map[string]any{
			"new_stage":      newStageID.String(),
			"new_stage_name": newStage.Name,
		}
		if oldStageID.Valid {
			stagePayload["old_stage"] = oldStageID.UUID.String()
		} else {
			stagePayload["old_stage"] = nil
		}
		if oldStageName != nil {
			stagePayload["old_stage_name"] = *oldStageName
		} else {
			stagePayload["old_stage_name"] = nil
		}
		return s.emitEntityActivity(ctx, tx, p, EntityTypeDeal, dealID, "deal.stage_changed", stagePayload)
	})
	if err != nil {
		return TransitionDealStageResult{}, err
	}
	if badStage {
		// Return a sentinel that callers map to 400.
		return TransitionDealStageResult{}, ErrBadStage
	}
	if notFoundDeal {
		return TransitionDealStageResult{}, ErrNotFound
	}
	return TransitionDealStageResult{Deal: dealFromRow(updated), Unchanged: unchanged}, nil
}

// ErrBadStage is returned when the target stage_id does not exist in the
// workspace's pipeline. REST adapters map this to 400.
var ErrBadStage = errors.New("stage not found")
