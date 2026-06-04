// Package capability – MCP intent write tools (ADR-012 §3).
//
// These are the three headline read-write operations the conversational
// MCP surface exposes (ADR-012 §3/§4, §10 Increment 1.3):
//
//   - AdvanceDeal     — stage transition + activity, with stage-name fuzzy
//     match and optional close (story S1).
//   - LogInteraction  — upsert a contact if needed, append an activity
//     (stories S2/S3).
//   - CaptureLead     — upsert a contact (dedup by email), optionally link a
//     company, and open a deal in the first pipeline stage (story S2).
//
// They are *intent-shaped composites*, NOT a CRUD mirror of REST (ADR-012
// §2): each encapsulates one real user story as a single safe, idempotent,
// auditable call. They compose the §6 safety primitives (writesafety.go) via
// GuardedWrite and dispatch to the same store/audit/activity building blocks
// the REST and connector paths use — no business logic is duplicated.
//
// CaptureLead's contact upsert is the *same* capability call the connector
// candidate-ingestion path uses (UpsertContactByEmail, reused by
// apps/api/internal/crm/connectors.go) — same capability op, different door
// (ADR-012 §3).
//
// Every mutation runs through writeTx (fail-closed audit) and is attributed
// the Principal's ActorType (ActorTypeMCPAgent for an MCP agent today, never
// hardcoded — an OAuth end-user client or human session writes through the
// same path with its own actor_type, satisfying the §7 non-foreclosure
// checklist).
package capability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gbconsult/lecrm/apps/api/internal/sqlcgen"
)

// ErrAmbiguous is returned when a fuzzy reference (deal title, stage name,
// contact/company name) matches more than one row, so the caller must be
// more specific. Transport adapters map it to 400/isError.
var ErrAmbiguous = errors.New("reference is ambiguous; please be more specific")

// MCPWritePrincipal builds the Principal an MCP write tool acts under. Unlike
// the read principal it sets NO ReadRole: writeTx must run as the pool's
// (write-capable) login role, not a per-workspace RO role. Role is derived
// from the token's granted scopes (RoleFromScopes), so a read-only-scoped
// token resolves to RoleMember and is rejected by AuthorizeWrite before any
// mutation — the primary blast-radius control (ADR-012 §6/§8). ActorType is
// the MCP agent; Scopes are retained so AuthorizeWrite's scope gate fires.
func MCPWritePrincipal(ws uuid.UUID, scopes []string) Principal {
	return Principal{
		WorkspaceID:    ws,
		Schema:         MCPSchemaName(ws),
		Role:           RoleFromScopes(scopes),
		Scopes:         scopes,
		ActorType:      ActorTypeMCPAgent,
		IsServiceToken: true,
	}
}

// =========================================================================
// advance_deal  (ADR-012 §3, story S1)
// =========================================================================

// AdvanceDealInput is the intent input for AdvanceDeal. Deal references a
// deal by UUID or a fuzzy title match; ToStage references a pipeline stage
// by UUID or a fuzzy name match. MarkClosedAt, when non-nil, stamps the
// deal's closed_at: an empty/"today"/"now" value uses the current time, a
// YYYY-MM-DD value uses that date. Marking a deal closed is classified
// destructive and triggers the confirmation handshake (§6).
type AdvanceDealInput struct {
	Deal         string
	ToStage      string
	Note         *string
	MarkClosedAt *string
}

// advanceDealDestructive reports whether an advance_deal call is destructive
// (and therefore requires the dry-run → confirmation handshake). Closing a
// deal is a significant, not-trivially-reversible state change, so a
// mark_closed_at request is destructive; a plain stage move is not.
func advanceDealDestructive(in AdvanceDealInput) bool { return in.MarkClosedAt != nil }

