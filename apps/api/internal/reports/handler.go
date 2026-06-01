package reports

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

// AuditWriter is the seam Handler uses to record `report.embed_token.issued`
// events. The production binding writes to core.audit_log; tests inject a
// recorder.
type AuditWriter interface {
	WriteEmbedTokenAudit(ctx context.Context, workspaceID, actorID uuid.UUID) error
}

// SessionDecoder reads and verifies the request's session cookie scoped
// to the given workspace slug. The production binding is
// auth.SessionFromRequestV2; tests inject a stub.
type SessionDecoder func(r *http.Request, slug string) (auth.Session, bool)

// Handler serves POST /v1/reports/embed-token. Behavior:
//
//   - 401 if no/invalid session cookie.
//   - 403 if the session's workspace_id does not match the workspace
//     resolved from the request's subdomain.
//   - 500 if the audit insert fails — fail-closed per ADR-009 §7.2.
//   - 200 with {"token": "...", "expires_at": "..."} otherwise.
type Handler struct {
	JWTSecret     []byte
	TTL           time.Duration
	DecodeSession SessionDecoder
	Audit         AuditWriter
	Logger        *slog.Logger
	// Pool backs the native reporting endpoints (/v1/reports/run and the
	// /v1/reports/definitions CRUD). When nil those routes 503 — the Cube
	// embed-token route still works without it.
	Pool *pgxpool.Pool
}

// RegisterRoutes mounts the handler. Caller must wrap this in a router
// group that has the workspace middleware attached (the workspace
// context is required).
func (h *Handler) RegisterRoutes(r chi.Router) {
	// Cube.dev embed-token (deployments that provision Cube).
	r.Post("/v1/reports/embed-token", h.handleEmbedToken)

	// Native reporting (always available where the API+DB run, incl. the demo).
	r.Post("/v1/reports/run", h.handleRunReport)
	r.Get("/v1/reports/definitions", h.handleListDefinitions)
	r.Post("/v1/reports/definitions", h.handleCreateDefinition)
	r.Get("/v1/reports/definitions/{id}", h.handleGetDefinition)
	r.Put("/v1/reports/definitions/{id}", h.handleUpdateDefinition)
	r.Delete("/v1/reports/definitions/{id}", h.handleDeleteDefinition)
}

// authedStore resolves the workspace + session and returns a Store scoped to
// that workspace's schema. On any auth/config failure it writes the response
// and returns nil — callers must check for nil and return.
func (h *Handler) authedStore(w http.ResponseWriter, r *http.Request) *Store {
	if h.Pool == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "reporting not configured"})
		return nil
	}
	ws, err := workspace.WorkspaceFromContext(r.Context())
	if err != nil {
		h.log(r.Context()).Error("reports: no workspace in context (middleware misconfigured)")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "workspace context missing"})
		return nil
	}
	session, ok := h.DecodeSession(r, ws.Slug)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return nil
	}
	// Defence in depth: a session for tenant A must never act on tenant B.
	if session.WorkspaceID != ws.ID {
		h.log(r.Context()).Warn("reports: cross-workspace attempt",
			"session_workspace_id", session.WorkspaceID,
			"request_workspace_id", ws.ID, "actor_id", session.UserID)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "workspace mismatch"})
		return nil
	}
	return NewStore(h.Pool, ws.RoleName, ws.ID)
}

