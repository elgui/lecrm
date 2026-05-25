package metadata

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

const maxBodySize = 1 << 20 // 1 MB

// Handler serves custom property definition and property CRUD endpoints.
type Handler struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger
	cache  *defCache
}

func (h *Handler) ensureCache() *defCache {
	if h.cache == nil {
		h.cache = newDefCache()
	}
	return h.cache
}

func (h *Handler) storeFromCtx(r *http.Request) (*Store, error) {
	ws, err := workspace.WorkspaceFromContext(r.Context())
	if err != nil {
		return nil, err
	}
	return NewWithCache(h.Pool, ws.RoleName, ws.ID, h.ensureCache()), nil
}

// RegisterRoutes mounts all metadata routes on r.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/metadata/definitions", h.ListDefinitions)
	r.Post("/v1/metadata/definitions", h.CreateDefinition)
	r.Delete("/v1/metadata/definitions/{id}", h.DeleteDefinition)

	r.Get("/v1/contacts/{id}/properties", h.GetProperties("contact"))
	r.Put("/v1/contacts/{id}/properties", h.SetProperties("contact"))
	r.Get("/v1/deals/{id}/properties", h.GetProperties("deal"))
	r.Put("/v1/deals/{id}/properties", h.SetProperties("deal"))
}

// ListDefinitions handles GET /v1/metadata/definitions?parent_type=contact
func (h *Handler) ListDefinitions(w http.ResponseWriter, r *http.Request) {
	store, err := h.storeFromCtx(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}

	parentType := r.URL.Query().Get("parent_type")
	if parentType == "" {
		writeErr(w, http.StatusBadRequest, "parent_type query parameter required")
		return
	}
	if !validParentTypes[parentType] {
		writeErr(w, http.StatusBadRequest, "parent_type must be 'contact' or 'deal'")
		return
	}

	defs, err := store.ListDefinitions(r.Context(), parentType)
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "list definitions failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "list definitions failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"definitions": defs})
}

// CreateDefinition handles POST /v1/metadata/definitions
func (h *Handler) CreateDefinition(w http.ResponseWriter, r *http.Request) {
	store, err := h.storeFromCtx(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	var input CreateDefinitionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	def, err := store.CreateDefinition(r.Context(), input)
	if err != nil {
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required") {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			writeErr(w, http.StatusConflict, "definition already exists for this parent_type and property_key")
			return
		}
		h.Logger.ErrorContext(r.Context(), "create definition failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "create definition failed")
		return
	}

	writeJSON(w, http.StatusCreated, def)
}

// DeleteDefinition handles DELETE /v1/metadata/definitions/:id
func (h *Handler) DeleteDefinition(w http.ResponseWriter, r *http.Request) {
	store, err := h.storeFromCtx(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "workspace context missing")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid definition id")
		return
	}

	if err := store.DeleteDefinition(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "no rows") {
			writeErr(w, http.StatusNotFound, "definition not found")
			return
		}
		h.Logger.ErrorContext(r.Context(), "delete definition failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "delete definition failed")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetProperties returns a handler for GET /v1/{entity}/{id}/properties
func (h *Handler) GetProperties(parentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store, err := h.storeFromCtx(r)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "workspace context missing")
			return
		}

		idStr := chi.URLParam(r, "id")
		parentID, err := uuid.Parse(idStr)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid id")
			return
		}

		props, err := store.Get(r.Context(), parentType, parentID)
		if err != nil {
			h.Logger.ErrorContext(r.Context(), "get properties failed", "err", err, "parent_type", parentType)
			writeErr(w, http.StatusInternalServerError, "get properties failed")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"properties": props})
	}
}

// SetProperties returns a handler for PUT /v1/{entity}/{id}/properties
func (h *Handler) SetProperties(parentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store, err := h.storeFromCtx(r)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "workspace context missing")
			return
		}

		idStr := chi.URLParam(r, "id")
		parentID, err := uuid.Parse(idStr)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid id")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if err := store.Set(r.Context(), parentType, parentID, body); err != nil {
			var ve *ValidationError
			if errors.As(err, &ve) {
				writeErr(w, http.StatusBadRequest, ve.Error())
				return
			}
			h.Logger.ErrorContext(r.Context(), "set properties failed", "err", err, "parent_type", parentType)
			writeErr(w, http.StatusInternalServerError, "set properties failed")
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		_ = err
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
