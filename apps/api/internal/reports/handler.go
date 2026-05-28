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
	JWTSecret      []byte
	TTL            time.Duration
	DecodeSession  SessionDecoder
	Audit          AuditWriter
	Logger         *slog.Logger
}

// RegisterRoutes mounts the handler. Caller must wrap this in a router
// group that has the workspace middleware attached (the workspace
// context is required).
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/reports/embed-token", h.handleEmbedToken)
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
