package members

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	"github.com/gbconsult/lecrm/apps/api/internal/rbac"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

const maxBodySize int64 = 1 << 20

// SessionDecoder reads and verifies the request's session cookie scoped to
// the given workspace slug (production binding: auth.SessionFromRequestV2).
// The handler uses it to identify the acting user for self-action guards
// (an owner may not demote or remove themselves).
type SessionDecoder func(r *http.Request, slug string) (auth.Session, bool)

// Inviter sends (or, at v0, logs) a workspace invitation email. The
// production binding is a no-op placeholder; ADR-011 wires Brevo later.
type Inviter interface {
	SendInvite(ctx context.Context, workspaceID uuid.UUID, slug, email, role string) error
}

// Handler serves the member-management endpoints. The owner-only routes
// (RegisterRoutes) and the member+ self-service route (RegisterMeRoute)
// are mounted under separate RBAC groups by the HTTP server.
//
// The zero value is unusable: Store and DecodeSession MUST be set.
type Handler struct {
	Store         Store
	DecodeSession SessionDecoder
	Inviter       Inviter // optional; nil → invites logged only
	Logger        *slog.Logger
}

// RegisterRoutes mounts the owner-only member-management routes. The caller
// MUST wrap this in a group gated by rbac.RequireRole(rbac.RoleOwner).
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/workspace/members", h.listMembers)
	r.Post("/v1/workspace/members/invite", h.invite)
	r.Patch("/v1/workspace/members/{id}/role", h.updateRole)
	r.Delete("/v1/workspace/members/{id}", h.removeMember)
}

// RegisterMeRoute mounts GET /v1/workspace/me. The caller MUST wrap this in
// a group gated by rbac.RequireRole(rbac.RoleMember) so any authenticated
// member can read their own role and permissions.
func (h *Handler) RegisterMeRoute(r chi.Router) {
	r.Get("/v1/workspace/me", h.me)
}

type meResponse struct {
	UserID      string           `json:"user_id"`
	Role        string           `json:"role"`
	ActorType   string           `json:"actor_type"`
	Permissions rbac.Permissions `json:"permissions"`
}

// me returns the caller's role and capability bundle. The principal was
// resolved by rbac.Resolve; this never hits the database.
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	p, ok := rbac.PrincipalFromContext(r.Context())
	if !ok {
		// RequireRole(member) guards this route, so this is unreachable
		// in production — defend anyway.
		writeErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	resp := meResponse{
		Role:        p.Role.String(),
		ActorType:   p.ActorType,
		Permissions: rbac.PermissionsFor(p.Role),
	}
	if p.UserID != uuid.Nil {
		resp.UserID = p.UserID.String()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	list, err := h.Store.ListMembers(r.Context(), ws.ID)
	if err != nil {
		h.log(r.Context()).Error("list members", "err", err)
		writeErr(w, http.StatusInternalServerError, "list members failed")
		return
	}
	if list == nil {
		list = []Member{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": list})
}

type inviteReq struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	var body inviteReq
	if !decodeBody(w, r, &body) {
		return
	}
	email := strings.TrimSpace(body.Email)
	if email == "" || !strings.Contains(email, "@") {
		writeErr(w, http.StatusBadRequest, "valid email is required")
		return
	}
	// Default new invites to the least-privileged role.
	roleStr := strings.TrimSpace(body.Role)
	if roleStr == "" {
		roleStr = rbac.RoleMember.String()
	}
	role, valid := rbac.ParseRole(roleStr)
	if !valid {
		writeErr(w, http.StatusBadRequest, "role must be one of: member, admin, owner")
		return
	}

	member, err := h.Store.Invite(r.Context(), ws.ID, email, role)
	if err != nil {
		h.log(r.Context()).Error("invite member", "err", err, "email", email)
		writeErr(w, http.StatusInternalServerError, "invite failed")
		return
	}

	// Invite email is a v0 placeholder — log the intent (and call the
	// optional Inviter seam when wired) without blocking the response.
	if h.Inviter != nil {
		if err := h.Inviter.SendInvite(r.Context(), ws.ID, ws.Slug, email, role.String()); err != nil {
			h.log(r.Context()).Warn("invite email send failed (non-fatal)", "err", err, "email", email)
		}
	} else {
		h.log(r.Context()).Info("invite email placeholder", "workspace", ws.Slug, "email", email, "role", role.String())
	}

	writeJSON(w, http.StatusCreated, member)
}

type updateRoleReq struct {
	Role string `json:"role"`
}

func (h *Handler) updateRole(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	targetID, ok := h.parseTargetID(w, r)
	if !ok {
		return
	}
	actor, ok := h.actorUserID(w, r, ws)
	if !ok {
		return
	}
	if targetID == actor {
		writeErr(w, http.StatusBadRequest, "cannot change your own role")
		return
	}

	var body updateRoleReq
	if !decodeBody(w, r, &body) {
		return
	}
	role, valid := rbac.ParseRole(body.Role)
	if !valid {
		writeErr(w, http.StatusBadRequest, "role must be one of: member, admin, owner")
		return
	}

	switch err := h.Store.UpdateRole(r.Context(), ws.ID, targetID, role); {
	case errors.Is(err, ErrMemberNotFound):
		writeErr(w, http.StatusNotFound, "member not found")
	case err != nil:
		h.log(r.Context()).Error("update role", "err", err)
		writeErr(w, http.StatusInternalServerError, "update role failed")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"user_id": targetID.String(), "role": role.String()})
	}
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	targetID, ok := h.parseTargetID(w, r)
	if !ok {
		return
	}
	actor, ok := h.actorUserID(w, r, ws)
	if !ok {
		return
	}
	if targetID == actor {
		writeErr(w, http.StatusBadRequest, "cannot remove yourself")
		return
	}

	switch err := h.Store.RemoveMember(r.Context(), ws.ID, targetID); {
	case errors.Is(err, ErrMemberNotFound):
		writeErr(w, http.StatusNotFound, "member not found")
	case err != nil:
		h.log(r.Context()).Error("remove member", "err", err)
		writeErr(w, http.StatusInternalServerError, "remove member failed")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- helpers ---

func (h *Handler) ws(w http.ResponseWriter, r *http.Request) (*workspace.Context, bool) {
	ws, err := workspace.WorkspaceFromContext(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return nil, false
	}
	return ws, true
}

func (h *Handler) parseTargetID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid member id")
		return uuid.Nil, false
	}
	return id, true
}

// actorUserID resolves the acting user from the session cookie. Owner-only
// routes are gated by RBAC (the actor is necessarily an owner), but the
// self-action guards need the actor's own user_id. A service token has no
// user_id and therefore can never reach these routes (capped at admin).
func (h *Handler) actorUserID(w http.ResponseWriter, r *http.Request, ws *workspace.Context) (uuid.UUID, bool) {
	sess, ok := h.DecodeSession(r, ws.Slug)
	if !ok || sess.UserID == uuid.Nil {
		writeErr(w, http.StatusUnauthorized, "authentication required")
		return uuid.Nil, false
	}
	return sess.UserID, true
}

func (h *Handler) log(ctx context.Context) *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}