// AdvanceDeal moves a deal to a new pipeline stage (fuzzy stage-name match),
// optionally closing it, and appends a stage-change activity. It composes the
// §6 controls via GuardedWrite: scope→role gate, dry-run preview, confirmation
// handshake (when closing), idempotent replay, and fail-closed audit.
func (s *Service) AdvanceDeal(ctx context.Context, p Principal, in AdvanceDealInput, opts WriteOptions, confirmer *Confirmer, now func() time.Time) (WriteResult, error) {
	gw := GuardedWrite{
		Operation:   "advance_deal",
		Destructive: advanceDealDestructive(in),
		Options:     opts,
		Confirmer:   confirmer,
		Now:         now,
	}

	plan := func() (Preview, error) {
		var pv Preview
		err := s.readTx(ctx, p, func(tx pgx.Tx) error {
			deal, e := resolveDealRef(ctx, tx, in.Deal)
			if e != nil {
				return e
			}
			_, stageName, e := resolveStageFuzzy(ctx, tx, in.ToStage)
			if e != nil {
				return e
			}
			before := map[string]any{"stage": stageNameOf(ctx, tx, deal.StageID)}
			after := map[string]any{"stage": stageName}
			if in.MarkClosedAt != nil {
				after["closed_at"] = closedAtTime(in.MarkClosedAt, gw.now()).Format(time.RFC3339)
			}
			pv = Preview{
				Summary: fmt.Sprintf("would advance deal %q to stage %q", deal.Title, stageName),
				Effects: []Effect{{
					Action:     "stage_change",
					EntityType: EntityTypeDeal,
					EntityID:   deal.ID.String(),
					Before:     before,
					After:      after,
				}},
			}
			return nil
		})
		return pv, err
	}

	replay := s.idempotentReplay(ctx, p, opts.IdempotencyKey)

	apply := func() (int, []byte, error) {
		const status = 200
		var body []byte
		err := s.writeTx(ctx, p, func(tx pgx.Tx) error {
			deal, e := resolveDealRef(ctx, tx, in.Deal)
			if e != nil {
				return e
			}
			stageID, stageName, e := resolveStageFuzzy(ctx, tx, in.ToStage)
			if e != nil {
				return e
			}
			updated, e := s.applyDealStageChange(ctx, tx, p, deal, stageID, stageName, in.Note, in.MarkClosedAt, gw.now())
			if e != nil {
				return e
			}
			body, e = json.Marshal(dealFromRow(updated))
			if e != nil {
				return e
			}
			if e := EmitAudit(ctx, tx, "deal.advanced", p.WorkspaceID, p.ActorType, map[string]any{
				"id": updated.ID.String(), "stage_id": stageID.String(), "stage_name": stageName,
				"closed": in.MarkClosedAt != nil,
			}); e != nil {
				return e
			}
			return s.storeIdempotent(ctx, tx, p, opts.IdempotencyKey, "advance_deal", status, body)
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil, ErrNotFound
		}
		return status, body, err
	}

	return gw.Run(p, replay, plan, apply)
}

// applyDealStageChange performs the stage move inside tx: it records the old
// stage name, updates stage_id (and closed_at when closing), writes the
// legacy objects-table activity row for the UI timeline (parity with
// TransitionDealStage), and emits the canonical deal.stage_changed activity
// attributed to the principal's actor type.
func (s *Service) applyDealStageChange(ctx context.Context, tx pgx.Tx, p Principal, deal sqlcgen.Deal, newStageID uuid.UUID, newStageName string, note, markClosedAt *string, now time.Time) (sqlcgen.Deal, error) {
	q := sqlcgen.New(tx)

	var oldStageName *string
	if deal.StageID.Valid {
		if old, err := q.GetPipelineStage(ctx, deal.StageID.UUID); err == nil {
			n := old.Name
			oldStageName = &n
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return sqlcgen.Deal{}, err
		}
	}

	updated, err := q.UpdateDealStage(ctx, sqlcgen.UpdateDealStageParams{
		ID:      deal.ID,
		StageID: uuid.NullUUID{UUID: newStageID, Valid: true},
	})
	if err != nil {
		return sqlcgen.Deal{}, err
	}

	if markClosedAt != nil {
		closed := closedAtTime(markClosedAt, now)
		if _, err := tx.Exec(ctx,
			`UPDATE deals SET closed_at = $2, updated_at = now() WHERE id = $1`,
			deal.ID, pgtype.Timestamptz{Time: closed, Valid: true}); err != nil {
			return sqlcgen.Deal{}, err
		}
		updated, err = q.GetDeal(ctx, deal.ID)
		if err != nil {
			return sqlcgen.Deal{}, err
		}
	}

	// Legacy objects-table activity (UI timeline parity with TransitionDealStage).
	activity := map[string]any{
		"kind":           "stage_change",
		"subject":        updated.Title,
		"occurred_at":    now.UTC().Format(time.RFC3339),
		"new_stage":      newStageID.String(),
		"new_stage_name": newStageName,
		"old_stage_name": derefStr(oldStageName),
	}
	if note != nil {
		activity["note"] = *note
	}
	data, err := json.Marshal(activity)
	if err != nil {
		return sqlcgen.Deal{}, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO objects (object_type, parent_type, parent_id, data) VALUES ('activity', 'deal', $1, $2)`,
		// string(data), not data: under pgx's simple query protocol a []byte
		// is sent as a bytea literal and rejected by the jsonb column (22P02).
		deal.ID, string(data)); err != nil {
		return sqlcgen.Deal{}, err
	}

	payload := map[string]any{
		"new_stage":      newStageID.String(),
		"new_stage_name": newStageName,
		"old_stage_name": derefStr(oldStageName),
		"closed":         markClosedAt != nil,
	}
	if note != nil {
		payload["note"] = *note
	}
	if err := s.emitEntityActivity(ctx, tx, p, EntityTypeDeal, deal.ID, "deal.stage_changed", payload); err != nil {
		return sqlcgen.Deal{}, err
	}
	return updated, nil
}

// =========================================================================
// log_interaction  (ADR-012 §3, stories S2/S3)
// =========================================================================

// LogInteractionInput is the intent input for LogInteraction. ContactOrCompany
// references the subject by UUID, email (contains '@'), or name; when it names
// a person that doesn't exist yet, a contact is upserted. Summary is the
// interaction text; Outcome is an optional disposition ("won", "no answer", …).
type LogInteractionInput struct {
	ContactOrCompany string
	Summary          string
	Outcome          *string
}

// LogInteraction resolves (upserting a contact when needed) the interaction's
// subject and appends an interaction activity. It is additive (non-destructive),
// idempotent, and audited.
func (s *Service) LogInteraction(ctx context.Context, p Principal, in LogInteractionInput, opts WriteOptions) (WriteResult, error) {
	if strings.TrimSpace(in.Summary) == "" {
		// AuthorizeWrite still runs first inside GuardedWrite; surface the
		// validation only after authorization so a read-only token sees the
		// scope denial, not a hint about argument shape.
		return s.guardedValidationWrite(p, opts, "log_interaction", &ValidationError{Msg: "summary is required"})
	}

	gw := GuardedWrite{Operation: "log_interaction", Options: opts}

	plan := func() (Preview, error) {
		return Preview{
			Summary: fmt.Sprintf("would log an interaction on %q", in.ContactOrCompany),
			Effects: []Effect{{
				Action:     "create",
				EntityType: "activity",
				After:      map[string]any{"summary": in.Summary, "outcome": derefStr(in.Outcome)},
			}},
		}, nil
	}

	replay := s.idempotentReplay(ctx, p, opts.IdempotencyKey)

	apply := func() (int, []byte, error) {
		const status = 201
		var body []byte
		err := s.writeTx(ctx, p, func(tx pgx.Tx) error {
			entityType, entityID, created, e := resolveInteractionSubject(ctx, tx, in.ContactOrCompany)
			if e != nil {
				return e
			}
			payload := map[string]any{"summary": in.Summary}
			if in.Outcome != nil {
				payload["outcome"] = *in.Outcome
			}
			if e := s.emitEntityActivity(ctx, tx, p, entityType, entityID, "interaction.logged", payload); e != nil {
				return e
			}
			if e := EmitAudit(ctx, tx, "interaction.logged", p.WorkspaceID, p.ActorType, map[string]any{
				"entity_type": entityType, "entity_id": entityID.String(), "outcome": derefStr(in.Outcome),
			}); e != nil {
				return e
			}
			body, e = json.Marshal(map[string]any{
				"entity_type":     entityType,
				"entity_id":       entityID.String(),
				"contact_created": created,
				"interaction":     "logged",
			})
			if e != nil {
				return e
			}
			return s.storeIdempotent(ctx, tx, p, opts.IdempotencyKey, "log_interaction", status, body)
		})
		return status, body, err
	}

	return gw.Run(p, replay, plan, apply)
}

// =========================================================================
// capture_lead  (ADR-012 §3, story S2)
// =========================================================================

// CaptureLeadInput is the intent input for CaptureLead. Name is the lead's
// full name (split on the first space into first/last); Email, when present,
// dedups against existing contacts; Company, when present, is found-or-created
// and linked; Source records where the lead came from (chatbot, voice, form…).
type CaptureLeadInput struct {
	Name    string
	Email   *string
	Company *string
	Source  string
}

// CaptureLeadResult is the canonical body CaptureLead returns.
type CaptureLeadResult struct {
	ContactID      string  `json:"contact_id"`
	DealID         string  `json:"deal_id"`
	CompanyID      *string `json:"company_id"`
	ContactCreated bool    `json:"contact_created"`
}

// CaptureLead upserts a contact (dedup by email — the *same* capability call
// the connector candidate path uses), optionally links a company, and opens a
// deal in the first pipeline stage. Additive (non-destructive), idempotent,
// audited.
func (s *Service) CaptureLead(ctx context.Context, p Principal, in CaptureLeadInput, opts WriteOptions) (WriteResult, error) {
	if strings.TrimSpace(in.Name) == "" {
		return s.guardedValidationWrite(p, opts, "capture_lead", &ValidationError{Msg: "name is required"})
	}
	if strings.TrimSpace(in.Source) == "" {
		return s.guardedValidationWrite(p, opts, "capture_lead", &ValidationError{Msg: "source is required"})
	}

	gw := GuardedWrite{Operation: "capture_lead", Options: opts}
	first, last := splitName(in.Name)

	plan := func() (Preview, error) {
		after := map[string]any{"first_name": first, "last_name": last, "source": in.Source}
		if in.Email != nil {
			after["email"] = *in.Email
		}
		if in.Company != nil {
			after["company"] = *in.Company
		}
		return Preview{
			Summary: fmt.Sprintf("would capture lead %q (source %q) and open a deal in the first stage", in.Name, in.Source),
			Effects: []Effect{
				{Action: "create", EntityType: EntityTypeContact, After: after},
				{Action: "create", EntityType: EntityTypeDeal, After: map[string]any{"title": in.Name}},
			},
		}, nil
	}

	replay := s.idempotentReplay(ctx, p, opts.IdempotencyKey)

	apply := func() (int, []byte, error) {
		const status = 201
		var body []byte
		err := s.writeTx(ctx, p, func(tx pgx.Tx) error {
			email := ""
			if in.Email != nil {
				email = *in.Email
			}
			// Shared connector/MCP upsert: dedup by email, else create.
			contactID, created, e := UpsertContactByEmail(ctx, tx, UpsertContactParams{
				FirstName: first, LastName: last, Email: email,
			})
			if e != nil {
				return e
			}

			var companyID *uuid.UUID
			if in.Company != nil && strings.TrimSpace(*in.Company) != "" {
				cid, e := findOrCreateCompanyByName(ctx, tx, *in.Company)
				if e != nil {
					return e
				}
				companyID = &cid
				// Link the contact to the company when it has none yet.
				if _, e := tx.Exec(ctx,
					`UPDATE contacts SET company_id = COALESCE(company_id, $2), updated_at = now() WHERE id = $1`,
					contactID, cid); e != nil {
					return e
				}
			}

			// Source as a custom property on the contact (mergeable bag).
			if e := MergeCustomProps(ctx, tx, EntityTypeContact, contactID, map[string]any{"source": in.Source}); e != nil {
				return e
			}

			// Open a deal in the first pipeline stage.
			stageID, _, e := firstPipelineStage(ctx, tx)
			if e != nil {
				return e
			}
			var dealID uuid.UUID
			if e := tx.QueryRow(ctx,
				`INSERT INTO deals (title, stage_id, contact_id, company_id) VALUES ($1, $2, $3, $4) RETURNING id`,
				in.Name, stageID, contactID, nullUUIDPtr(companyID)).Scan(&dealID); e != nil {
				return e
			}

			if e := s.emitEntityActivity(ctx, tx, p, EntityTypeContact, contactID, "lead.captured", map[string]any{
				"source": in.Source, "email": email, "deal_id": dealID.String(),
			}); e != nil {
				return e
			}
			if e := s.emitEntityActivity(ctx, tx, p, EntityTypeDeal, dealID, "entity.created", map[string]any{
				"title": in.Name, "stage_id": stageID.String(), "source": in.Source,
			}); e != nil {
				return e
			}
			if e := EmitAudit(ctx, tx, "lead.captured", p.WorkspaceID, p.ActorType, map[string]any{
				"contact_id": contactID.String(), "deal_id": dealID.String(), "source": in.Source,
			}); e != nil {
				return e
			}

			res := CaptureLeadResult{ContactID: contactID.String(), DealID: dealID.String(), ContactCreated: created}
			if companyID != nil {
				s := companyID.String()
				res.CompanyID = &s
			}
			body, e = json.Marshal(res)
			if e != nil {
				return e
			}
			return s.storeIdempotent(ctx, tx, p, opts.IdempotencyKey, "capture_lead", status, body)
		})
		return status, body, err
	}

	return gw.Run(p, replay, plan, apply)
}

// =========================================================================
// shared upsert (the single contact-upsert path; connector + MCP)
// =========================================================================

// UpsertContactParams is the input to UpsertContactByEmail.
type UpsertContactParams struct {
	FirstName string
	LastName  string
	Email     string
	Phone     string
}

// UpsertContactByEmail dedups a contact by email inside tx (search_path
// already pinned to the workspace schema). When Email is non-empty and an
// existing contact carries it, the supplied non-blank fields are merged via
// COALESCE and (id, false) is returned. Otherwise a new contact is inserted
// and (id, true) is returned. Names are NOT NULL in the schema, so blanks
// insert as ”.
//
// This is the single contact-upsert path shared by capture_lead (MCP) and the
// connector candidate ingestion (apps/api/internal/crm/connectors.go) — the
// same capability call, different door (ADR-012 §3).
func UpsertContactByEmail(ctx context.Context, tx pgx.Tx, in UpsertContactParams) (uuid.UUID, bool, error) {
	if in.Email != "" {
		var id uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT id FROM contacts WHERE email = $1 ORDER BY created_at LIMIT 1`, in.Email).Scan(&id)
		if err == nil {
			if _, e := tx.Exec(ctx,
				`UPDATE contacts SET
				   first_name = COALESCE(NULLIF($2,''), first_name),
				   last_name  = COALESCE(NULLIF($3,''), last_name),
				   email      = COALESCE(NULLIF($4,''), email),
				   phone      = COALESCE(NULLIF($5,''), phone),
				   updated_at = now()
				 WHERE id = $1`,
				id, in.FirstName, in.LastName, in.Email, in.Phone); e != nil {
				return uuid.Nil, false, e
			}
			return id, false, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, false, err
		}
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO contacts (first_name, last_name, email, phone)
		 VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''))
		 RETURNING id`,
		in.FirstName, in.LastName, in.Email, in.Phone).Scan(&id); err != nil {
		return uuid.Nil, false, err
	}
	return id, true, nil
}

// =========================================================================
// resolution helpers (raw tx SQL, mirroring the connector path)
// =========================================================================

// resolveDealRef resolves a deal by UUID or by a fuzzy, case-insensitive
// title substring. Zero matches → ErrNotFound; more than one → ErrAmbiguous.
func resolveDealRef(ctx context.Context, tx pgx.Tx, ref string) (sqlcgen.Deal, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return sqlcgen.Deal{}, &ValidationError{Msg: "deal is required"}
	}
	q := sqlcgen.New(tx)
	if id, err := uuid.Parse(ref); err == nil {
		deal, e := q.GetDeal(ctx, id)
		if errors.Is(e, pgx.ErrNoRows) {
			return sqlcgen.Deal{}, ErrNotFound
		}
		return deal, e
	}
	rows, err := tx.Query(ctx,
		`SELECT id FROM deals WHERE title ILIKE '%' || $1 || '%' ORDER BY created_at DESC LIMIT 5`, ref)
	if err != nil {
		return sqlcgen.Deal{}, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if e := rows.Scan(&id); e != nil {
			rows.Close()
			return sqlcgen.Deal{}, e
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return sqlcgen.Deal{}, err
	}
	switch len(ids) {
	case 0:
		return sqlcgen.Deal{}, ErrNotFound
	case 1:
		return q.GetDeal(ctx, ids[0])
	default:
		return sqlcgen.Deal{}, fmt.Errorf("%w: deal %q matches %d deals", ErrAmbiguous, ref, len(ids))
	}
}

// resolveStageFuzzy resolves a pipeline stage by UUID or by name with a
// precision cascade: exact (case-insensitive) → prefix → substring. The first
// level with exactly one match wins; a level with more than one match is
// ErrAmbiguous; no match at all is ErrBadStage.
func resolveStageFuzzy(ctx context.Context, tx pgx.Tx, ref string) (uuid.UUID, string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return uuid.Nil, "", &ValidationError{Msg: "to_stage is required"}
	}
	if id, err := uuid.Parse(ref); err == nil {
		var name string
		switch e := tx.QueryRow(ctx, `SELECT name FROM pipeline_stages WHERE id = $1`, id).Scan(&name); {
		case errors.Is(e, pgx.ErrNoRows):
			return uuid.Nil, "", ErrBadStage
		case e != nil:
			return uuid.Nil, "", e
		}
		return id, name, nil
	}

	rows, err := tx.Query(ctx, `SELECT id, name FROM pipeline_stages ORDER BY order_index`)
	if err != nil {
		return uuid.Nil, "", err
	}
	type st struct {
		id   uuid.UUID
		name string
	}
	var all []st
	for rows.Next() {
		var s st
		if e := rows.Scan(&s.id, &s.name); e != nil {
			rows.Close()
			return uuid.Nil, "", e
		}
		all = append(all, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return uuid.Nil, "", err
	}

	lref := strings.ToLower(ref)
	var exact, prefix, sub []st
	for _, s := range all {
		ln := strings.ToLower(s.name)
		switch {
		case ln == lref:
			exact = append(exact, s)
		case strings.HasPrefix(ln, lref):
			prefix = append(prefix, s)
		case strings.Contains(ln, lref):
			sub = append(sub, s)
		}
	}
	for _, set := range [][]st{exact, prefix, sub} {
		switch len(set) {
		case 1:
			return set[0].id, set[0].name, nil
		case 0:
			continue
		default:
			return uuid.Nil, "", fmt.Errorf("%w: stage %q matches %d stages", ErrAmbiguous, ref, len(set))
		}
	}
	return uuid.Nil, "", ErrBadStage
}

// firstPipelineStage returns the lowest-ordered pipeline stage (the entry
// stage a captured lead's deal opens in).
func firstPipelineStage(ctx context.Context, tx pgx.Tx) (uuid.UUID, string, error) {
	var id uuid.UUID
	var name string
	err := tx.QueryRow(ctx, `SELECT id, name FROM pipeline_stages ORDER BY order_index LIMIT 1`).Scan(&id, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", &ValidationError{Msg: "workspace has no pipeline stages"}
	}
	return id, name, err
}

// findOrCreateCompanyByName finds a company by exact (case-insensitive) name
// or creates it. Used by capture_lead to attach a named company.
func findOrCreateCompanyByName(ctx context.Context, tx pgx.Tx, name string) (uuid.UUID, error) {
	name = strings.TrimSpace(name)
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM companies WHERE lower(name) = lower($1) ORDER BY created_at LIMIT 1`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}
	if e := tx.QueryRow(ctx, `INSERT INTO companies (name) VALUES ($1) RETURNING id`, name).Scan(&id); e != nil {
		return uuid.Nil, e
	}
	return id, nil
}

