package crm

// Connector event ingestion endpoint (ADR-011 — chatboting connector
// boundary). External systems (chatboting first; future connectors
// next) POST asynchronous domain events to
//
//	POST /v1/connectors/{source}/events
//
// authenticated by a workspace-scoped service token carrying the
// `connector.push_events` scope (ADR-009 §4.1). Each event is mapped to
// a CRM mutation: contacts are upserted, deals are created and advanced
// through pipeline stages, and an append-only activity row is written
// for the timeline. All mutations are attributed actor_type=`connector`
// with source_system set to the {source} path segment, and are
// audit-logged fail-closed (ADR-009 §7.2).
//
// Idempotency: every envelope carries an `idempotency_key`. The first
// delivery processes and caches its response in core.idempotency_keys;
// duplicate deliveries replay the cached 200 without re-mutating.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gbconsult/lecrm/apps/api/capability"
	"github.com/gbconsult/lecrm/apps/api/internal/auth"
)

// ConnectorPushScope is the service-token scope required to push events.
const ConnectorPushScope = "connector.push_events"

// Connector event types (ADR-011 §4 event table). The handler rejects
// any event name outside this set with 400.
const (
	evtCandidateEnriched  = "candidate.enriched"
	evtInvitationCreated  = "invitation.created"
	evtInvitationSent     = "invitation.sent"
	evtInvitationOpened   = "invitation.opened"
	evtInvitationClaimed  = "invitation.claimed"
	evtInvitationExpired  = "invitation.expired"
	evtInvitationReplyPos = "invitation.reply_positive"
)

var knownConnectorEvents = map[string]struct{}{
	evtCandidateEnriched:  {},
	evtInvitationCreated:  {},
	evtInvitationSent:     {},
	evtInvitationOpened:   {},
	evtInvitationClaimed:  {},
	evtInvitationExpired:  {},
	evtInvitationReplyPos: {},
}

// Pipeline stage names the connector advances deals through. "Closed-Won"
// and "Closed-Lost" resolve to the seeded combined "Closed-Won/Lost"
// stage via resolveStage's Closed-prefix fallback.
const (
	stageDiscovery    = "Discovery"
	stageProposalSent = "Proposal Sent"
	stageClosedWon    = "Closed-Won"
	stageClosedLost   = "Closed-Lost"
)

// connectorEvent is the ADR-011 §4 event envelope.
type connectorEvent struct {
	Event          string          `json:"event"`
	Source         string          `json:"source"`
	Timestamp      string          `json:"timestamp"`
	IdempotencyKey string          `json:"idempotency_key"`
	Workspace      string          `json:"workspace"`
	Payload        json.RawMessage `json:"payload"`
}

