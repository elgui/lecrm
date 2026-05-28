package crm

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// idempotencyTTL is the lifetime of a cached response for one
// (workspace_id, Idempotency-Key) pair. ADR-009 §4 mandates 24h.
const idempotencyTTL = 24 * time.Hour

// maxIdempotencyKeyLen caps the header to a reasonable opaque token
// length so a buggy or hostile client cannot push huge values into
// core.idempotency_keys. Stripe uses 255; we match.
const maxIdempotencyKeyLen = 255

// idempotencyLookup checks for a cached response for (workspace_id, key).
// Returns hit=true with the cached status + body when a non-expired row
// exists. The lookup runs outside the mutation transaction so a cache
// hit can short-circuit before opening a write tx.
func idempotencyLookup(ctx context.Context, pool *pgxpool.Pool, ws uuid.UUID, key string) (int, []byte, bool, error) {
	var status int
	var body []byte
	err := pool.QueryRow(ctx,
		`SELECT response_status, response_body
		   FROM core.idempotency_keys
		  WHERE workspace_id = $1 AND key = $2 AND expires_at > now()`,
		ws, key,
	).Scan(&status, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, false, nil
	}
	if err != nil {
		return 0, nil, false, err
	}
	return status, body, true, nil
}

// idempotencyStore persists a captured response inside the caller's
// transaction. The same writeTx that holds the entity mutation +
// audit-log insert holds the idempotency-key insert — atomic by
// construction. ON CONFLICT DO NOTHING handles a racing duplicate:
// the first committer wins, the second sees the cached row on the
// subsequent lookup.
func idempotencyStore(ctx context.Context, tx pgx.Tx, ws uuid.UUID, key, method, path string, status int, body []byte) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO core.idempotency_keys
		   (key, workspace_id, method, path, response_status, response_body, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now() + $7::interval)
		 ON CONFLICT (workspace_id, key) DO NOTHING`,
		key, ws, method, path, status, body, idempotencyTTL.String(),
	)
	return err
}

// readIdempotencyKey extracts and validates the `Idempotency-Key` header.
// Returns ("", true) when the header is absent (no idempotency requested).
// Writes 400 + returns ("", false) when the header is too long.
func readIdempotencyKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) > maxIdempotencyKeyLen {
		writeErr(w, http.StatusBadRequest, "Idempotency-Key too long")
		return "", false
	}
	return key, true
}

// replayIdempotent looks up a cached response for (workspaceID, key).
// The 4-value return mirrors the (status, body, hit, ok) shape callers
// need: ok=false means the lookup itself failed and a 500 has been
// written to w; ok=true + hit=true means the caller should writeReplay
// and return; ok=true + hit=false means proceed with the mutation.
func (h *Handler) replayIdempotent(w http.ResponseWriter, r *http.Request, workspaceID uuid.UUID, key string) (int, []byte, bool, bool) {
	st, body, hit, err := idempotencyLookup(r.Context(), h.Pool, workspaceID, key)
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "idempotency lookup", "err", err)
		writeErr(w, http.StatusInternalServerError, "idempotency lookup failed")
		return 0, nil, false, false
	}
	return st, body, hit, true
}

// writeRaw emits a pre-marshalled JSON body. Used by handlers that
// build the response inside a writeTx (so the idempotency cache stores
// the exact bytes that will be sent back) and then need to flush after
// commit. Mirrors writeJSON's headers without the json.Encoder round-trip.
func writeRaw(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeReplay emits a cached idempotent response to the client and
// flags it with the `Idempotency-Replayed: true` header so callers can
// distinguish replays from fresh executions (useful for tests + for
// clients deduplicating user-visible "created" toasts).
func writeReplay(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Idempotency-Replayed", "true")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