// resolveInteractionSubject resolves the subject of a logged interaction by
// UUID (contact then company), email (contact, upserting when absent), or
// name (contact by full name, else company by name, else upsert a contact).
// Returns the entity type, id, and whether a contact was created.
func resolveInteractionSubject(ctx context.Context, tx pgx.Tx, ref string) (string, uuid.UUID, bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", uuid.Nil, false, &ValidationError{Msg: "contact_or_company is required"}
	}

	// UUID: match a contact, then a company.
	if id, err := uuid.Parse(ref); err == nil {
		var exists bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contacts WHERE id = $1)`, id).Scan(&exists); e != nil {
			return "", uuid.Nil, false, e
		}
		if exists {
			return EntityTypeContact, id, false, nil
		}
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM companies WHERE id = $1)`, id).Scan(&exists); e != nil {
			return "", uuid.Nil, false, e
		}
		if exists {
			return EntityTypeCompany, id, false, nil
		}
		return "", uuid.Nil, false, ErrNotFound
	}

	// Email: dedup/upsert a contact.
	if strings.Contains(ref, "@") {
		id, created, e := UpsertContactByEmail(ctx, tx, UpsertContactParams{Email: ref})
		if e != nil {
			return "", uuid.Nil, false, e
		}
		return EntityTypeContact, id, created, nil
	}

	// Name: contact by full name, then company by name.
	if id, ok, e := matchContactByName(ctx, tx, ref); e != nil {
		return "", uuid.Nil, false, e
	} else if ok {
		return EntityTypeContact, id, false, nil
	}
	var companyID uuid.UUID
	switch e := tx.QueryRow(ctx, `SELECT id FROM companies WHERE lower(name) = lower($1) ORDER BY created_at LIMIT 1`, ref).Scan(&companyID); {
	case e == nil:
		return EntityTypeCompany, companyID, false, nil
	case !errors.Is(e, pgx.ErrNoRows):
		return "", uuid.Nil, false, e
	}

	// Nothing matched: upsert a contact from the bare name (S2 "upsert if needed").
	first, last := splitName(ref)
	id, created, e := UpsertContactByEmail(ctx, tx, UpsertContactParams{FirstName: first, LastName: last})
	if e != nil {
		return "", uuid.Nil, false, e
	}
	return EntityTypeContact, id, created, nil
}