func (h *Handler) handleRunReport(w http.ResponseWriter, r *http.Request) {
	store := h.authedStore(w, r)
	if store == nil {
		return
	}
	var def Definition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	res, err := store.Run(r.Context(), def, time.Now())
	if h.handleStoreErr(w, r, err, "run report") {
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) handleListDefinitions(w http.ResponseWriter, r *http.Request) {
	store := h.authedStore(w, r)
	if store == nil {
		return
	}
	reports, err := store.ListSaved(r.Context())
	if h.handleStoreErr(w, r, err, "list report definitions") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": reports})
}

func (h *Handler) handleCreateDefinition(w http.ResponseWriter, r *http.Request) {
	store := h.authedStore(w, r)
	if store == nil {
		return
	}
	var def Definition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	sr, err := store.CreateSaved(r.Context(), def)
	if h.handleStoreErr(w, r, err, "create report definition") {
		return
	}
	writeJSON(w, http.StatusCreated, sr)
}

func (h *Handler) handleGetDefinition(w http.ResponseWriter, r *http.Request) {
	store := h.authedStore(w, r)
	if store == nil {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	sr, err := store.GetSaved(r.Context(), id)
	if h.handleStoreErr(w, r, err, "get report definition") {
		return
	}
	writeJSON(w, http.StatusOK, sr)
}

func (h *Handler) handleUpdateDefinition(w http.ResponseWriter, r *http.Request) {
	store := h.authedStore(w, r)
	if store == nil {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var def Definition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	sr, err := store.UpdateSaved(r.Context(), id, def)
	if h.handleStoreErr(w, r, err, "update report definition") {
		return
	}
	writeJSON(w, http.StatusOK, sr)
}

func (h *Handler) handleDeleteDefinition(w http.ResponseWriter, r *http.Request) {
	store := h.authedStore(w, r)
	if store == nil {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	err = store.DeleteSaved(r.Context(), id)
	if h.handleStoreErr(w, r, err, "delete report definition") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleStoreErr maps store errors to HTTP responses. Returns true when it
// wrote a response (the caller must then return).
func (h *Handler) handleStoreErr(w http.ResponseWriter, r *http.Request, err error, op string) bool {
	if err == nil {
		return false
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": ve.Msg})
		return true
	}
	if IsNotFound(err) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return true
	}
	h.log(r.Context()).Error("reports: "+op, "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": op + " failed"})
	return true
}

type embedTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (h *Handler) handleEmbedToken(w http.ResponseWriter, r *http.Request) {
	if len(h.JWTSecret) < 32 {
		h.log(r.Context()).Error("embed-token: JWT secret unset or too short")
		writeJSON(w, http.StatusServiceUnavailable,
			map[string]string{"error": "embed reporting disabled (LECRM_CUBE_JWT_SECRET unset)"})
		return
	}

	ws, err := workspace.WorkspaceFromContext(r.Context())
	if err != nil {
		h.log(r.Context()).Error("embed-token: no workspace in context (middleware misconfigured)")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "workspace context missing"})
		return
	}

	session, ok := h.DecodeSession(r, ws.Slug)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	// Cross-workspace check: a session for tenant A presenting on
	// tenant B's subdomain MUST be rejected. The session cookie is
	// workspace-bound (auth.DecodeSessionV2 verifies slug), but the
	// inner WorkspaceID is the source of truth — defense in depth.
	if session.WorkspaceID != ws.ID {
		h.log(r.Context()).Warn("embed-token: cross-workspace attempt",
			"session_workspace_id", session.WorkspaceID,
			"request_workspace_id", ws.ID,
			"actor_id", session.UserID)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "workspace mismatch"})
		return
	}

	ttl := h.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	now := time.Now()
	claims := EmbedClaims{
		WorkspaceID: ws.ID,
		Audience:    CubeAudience,
		IssuedAt:    now.Unix(),
		ExpiresAt:   now.Add(ttl).Unix(),
	}
	token, exp, err := SignEmbedToken(claims, h.JWTSecret)
	if err != nil {
		h.log(r.Context()).Error("embed-token: sign failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token signing failed"})
		return
	}

	// Audit fail-closed: refuse to issue the token if we cannot record
	// the mint. Per ADR-009 §7.2 a mutation that cannot be audited must
	// be rejected; token issuance is materially a privilege grant.
	if h.Audit != nil {
		if err := h.Audit.WriteEmbedTokenAudit(r.Context(), ws.ID, session.UserID); err != nil {
			h.log(r.Context()).Error("embed-token: audit failed", "err", err,
				"workspace_id", ws.ID, "actor_id", session.UserID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "audit failed"})
			return
		}
	}

	writeJSON(w, http.StatusOK, embedTokenResponse{Token: token, ExpiresAt: exp})
}

// PgAuditWriter writes report.embed_token.issued to core.audit_log.
type PgAuditWriter struct {
	Pool *pgxpool.Pool
}

func (p *PgAuditWriter) WriteEmbedTokenAudit(ctx context.Context, workspaceID, actorID uuid.UUID) error {
	if p == nil || p.Pool == nil {
		return errors.New("reports: pg audit writer not configured")
	}
	payload, err := json.Marshal(map[string]any{
		"workspace_id": workspaceID.String(),
		"actor_id":     actorID.String(),
	})
	if err != nil {
		return err
	}
	_, err = p.Pool.Exec(ctx, `
		INSERT INTO core.audit_log (event, workspace_id, actor_type, actor_user_id, payload)
		VALUES ('report.embed_token.issued', $1, 'human_api', $2, $3)
	`, workspaceID, actorID, payload)
	return err
}

func (h *Handler) log(ctx context.Context) *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
