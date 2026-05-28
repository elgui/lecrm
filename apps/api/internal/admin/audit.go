// Package admin holds the Léo-facing administrative HTTP endpoints
// that sit outside the per-workspace subdomain routing: the integrator
// queries cross-tenant by passing ?tenant=X explicitly. Auth is a
// shared bearer token (LECRM_ADMIN_TOKEN) compared in constant time —
// this is the v0 minimum per the Phase 3 tasket. v1+ moves to OIDC
// admin claims.
package admin

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaxLimit caps a single page; mirrors apps/admin/internal/audit.
const MaxLimit = 500

// DefaultLimit applies when caller omits limit.
const DefaultLimit = 100

// AuditHandler serves GET /admin/audit. The token is empty-valued by
// default; an empty token disables the route (the handler 503s) so a
// misconfigured deploy cannot accidentally expose the audit surface
// unauthenticated.
type AuditHandler struct {
	Pool   *pgxpool.Pool
	Token  string
	Logger *slog.Logger
}

// AuditEntry is the JSON shape returned for one audit row.
type AuditEntry struct {
	ID          int64          `json:"id"`
	OccurredAt  time.Time      `json:"occurred_at"`
	Event       string         `json:"event"`
	WorkspaceID *uuid.UUID     `json:"workspace_id,omitempty"`
	Slug        string         `json:"slug,omitempty"`
	ActorType   string         `json:"actor_type,omitempty"`
	ActorUserID *uuid.UUID     `json:"actor_user_id,omitempty"`
	RequestID   *uuid.UUID     `json:"request_id,omitempty"`
	Payload     map[string]any `json:"payload"`
}

// Register mounts /admin/audit on the given router. The route lives
// OUTSIDE workspace.Middleware: Léo queries cross-tenant via ?tenant=,
// not via subdomain.
func (h *AuditHandler) Register(r chi.Router) {
	r.Get("/admin/audit", h.handleQuery)
}

func (h *AuditHandler) handleQuery(w http.ResponseWriter, r *http.Request) {
	if h.Token == "" {
		writeErr(w, http.StatusServiceUnavailable, "admin audit endpoint disabled (LECRM_ADMIN_TOKEN unset)")
		return
	}
	if !h.checkAuth(r) {
		writeErr(w, http.StatusUnauthorized, "invalid admin token")
		return
	}

	q := r.URL.Query()
	slug := strings.TrimSpace(q.Get("tenant"))
	if slug == "" {
		writeErr(w, http.StatusBadRequest, "tenant query param required")
		return
	}

	var since, until time.Time
	if v := strings.TrimSpace(q.Get("since")); v != "" {
		t, err := parseTime(v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "since: "+err.Error())
			return
		}
		since = t
	}
	if v := strings.TrimSpace(q.Get("until")); v != "" {
		t, err := parseTime(v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "until: "+err.Error())
			return
		}
		until = t
	}

	limit := DefaultLimit
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeErr(w, http.StatusBadRequest, "limit must be a non-negative integer")
			return
		}
		if n > 0 {
			limit = n
		}
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	entries, err := queryAudit(r.Context(), h.Pool, slug, since, until,
		strings.TrimSpace(q.Get("event")),
		strings.TrimSpace(q.Get("actor")),
		limit)
	if errors.Is(err, errUnknownSlug) {
		writeErr(w, http.StatusNotFound, "tenant not found")
		return
	}
	if err != nil {
		if h.Logger != nil {
			h.Logger.ErrorContext(r.Context(), "audit query failed", "err", err, "slug", slug)
		}
		writeErr(w, http.StatusInternalServerError, "audit query failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"tenant":  slug,
		"count":   len(entries),
		"entries": entries,
	})
}

var errUnknownSlug = errors.New("admin: unknown tenant slug")

func queryAudit(ctx context.Context, pool *pgxpool.Pool, slug string, since, until time.Time, event, actorType string, limit int) ([]AuditEntry, error) {
	var workspaceID uuid.UUID
	err := pool.QueryRow(ctx,
		`SELECT id FROM core.workspaces WHERE slug = $1`,
		slug).Scan(&workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errUnknownSlug
	}
	if err != nil {
		return nil, fmt.Errorf("resolve slug: %w", err)
	}

	sql := `SELECT id, occurred_at, event, workspace_id, actor_type,
	               actor_user_id, request_id, payload
	          FROM core.audit_log
	         WHERE workspace_id = $1`
	args := []any{workspaceID}
	idx := 2
	if !since.IsZero() {
		sql += fmt.Sprintf(" AND occurred_at >= $%d", idx)
		args = append(args, since)
		idx++
	}
	if !until.IsZero() {
		sql += fmt.Sprintf(" AND occurred_at < $%d", idx)
		args = append(args, until)
		idx++
	}
	if event != "" {
		sql += fmt.Sprintf(" AND event = $%d", idx)
		args = append(args, event)
		idx++
	}
	if actorType != "" {
		sql += fmt.Sprintf(" AND actor_type = $%d", idx)
		args = append(args, actorType)
		idx++
	}
	sql += fmt.Sprintf(" ORDER BY occurred_at DESC, id DESC LIMIT $%d", idx)
	args = append(args, limit)

	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	out := make([]AuditEntry, 0, limit)
	for rows.Next() {
		var e AuditEntry
		var rawPayload []byte
		var wsID, actorUserID, requestID *uuid.UUID
		var actor *string
		if err := rows.Scan(&e.ID, &e.OccurredAt, &e.Event, &wsID, &actor, &actorUserID, &requestID, &rawPayload); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		e.WorkspaceID = wsID
		if wsID != nil {
			e.Slug = slug
		}
		if actor != nil {
			e.ActorType = *actor
		}
		e.ActorUserID = actorUserID
		e.RequestID = requestID
		if len(rawPayload) > 0 {
			if err := json.Unmarshal(rawPayload, &e.Payload); err != nil {
				return nil, fmt.Errorf("decode payload (id=%d): %w", e.ID, err)
			}
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}
	return out, nil
}

func (h *AuditHandler) checkAuth(r *http.Request) bool {
	got := bearerToken(r.Header.Get("Authorization"))
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.Token)) == 1
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func parseTime(v string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	if strings.HasSuffix(v, "d") {
		days := strings.TrimSuffix(v, "d")
		if n, err := strconv.Atoi(days); err == nil {
			return time.Now().UTC().Add(-time.Duration(n) * 24 * time.Hour), nil
		}
	}
	if d, err := time.ParseDuration(v); err == nil {
		return time.Now().UTC().Add(-d), nil
	}
	return time.Time{}, errors.New("expected RFC3339 timestamp or duration like 24h/7d")
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
