package auth

// HTTP handlers for /v1/workspace/tokens (Sprint 7 — ADR-009 §4.1).
//
// Routes:
//
//	GET    /v1/workspace/tokens      → list tokens (no hashes)
//	POST   /v1/workspace/tokens      → create token (plaintext returned ONCE)
//	DELETE /v1/workspace/tokens/{id} → revoke token
//
// All three routes live behind workspace.Middleware and operate on
// the resolved workspace from context.

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

// ServiceTokenHandler bundles the dependencies for the
// /v1/workspace/tokens routes.
type ServiceTokenHandler struct {
	Store  *PgServiceTokenStore
	Logger *slog.Logger
}

// RegisterRoutes mounts the handler on r.
func (h *ServiceTokenHandler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/workspace/tokens", h.List)
	r.Post("/v1/workspace/tokens", h.Create)
	r.Delete("/v1/workspace/tokens/{id}", h.Delete)
}

func (h *ServiceTokenHandler) List(w http.ResponseWriter, r *http.Request) {
	ws, err := workspace.WorkspaceFromContext(r.Context())
	if err != nil {
		writeTokenJSONError(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	tokens, err := h.Store.List(r.Context(), ws.ID)
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "list service tokens failed", "err", err)
		writeTokenJSONError(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeTokenJSON(w, http.StatusOK, map[string]any{"data": tokens})
}

type createTokenReq struct {
	Name      string     `json:"name"`
	ActorType string     `json:"actor_type"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type createTokenResp struct {
	Token        string              `json:"token"`
	ServiceToken serviceTokenSummary `json:"service_token"`
}

type serviceTokenSummary struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	ActorType  string     `json:"actor_type"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (h *ServiceTokenHandler) Create(w http.ResponseWriter, r *http.Request) {
	ws, err := workspace.WorkspaceFromContext(r.Context())
	if err != nil {
		writeTokenJSONError(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	var req createTokenReq
	r.Body = http.MaxBytesReader(w, r.Body, 1<<14)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeTokenJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == "" || req.ActorType == "" {
		writeTokenJSONError(w, http.StatusBadRequest, "name and actor_type required")
		return
	}
	if err := ValidateActorType(req.ActorType); err != nil {
		writeTokenJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := h.Store.Create(r.Context(), ws.ID, ws.Slug, CreateServiceTokenInput(req))
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "create service token failed", "err", err)
		writeTokenJSONError(w, http.StatusInternalServerError, "create failed")
		return
	}

	writeTokenJSON(w, http.StatusCreated, createTokenResp{
		Token: created.Plaintext,
		ServiceToken: serviceTokenSummary{
			ID:         created.ID,
			Name:       created.Name,
			ActorType:  created.ActorType,
			Scopes:     created.Scopes,
			ExpiresAt:  created.ExpiresAt,
			LastUsedAt: created.LastUsedAt,
			CreatedAt:  created.CreatedAt,
		},
	})
}

func (h *ServiceTokenHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ws, err := workspace.WorkspaceFromContext(r.Context())
	if err != nil {
		writeTokenJSONError(w, http.StatusInternalServerError, "workspace context missing")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeTokenJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.Store.Delete(r.Context(), ws.ID, id); err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			writeTokenJSONError(w, http.StatusNotFound, "token not found")
			return
		}
		h.Logger.ErrorContext(r.Context(), "delete service token failed", "err", err)
		writeTokenJSONError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeTokenJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeTokenJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
