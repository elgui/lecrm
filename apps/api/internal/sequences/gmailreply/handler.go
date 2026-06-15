package gmailreply

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// PushRoute is the path Pub/Sub delivers Gmail watch notifications to (ADR-004
// rev 2 §4; the subscription's push endpoint in the setup runbook). Centralised
// so the route registration and any test cannot drift from the deployed value.
const PushRoute = "/v1/webhooks/gmail/push"

// maxPushBodyBytes caps the Pub/Sub push body. Gmail notifications are tiny
// ({emailAddress, historyId}); 64 KiB is generous and bounds a hostile sender.
const maxPushBodyBytes = 64 * 1024

// PushHandler receives Pub/Sub push deliveries of Gmail watch notifications,
// authenticates each by its Google-signed OIDC JWT, resolves the mailbox to a
// workspace+user, and enqueues a mailbox-scan job (ADR-004 rev 2 §4 step 1).
//
// It deliberately does NO Gmail I/O: a push must be acked fast, and history.list
// belongs to the worker. Every decision returns quickly with the right status
// so Pub/Sub redelivers only what is actually worth retrying.
type PushHandler struct {
	// Validator verifies the OIDC JWT (signature/aud/exp/issuer/email).
	Validator TokenValidator
	// Audience is the expected `aud` claim — the push endpoint URL the
	// subscription was created with. Empty disables the audience check (NOT
	// recommended; the setup runbook always sets it).
	Audience string
	// ServiceAccount, when non-empty, is the expected verified `email` claim —
	// the push-auth service account from the subscription. It pins the caller to
	// the one SA Pub/Sub mints tokens as, so a valid Google token for any other
	// identity is rejected.
	ServiceAccount string
	// Resolver maps the notification's emailAddress to a workspace+user.
	Resolver ConnectionResolver
	// Enqueuer inserts the poll_mailbox job into the resolved workspace's queue.
	Enqueuer MailboxPollEnqueuer
	// Logger is used for structured rejection/ack logging. Required.
	Logger *slog.Logger
}

// RegisterRoute mounts the push endpoint. It is mounted OUTSIDE the
// workspace-middleware group (like the Brevo inbound webhook): authentication
// is the Google-signed JWT over the request, not a session cookie.
func (h *PushHandler) RegisterRoute(r chi.Router) {
	r.Post(PushRoute, h.ServeHTTP)
}

// ServeHTTP handles one Pub/Sub push delivery.
//
//	401 — missing/invalid OIDC token (the "rejects without a valid Google-signed
//	      JWT" requirement). Pub/Sub retries; an operator misconfig is loud.
//	400 — token valid but body malformed. Acked-as-rejected (Pub/Sub would just
//	      re-fail), but distinct from 401 for observability.
//	204 — accepted (job enqueued) OR notification for an unknown mailbox (acked
//	      so Pub/Sub stops redelivering something we can never act on).
//	500 — transient enqueue failure; Pub/Sub retries.
func (h *PushHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	token, ok := bearerToken(r)
	if !ok {
		h.reject(w, r, http.StatusUnauthorized, "missing bearer token", nil)
		return
	}

	email, err := h.Validator.Validate(ctx, token, h.Audience)
	if err != nil {
		h.reject(w, r, http.StatusUnauthorized, "token validation failed", err)
		return
	}
	if h.ServiceAccount != "" && email != h.ServiceAccount {
		h.reject(w, r, http.StatusUnauthorized, "unexpected push service account", nil)
		return
	}

	body, err := io.ReadAll(io.LimitReader(http.MaxBytesReader(w, r.Body, maxPushBodyBytes), maxPushBodyBytes+1))
	if err != nil {
		h.reject(w, r, http.StatusBadRequest, "read body", err)
		return
	}
	if len(body) > maxPushBodyBytes {
		h.reject(w, r, http.StatusRequestEntityTooLarge, "push body too large", nil)
		return
	}

	notif, err := ParsePushBody(body)
	if err != nil {
		h.reject(w, r, http.StatusBadRequest, "malformed push body", err)
		return
	}

	target, err := h.Resolver.ResolveByEmail(ctx, notif.EmailAddress)
	if err != nil {
		if errors.Is(err, ErrNoConnection) {
			// Not ours — ack so Pub/Sub stops redelivering. Logged at info; a
			// burst would indicate the subscription is fanning out too widely.
			h.Logger.InfoContext(ctx, "gmail push: no connection for mailbox; acking",
				"email", notif.EmailAddress)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.reject(w, r, http.StatusInternalServerError, "resolve mailbox", err)
		return
	}

	if err := h.Enqueuer.EnqueuePollMailbox(ctx, PollMailboxArgs{
		WorkspaceID:  target.WorkspaceID,
		UserID:       target.UserID,
		EmailAddress: target.EmailAddress,
		HistoryID:    notif.HistoryID,
	}); err != nil {
		h.reject(w, r, http.StatusInternalServerError, "enqueue poll_mailbox", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// bearerToken extracts the JWT from the `Authorization: Bearer <jwt>` header
// Pub/Sub sets when the subscription has an OIDC push-auth service account.
func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	tok := strings.TrimSpace(h[len(prefix):])
	if tok == "" {
		return "", false
	}
	return tok, true
}

// reject logs the rejection with its cause and writes a small JSON error. It
// never echoes the cause to the client (it may carry token internals); the
// client gets a generic message and the detail goes to the structured log.
func (h *PushHandler) reject(w http.ResponseWriter, r *http.Request, status int, msg string, cause error) {
	attrs := []any{"status", status, "reason", msg}
	if cause != nil {
		attrs = append(attrs, "err", cause.Error())
	}
	// 5xx is a real server problem; 4xx is an expected reject (bad caller).
	if status >= 500 {
		h.Logger.ErrorContext(r.Context(), "gmail push rejected", attrs...)
	} else {
		h.Logger.WarnContext(r.Context(), "gmail push rejected", attrs...)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