// matchContactByName returns the single contact whose "first last" matches ref
// (case-insensitive). Zero matches → (_, false, nil); more than one →
// ErrAmbiguous so the caller doesn't silently log against the wrong person.
func matchContactByName(ctx context.Context, tx pgx.Tx, ref string) (uuid.UUID, bool, error) {
	rows, err := tx.Query(ctx,
		`SELECT id FROM contacts WHERE lower(trim(first_name || ' ' || last_name)) = lower($1) ORDER BY created_at LIMIT 5`, ref)
	if err != nil {
		return uuid.Nil, false, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if e := rows.Scan(&id); e != nil {
			rows.Close()
			return uuid.Nil, false, e
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return uuid.Nil, false, err
	}
	switch len(ids) {
	case 0:
		return uuid.Nil, false, nil
	case 1:
		return ids[0], true, nil
	default:
		return uuid.Nil, false, fmt.Errorf("%w: %q matches %d contacts", ErrAmbiguous, ref, len(ids))
	}
}

// stageNameOf returns the name of a (nullable) stage id, or nil when unset or
// missing. Used for the dry-run "before" diff.
func stageNameOf(ctx context.Context, tx pgx.Tx, stage uuid.NullUUID) any {
	if !stage.Valid {
		return nil
	}
	var name string
	if err := tx.QueryRow(ctx, `SELECT name FROM pipeline_stages WHERE id = $1`, stage.UUID).Scan(&name); err != nil {
		return nil
	}
	return name
}

// =========================================================================
// small shared helpers
// =========================================================================

// idempotentReplay returns a GuardedWrite replay closure that looks up the
// idempotency cache for key (or a never-replays closure when key is empty).
func (s *Service) idempotentReplay(ctx context.Context, p Principal, key string) func() (int, []byte, bool, error) {
	if key == "" {
		return nil
	}
	return func() (int, []byte, bool, error) {
		return IdempotencyLookup(ctx, s.Pool, p.WorkspaceID, key)
	}
}

// storeIdempotent persists the canonical response under key inside tx (no-op
// when key is empty), so the mutation + audit + key insert commit atomically.
func (s *Service) storeIdempotent(ctx context.Context, tx pgx.Tx, p Principal, key, op string, status int, body []byte) error {
	if key == "" {
		return nil
	}
	return IdempotencyStore(ctx, tx, p.WorkspaceID, key, "MCP", op, status, body)
}

// guardedValidationWrite runs the scope/role gate (and confirmation/replay
// ordering) for a write whose arguments are already known invalid, so a
// read-only token still sees the scope denial first and a write-scoped token
// sees the validation error — never leaking which failed to an unauthorized
// caller.
func (s *Service) guardedValidationWrite(p Principal, opts WriteOptions, op string, valErr error) (WriteResult, error) {
	gw := GuardedWrite{Operation: op, Options: opts}
	return gw.Run(p,
		nil,
		func() (Preview, error) { return Preview{}, valErr },
		func() (int, []byte, error) { return 0, nil, valErr },
	)
}

// splitName splits a full name into (first, last) on the first space. A
// single token becomes the first name with an empty last name.
func splitName(name string) (string, string) {
	name = strings.TrimSpace(name)
	if i := strings.IndexByte(name, ' '); i >= 0 {
		return strings.TrimSpace(name[:i]), strings.TrimSpace(name[i+1:])
	}
	return name, ""
}

func derefStr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func nullUUIDPtr(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

// closedAtTime resolves a mark_closed_at request to a concrete time: an
// empty / "today" / "now" value (or an unparseable one) uses now; a
// YYYY-MM-DD value uses that calendar date (UTC midnight).
func closedAtTime(markClosedAt *string, now time.Time) time.Time {
	if markClosedAt == nil {
		return now
	}
	v := strings.TrimSpace(strings.ToLower(*markClosedAt))
	if v == "" || v == "today" || v == "now" {
		return now
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t
	}
	return now
}