type candidatePayload struct {
	URL       string   `json:"url"`
	Email     string   `json:"email"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Phone     string   `json:"phone"`
	Score     *float64 `json:"score"`
	CMS       string   `json:"cms"`
	Geo       string   `json:"geo"`
	Category  string   `json:"category"`
}

type candidateEnvelope struct {
	Candidate candidatePayload `json:"candidate"`
}

type invitationPayload struct {
	ID             string   `json:"id"`
	CandidateURL   string   `json:"candidate_url"`
	CandidateEmail string   `json:"candidate_email"`
	Title          string   `json:"title"`
	Amount         *float64 `json:"amount"`
	TenantURL      string   `json:"tenant_url"`
}

type invitationEnvelope struct {
	Invitation invitationPayload `json:"invitation"`
}

// RegisterConnectorRoutes mounts the connector ingestion endpoint. It
// MUST be wrapped by RequireConnectorScope (applied in the router) so
// only `connector.push_events` tokens reach the handler.
func (h *Handler) RegisterConnectorRoutes(r chi.Router) {
	r.Post("/v1/connectors/{source}/events", h.PostConnectorEvent)
}

// RequireConnectorScope is middleware that admits only verified service
// tokens carrying the connector.push_events scope. 401 when no bearer
// actor authenticated the request; 403 when the actor lacks the scope.
func RequireConnectorScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := auth.BearerActorFromContext(r.Context())
		status := authorizeConnector(actor, ok)
		switch status {
		case http.StatusOK:
			next.ServeHTTP(w, r)
		case http.StatusUnauthorized:
			writeErr(w, http.StatusUnauthorized, "connector authentication required")
		default:
			writeErr(w, http.StatusForbidden, "connector.push_events scope required")
		}
	})
}

// authorizeConnector returns the HTTP status a connector request should
// receive based on the verified actor. Pure for unit-testing.
func authorizeConnector(actor *auth.BearerActor, present bool) int {
	if !present || actor == nil {
		return http.StatusUnauthorized
	}
	for _, s := range actor.Scopes {
		if s == "*" || s == ConnectorPushScope {
			return http.StatusOK
		}
	}
	return http.StatusForbidden
}

// PostConnectorEvent ingests one connector event.
func (h *Handler) PostConnectorEvent(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	source := chi.URLParam(r, "source")
	if source == "" {
		writeErr(w, http.StatusBadRequest, "missing source")
		return
	}

	var ev connectorEvent
	if !decodeBody(w, r, &ev) {
		return
	}
	if status, msg := validateConnectorEvent(&ev, source, ws.Slug); status != http.StatusOK {
		writeErr(w, status, msg)
		return
	}

	// Idempotency replay: a duplicate key returns the cached 200 without
	// re-mutating (ADR-011 §4 — at-least-once delivery from connectors).
	if st, cached, hit, ok := h.replayIdempotent(w, r, ws.ID, ev.IdempotencyKey); ok && hit {
		writeReplay(w, st, cached)
		return
	} else if !ok {
		return
	}

	respBody, _ := json.Marshal(map[string]any{
		"status": "processed",
		"event":  ev.Event,
		"source": source,
	})
	const respStatus = http.StatusOK

	err := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		if err := h.handleConnectorEvent(r.Context(), tx, ws.ID, source, &ev); err != nil {
			return err
		}
		return idempotencyStore(r.Context(), tx, ws.ID, ev.IdempotencyKey, r.Method, r.URL.Path, respStatus, respBody)
	})
	if errors.Is(err, errBadPayload) {
		writeErr(w, http.StatusBadRequest, "invalid event payload")
		return
	}
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "connector event", "err", err, "event", ev.Event, "source", source)
		writeErr(w, http.StatusInternalServerError, "connector event processing failed")
		return
	}
	writeRaw(w, respStatus, respBody)
}

var errBadPayload = errors.New("connector: bad payload")

// validateConnectorEvent enforces the envelope contract. Returns
// http.StatusOK + "" when valid.
func validateConnectorEvent(ev *connectorEvent, urlSource, resolvedSlug string) (int, string) {
	if _, ok := knownConnectorEvents[ev.Event]; !ok {
		return http.StatusBadRequest, "unknown event type"
	}
	if ev.IdempotencyKey == "" {
		return http.StatusBadRequest, "idempotency_key is required"
	}
	if len(ev.IdempotencyKey) > maxIdempotencyKeyLen {
		return http.StatusBadRequest, "idempotency_key too long"
	}
	if ev.Source != "" && ev.Source != urlSource {
		return http.StatusBadRequest, "source mismatch between body and path"
	}
	// Cross-tenant guard: the envelope's declared workspace must match the
	// workspace the (token-scoped) request resolved to. A token for A
	// declaring workspace B is a cross-tenant push attempt → 403.
	if ev.Workspace != "" && ev.Workspace != resolvedSlug {
		return http.StatusForbidden, "workspace mismatch"
	}
	return http.StatusOK, ""
}

// handleConnectorEvent dispatches one validated event to its handler,
// running inside the caller's write transaction (search_path already set
// to the workspace schema).
func (h *Handler) handleConnectorEvent(ctx context.Context, tx pgx.Tx, wsID uuid.UUID, source string, ev *connectorEvent) error {
	switch ev.Event {
	case evtCandidateEnriched:
		return h.handleCandidateEnriched(ctx, tx, wsID, source, ev.Payload)
	case evtInvitationCreated:
		return h.handleInvitationStage(ctx, tx, wsID, source, ev, stageDiscovery, false)
	case evtInvitationSent:
		return h.handleInvitationStage(ctx, tx, wsID, source, ev, stageProposalSent, false)
	case evtInvitationOpened:
		return h.handleInvitationStage(ctx, tx, wsID, source, ev, "", false)
	case evtInvitationClaimed:
		return h.handleInvitationStage(ctx, tx, wsID, source, ev, stageClosedWon, true)
	case evtInvitationExpired:
		return h.handleInvitationStage(ctx, tx, wsID, source, ev, stageClosedLost, true)
	case evtInvitationReplyPos:
		return h.handleInvitationReplyPositive(ctx, tx, wsID, source, ev)
	default:
		return errBadPayload
	}
}

// --- candidate.enriched ---

func (h *Handler) handleCandidateEnriched(ctx context.Context, tx pgx.Tx, wsID uuid.UUID, source string, payload json.RawMessage) error {
	var env candidateEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return errBadPayload
	}
	c := env.Candidate
	if c.URL == "" && c.Email == "" {
		return errBadPayload
	}

	contactID, err := h.upsertConnectorContact(ctx, tx, source, c)
	if err != nil {
		return err
	}

	// Set enrichment custom properties (merged with any existing bag).
	props := map[string]any{}
	if c.Score != nil {
		props["score"] = *c.Score
	}
	if c.CMS != "" {
		props["cms"] = c.CMS
	}
	if c.Geo != "" {
		props["geo"] = c.Geo
	}
	if c.Category != "" {
		props["category"] = c.Category
	}
	if len(props) > 0 {
		if err := capability.MergeCustomProps(ctx, tx, capability.EntityTypeContact, contactID, props); err != nil {
			return err
		}
	}

	if err := capability.EmitActivity(ctx, tx, capability.EntityTypeContact, contactID, evtCandidateEnriched, capability.ActorTypeConnector, source, map[string]any{
		"url":      c.URL,
		"email":    c.Email,
		"category": c.Category,
	}); err != nil {
		return err
	}
	return capability.EmitAudit(ctx, tx, "connector.contact.enriched", wsID, capability.ActorTypeConnector, map[string]any{
		"id":     contactID.String(),
		"source": source,
	})
}

// upsertConnectorContact matches an existing contact by the connector's
// external URL (via external_entity_mappings) or by email, updating it;
// otherwise it creates a new contact and records the mapping.
func (h *Handler) upsertConnectorContact(ctx context.Context, tx pgx.Tx, source string, c candidatePayload) (uuid.UUID, error) {
	var contactID uuid.UUID
	found := false

	if c.URL != "" {
		err := tx.QueryRow(ctx,
			`SELECT entity_id FROM external_entity_mappings
			  WHERE provider_id = $1 AND external_id = $2 AND entity_type = 'contact'`,
			source, c.URL).Scan(&contactID)
		if err == nil {
			found = true
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, err
		}
	}
	if !found && c.Email != "" {
		err := tx.QueryRow(ctx,
			`SELECT id FROM contacts WHERE email = $1 ORDER BY created_at LIMIT 1`, c.Email).Scan(&contactID)
		if err == nil {
			found = true
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, err
		}
	}

	if found {
		// Update only the fields the connector supplied (COALESCE keeps
		// existing values when the payload omits a field).
		_, err := tx.Exec(ctx,
			`UPDATE contacts SET
			   first_name = COALESCE(NULLIF($2,''), first_name),
			   last_name  = COALESCE(NULLIF($3,''), last_name),
			   email      = COALESCE(NULLIF($4,''), email),
			   phone      = COALESCE(NULLIF($5,''), phone),
			   updated_at = now()
			 WHERE id = $1`,
			contactID, c.FirstName, c.LastName, c.Email, c.Phone)
		if err != nil {
			return uuid.Nil, err
		}
		return contactID, nil
	}

	// Create. Names are NOT NULL in the schema — default blanks to empty
	// string when the connector only supplied a URL/email.
	if err := tx.QueryRow(ctx,
		`INSERT INTO contacts (first_name, last_name, email, phone)
		 VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''))
		 RETURNING id`,
		c.FirstName, c.LastName, c.Email, c.Phone).Scan(&contactID); err != nil {
		return uuid.Nil, err
	}
	if c.URL != "" {
		if _, err := tx.Exec(ctx,
			`INSERT INTO external_entity_mappings (provider_id, external_id, entity_type, entity_id)
			 VALUES ($1, $2, 'contact', $3)
			 ON CONFLICT (provider_id, external_id) DO NOTHING`,
			source, c.URL, contactID); err != nil {
			return uuid.Nil, err
		}
	}
	return contactID, nil
}

// --- invitation.* ---

// handleInvitationStage resolves (or lazily creates) the deal mapped to
// the invitation, optionally moves it to targetStage, sets closed_at
// when `closing`, and writes an activity. An empty targetStage means
// "do not change stage" (invitation.opened).
func (h *Handler) handleInvitationStage(ctx context.Context, tx pgx.Tx, wsID uuid.UUID, source string, ev *connectorEvent, targetStage string, closing bool) error {
	var env invitationEnvelope
	if err := json.Unmarshal(ev.Payload, &env); err != nil {
		return errBadPayload
	}
	inv := env.Invitation
	if inv.ID == "" {
		return errBadPayload
	}

	dealID, err := h.findOrCreateInvitationDeal(ctx, tx, source, inv)
	if err != nil {
		return err
	}

	activity := map[string]any{"invitation_id": inv.ID}
	if inv.TenantURL != "" {
		activity["tenant_url"] = inv.TenantURL
	}

	if targetStage != "" {
		stageID, stageName, err := resolveStage(ctx, tx, targetStage)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE deals SET stage_id = $2,
			    closed_at = CASE WHEN $3 THEN now() ELSE closed_at END,
			    updated_at = now()
			  WHERE id = $1`,
			dealID, stageID, closing); err != nil {
			return err
		}
		activity["new_stage"] = stageID.String()
		activity["new_stage_name"] = stageName
		activity["requested_stage"] = targetStage
	}

	if err := capability.EmitActivity(ctx, tx, capability.EntityTypeDeal, dealID, ev.Event, capability.ActorTypeConnector, source, activity); err != nil {
		return err
	}
	return capability.EmitAudit(ctx, tx, "connector."+ev.Event, wsID, capability.ActorTypeConnector, map[string]any{
		"deal_id": dealID.String(),
		"source":  source,
	})
}

