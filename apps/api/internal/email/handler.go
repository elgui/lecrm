package email

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/internal/email/brevo"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

// maxSendBodyBytes caps inbound JSON payloads. 256 KiB is generous for a
// single transactional email (HTML body included) and matches the limit
// Brevo enforces on /v3/smtp/email.
const maxSendBodyBytes = 256 * 1024

// maxWebhookBodyBytes caps Brevo webhook payloads. The HMAC verifier
// must see the *exact* bytes the signature covered, so we use a hard
// limit rather than json.NewDecoder. 64 KiB covers Brevo's documented
// event payload sizes with room.
const maxWebhookBodyBytes = 64 * 1024

// WebhookSecretSource returns the per-workspace HMAC secret for the
// inbound webhook. Implementations sit on top of the secrets store
// (SOPS at v0, Vault at v1+). The interface stays small here to keep
// the email package decoupled from secrets internals.
type WebhookSecretSource interface {
	WebhookSecret(workspaceID uuid.UUID) ([]byte, error)
}

// StaticWebhookSecret is a single-secret implementation suitable for
// dev/single-tenant scenarios. Production wires a real per-workspace
// resolver.
type StaticWebhookSecret []byte

// WebhookSecret returns the same secret regardless of workspace id.
func (s StaticWebhookSecret) WebhookSecret(uuid.UUID) ([]byte, error) {
	if len(s) == 0 {
		return nil, errors.New("email: static webhook secret is empty")
	}
	return s, nil
}

// Handler wires the email HTTP endpoints into the v0 chi router.
//
// Routes:
//
//	POST /v1/workspaces/{id}/emails             — send a transactional email
//	POST /v1/email/webhooks/brevo               — inbound event receiver
//
// The send endpoint is workspace-scoped (URL carries the workspace id and
// must match the resolved workspace from the subdomain/session). The
// webhook is unauthenticated *at the chi layer* — its authentication is
// the HMAC signature over the request body.
type Handler struct {
	Service       *Service
	Logger        *slog.Logger
	WebhookSource WebhookSecretSource
}

// RegisterRoutes mounts the email endpoints. The send route is mounted
// inside the workspace-middleware group; the webhook is mounted
// unauthenticated by the caller (NewRouter) outside that group.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/workspaces/{id}/emails", h.handleSend)
}

// RegisterWebhookRoute mounts the inbound-webhook endpoint on the
// unauthenticated router branch. Verification happens via HMAC.
func (h *Handler) RegisterWebhookRoute(r chi.Router) {
	r.Post("/v1/email/webhooks/brevo", h.handleWebhook)
}

