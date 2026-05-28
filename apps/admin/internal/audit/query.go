// Package audit exposes a Léo-facing query view over core.audit_log so
// the integrator can self-serve "did automation A fire on tenant X
// yesterday?" without DB shell access. The same package is consumed
// by the lecrm-admin CLI (audit query) and the lecrm-api admin REST
// endpoint (/admin/audit), so the SQL surface lives in one place.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// MaxLimit caps a single page. Léo's debugging flow rarely scans more
// than a few hundred rows; the cap exists so a missing limit cannot
// scan an entire workspace history.
const MaxLimit = 500

// DefaultLimit applies when the caller omits limit.
const DefaultLimit = 100

// DBTX matches both *pgx.Conn and *pgxpool.Pool, which is the seam
// that lets the CLI (single conn) and the HTTP handler (pool) share
// this implementation.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Filter narrows the query. All fields are optional except Slug.
type Filter struct {
	Slug      string
	Since     time.Time
	Until     time.Time
	Event     string
	ActorType string
	Limit     int
}

// Entry is one row of core.audit_log shaped for human / JSON
// consumption (payload exposed as decoded map, not raw bytes).
type Entry struct {
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

// ErrUnknownSlug is returned by Query when the slug does not resolve.
var ErrUnknownSlug = errors.New("audit: unknown tenant slug")

// Query runs the filter and returns at most Limit entries newest-first.
func Query(ctx context.Context, db DBTX, f Filter) ([]Entry, error) {
	if f.Slug == "" {
		return nil, errors.New("audit: slug is required")
	}

	limit := f.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	var workspaceID uuid.UUID
	err := db.QueryRow(ctx,
		`SELECT id FROM core.workspaces WHERE slug = $1`,
		f.Slug).Scan(&workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnknownSlug
	}
	if err != nil {
		return nil, fmt.Errorf("audit: resolve slug: %w", err)
	}

	sql := `SELECT id, occurred_at, event, workspace_id, actor_type,
	               actor_user_id, request_id, payload
	          FROM core.audit_log
	         WHERE workspace_id = $1`
	args := []any{workspaceID}
	idx := 2

	if !f.Since.IsZero() {
		sql += fmt.Sprintf(" AND occurred_at >= $%d", idx)
		args = append(args, f.Since)
		idx++
	}
	if !f.Until.IsZero() {
		sql += fmt.Sprintf(" AND occurred_at < $%d", idx)
		args = append(args, f.Until)
		idx++
	}
	if f.Event != "" {
		sql += fmt.Sprintf(" AND event = $%d", idx)
		args = append(args, f.Event)
		idx++
	}
	if f.ActorType != "" {
		sql += fmt.Sprintf(" AND actor_type = $%d", idx)
		args = append(args, f.ActorType)
		idx++
	}
	sql += fmt.Sprintf(" ORDER BY occurred_at DESC, id DESC LIMIT $%d", idx)
	args = append(args, limit)

	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("audit: query: %w", err)
	}
	defer rows.Close()

	out := make([]Entry, 0, limit)
	for rows.Next() {
		var e Entry
		var rawPayload []byte
		var wsID, actorUserID, requestID *uuid.UUID
		var actorType *string
		if err := rows.Scan(&e.ID, &e.OccurredAt, &e.Event, &wsID, &actorType, &actorUserID, &requestID, &rawPayload); err != nil {
			return nil, fmt.Errorf("audit: scan: %w", err)
		}
		e.WorkspaceID = wsID
		if wsID != nil {
			e.Slug = f.Slug
		}
		if actorType != nil {
			e.ActorType = *actorType
		}
		e.ActorUserID = actorUserID
		e.RequestID = requestID
		if len(rawPayload) > 0 {
			if err := json.Unmarshal(rawPayload, &e.Payload); err != nil {
				return nil, fmt.Errorf("audit: decode payload (id=%d): %w", e.ID, err)
			}
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit: iterate: %w", err)
	}
	return out, nil
}