// handleInvitationReplyPositive records a positive reply and flags the
// deal for follow-up (custom property) without changing its stage.
func (h *Handler) handleInvitationReplyPositive(ctx context.Context, tx pgx.Tx, wsID uuid.UUID, source string, ev *connectorEvent) error {
	var env invitationEnvelope
	if err := json.Unmarshal(ev.Payload, &env); err != nil {
		return errBadPayload
	}
	inv := env.Invitation
	if inv.ID == "" {
		return errBadPayload
	}
	dealID, err := h.findOrCreateInvitationDeal(ctx, tx, source, inv)
	if err != nil {
		return err
	}
	if err := capability.MergeCustomProps(ctx, tx, capability.EntityTypeDeal, dealID, map[string]any{"follow_up": true}); err != nil {
		return err
	}
	if err := capability.EmitActivity(ctx, tx, capability.EntityTypeDeal, dealID, ev.Event, capability.ActorTypeConnector, source, map[string]any{
		"invitation_id": inv.ID,
		"follow_up":     true,
	}); err != nil {
		return err
	}
	return capability.EmitAudit(ctx, tx, "connector."+ev.Event, wsID, capability.ActorTypeConnector, map[string]any{
		"deal_id": dealID.String(),
		"source":  source,
	})
}

// findOrCreateInvitationDeal returns the deal mapped to inv.ID, creating
// it (at Discovery, linked to the candidate's contact when resolvable)
// on first sight. This makes invitation.* events resilient to
// out-of-order delivery.
func (h *Handler) findOrCreateInvitationDeal(ctx context.Context, tx pgx.Tx, source string, inv invitationPayload) (uuid.UUID, error) {
	var dealID uuid.UUID
	err := tx.QueryRow(ctx,
		`SELECT entity_id FROM external_entity_mappings
		  WHERE provider_id = $1 AND external_id = $2 AND entity_type = 'deal'`,
		source, inv.ID).Scan(&dealID)
	if err == nil {
		return dealID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}

	// Resolve the linked contact, if the invitation references one.
	contactID := h.resolveCandidateContact(ctx, tx, source, inv)

	title := inv.Title
	if title == "" {
		title = "Invitation " + inv.ID
	}
	stageID, _, err := resolveStage(ctx, tx, stageDiscovery)
	if err != nil {
		return uuid.Nil, err
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO deals (title, amount, stage_id, contact_id)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		title, toNumeric(inv.Amount), stageID, contactID).Scan(&dealID); err != nil {
		return uuid.Nil, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO external_entity_mappings (provider_id, external_id, entity_type, entity_id)
		 VALUES ($1, $2, 'deal', $3)
		 ON CONFLICT (provider_id, external_id) DO NOTHING`,
		source, inv.ID, dealID); err != nil {
		return uuid.Nil, err
	}
	return dealID, nil
}

// resolveCandidateContact looks up the contact linked to an invitation
// via the candidate URL mapping or email. Returns a NULL uuid when none
// is found (the deal is created unlinked).
func (h *Handler) resolveCandidateContact(ctx context.Context, tx pgx.Tx, source string, inv invitationPayload) uuid.NullUUID {
	var id uuid.UUID
	if inv.CandidateURL != "" {
		if err := tx.QueryRow(ctx,
			`SELECT entity_id FROM external_entity_mappings
			  WHERE provider_id = $1 AND external_id = $2 AND entity_type = 'contact'`,
			source, inv.CandidateURL).Scan(&id); err == nil {
			return uuid.NullUUID{UUID: id, Valid: true}
		}
	}
	if inv.CandidateEmail != "" {
		if err := tx.QueryRow(ctx,
			`SELECT id FROM contacts WHERE email = $1 ORDER BY created_at LIMIT 1`,
			inv.CandidateEmail).Scan(&id); err == nil {
			return uuid.NullUUID{UUID: id, Valid: true}
		}
	}
	return uuid.NullUUID{}
}

// resolveStage finds a pipeline stage by exact name, falling back to a
// Closed-prefix match so the connector's logical "Closed-Won"/"Closed-Lost"
// targets resolve to the seeded combined "Closed-Won/Lost" stage.
func resolveStage(ctx context.Context, tx pgx.Tx, name string) (uuid.UUID, string, error) {
	var id uuid.UUID
	var actual string
	err := tx.QueryRow(ctx,
		`SELECT id, name FROM pipeline_stages WHERE name = $1`, name).Scan(&id, &actual)
	if err == nil {
		return id, actual, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", err
	}
	if len(name) >= 6 && name[:6] == "Closed" {
		err = tx.QueryRow(ctx,
			`SELECT id, name FROM pipeline_stages WHERE name ILIKE 'Closed%' ORDER BY order_index LIMIT 1`).
			Scan(&id, &actual)
		if err == nil {
			return id, actual, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, "", err
		}
	}
	return uuid.Nil, "", errors.New("connector: pipeline stage not found: " + name)
}

// mergeCustomProps was removed; connector code now uses capability.MergeCustomProps directly.