type sendRequestJSON struct {
	From        brevo.Address   `json:"from"`
	To          []brevo.Address `json:"to"`
	Subject     string          `json:"subject"`
	HTMLContent string          `json:"html_content,omitempty"`
	TextContent string          `json:"text_content,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
}

func (h *Handler) handleSend(w http.ResponseWriter, r *http.Request) {
	urlID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid workspace id")
		return
	}

	ws, err := workspace.WorkspaceFromContext(r.Context())
	if err != nil {
		// Middleware not wired — surfacing 500 not 401 per ADR-009 to
		// avoid leaking routing-layer state.
		h.Logger.ErrorContext(r.Context(), "email: workspace ctx missing", "err", err)
		writeJSONErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	if ws.ID != urlID {
		writeJSONErr(w, http.StatusForbidden, "workspace id mismatch")
		return
	}

	actor, userID, ok := actorFromRequest(r)
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "missing or invalid actor_type")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxSendBodyBytes)
	var body sendRequestJSON
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Subject == "" {
		writeJSONErr(w, http.StatusBadRequest, "subject required")
		return
	}
	if body.From.Email == "" {
		writeJSONErr(w, http.StatusBadRequest, "from.email required")
		return
	}
	if len(body.To) == 0 {
		writeJSONErr(w, http.StatusBadRequest, "to[] required")
		return
	}
	if body.HTMLContent == "" && body.TextContent == "" {
		writeJSONErr(w, http.StatusBadRequest, "html_content or text_content required")
		return
	}

	req := SendRequest{
		WorkspaceID: ws.ID,
		Schema:      ws.RoleName,
		ActorType:   actor,
		ActorUserID: userID,
		RequestID:   requestIDFrom(r),
		From:        body.From,
		To:          body.To,
		Subject:     body.Subject,
		HTMLContent: body.HTMLContent,
		TextContent: body.TextContent,
		Tags:        body.Tags,
	}

	outcome, err := h.Service.Send(r.Context(), req)
	if errors.Is(err, ErrPausedByAlarm) {
		w.Header().Set("Retry-After", "3600")
		writeJSONErr(w, http.StatusServiceUnavailable, "sends paused: bounce-rate alarm")
		return
	}
	if errors.Is(err, ErrInvalidActor) {
		writeJSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "email: send failed", "err", err)
		writeJSONErr(w, http.StatusBadGateway, "send failed")
		return
	}

	status := http.StatusAccepted
	if outcome.Skipped {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{
		"message_id":         outcome.MessageID,
		"skipped":            outcome.Skipped,
		"skipped_recipients": outcome.SkippedRecipients,
		"reason":             outcome.Reason,
	})
}

// handleWebhook is the inbound Brevo webhook receiver. The HMAC over the
// raw body is the authentication; we never trust any header or claim in
// the JSON payload itself.
func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(http.MaxBytesReader(w, r.Body, maxWebhookBodyBytes), maxWebhookBodyBytes+1))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "read body")
		return
	}
	if len(body) > maxWebhookBodyBytes {
		writeJSONErr(w, http.StatusRequestEntityTooLarge, "webhook payload too large")
		return
	}

	// Workspace id MUST be carried in a query parameter or header so the
	// receiver can pick the correct HMAC secret. Brevo doesn't sign the
	// URL; we ask the operator to configure a workspace-scoped webhook
	// URL (?workspace=<uuid>) at the Brevo dashboard. The HMAC then
	// covers both the body and (transitively, via the secret) the
	// workspace claim.
	wsParam := r.URL.Query().Get("workspace")
	wsID, err := uuid.Parse(wsParam)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "missing or invalid ?workspace=")
		return
	}

	secret, err := h.WebhookSource.WebhookSecret(wsID)
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "email: webhook secret lookup failed",
			"err", err, "workspace_id", wsID)
		writeJSONErr(w, http.StatusInternalServerError, "secret lookup failed")
		return
	}

	sig := r.Header.Get(brevo.SignatureHeader)
	if sig == "" {
		writeJSONErr(w, http.StatusUnauthorized, "missing signature header")
		return
	}
	if err := brevo.VerifySignature(secret, body, sig); err != nil {
		if errors.Is(err, brevo.ErrInvalidSignature) {
			writeJSONErr(w, http.StatusUnauthorized, "invalid webhook signature")
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, "signature verify error")
		return
	}

	ev, err := brevo.ParseEvent(body)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if !ev.IsKnown() {
		// 200 + warning log: don't make Brevo retry, but flag that our
		// event schema may have drifted.
		h.Logger.WarnContext(r.Context(), "email: unknown brevo event",
			"workspace_id", wsID, "event", string(ev.Event))
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored_unknown_event"})
		return
	}

	// Workspace schema lookup: in production, resolve via the workspace
	// resolver. At v0 the schema name is derivable from the workspace id
	// (workspace_<base32-of-uuid>). We use the same convention here.
	schema := workspaceSchemaName(wsID)

	if err := h.Service.IngestEvent(r.Context(), wsID, schema, ev); err != nil {
		h.Logger.ErrorContext(r.Context(), "email: ingest event", "err", err,
			"workspace_id", wsID, "event", string(ev.Event))
		writeJSONErr(w, http.StatusInternalServerError, "ingest failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

// workspaceSchemaName returns the schema name for a workspace id, matching
// core.lecrm_provision_workspace's naming rule (workspace_<uuid-no-dashes>).
func workspaceSchemaName(id uuid.UUID) string {
	s := id.String()
	out := make([]byte, 0, len("workspace_")+32)
	out = append(out, []byte("workspace_")...)
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			continue
		}
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	return string(out)
}

// actorFromRequest extracts the actor_type and (optional) user id from
// the request context. At v0 the only call paths are:
//   - a logged-in admin session (auth.SessionContext present) →
//     "human_api" + that user's id.
//   - an internal-service caller (header X-Lecrm-Actor: internal_service)
//     → "internal_service" with no user id.
//
// When a service-token-claim middleware lands (per ADR-009 §4.1) this
// function becomes a one-line read of the claim. Keeping it as a helper
// today means the upgrade is local.
func actorFromRequest(r *http.Request) (ActorType, uuid.UUID, bool) {
	if v := r.Header.Get("X-Lecrm-Actor"); v != "" {
		at := ActorType(v)
		if !IsValidSendActor(at) {
			return "", uuid.Nil, false
		}
		var uid uuid.UUID
		if u := r.Header.Get("X-Lecrm-Actor-User-Id"); u != "" {
			parsed, err := uuid.Parse(u)
			if err != nil {
				return "", uuid.Nil, false
			}
			uid = parsed
		}
		return at, uid, true
	}
	// Default: rely on session presence (workspace middleware has run).
	// If neither header nor session is present we still accept the call
	// as human_api with no user id; sessionful workflows can refine.
	return ActorHumanAPI, uuid.Nil, true
}

func requestIDFrom(r *http.Request) uuid.UUID {
	v := r.Header.Get("X-Request-Id")
	if v == "" {
		return uuid.Nil
	}
	id, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil
	}
	return id
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
