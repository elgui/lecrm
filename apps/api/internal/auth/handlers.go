package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

// Handler bundles the dependencies for the /auth/* routes.
type Handler struct {
	Provider     *Provider
	Store        *Store
	SessionSecret []byte
	DomainTLD     string // e.g. "lecrm.fr"
	CookieSecure  bool
	Logger        *slog.Logger
}

// Register mounts the auth routes onto m at the conventional paths.
func (h *Handler) Register(m interface {
	Get(pattern string, handlerFn http.HandlerFunc)
	Post(pattern string, handlerFn http.HandlerFunc)
}) {
	m.Get("/auth/login", h.Login)
	m.Get("/auth/callback", h.Callback)
	m.Get("/auth/me", h.Me)
	m.Post("/auth/logout", h.Logout)
}

// Login starts an OIDC authorization-code flow. The workspace is
// derived from the Host header (the per-workspace subdomain), the state
// + PKCE verifier are signed into a short-lived workspace-scoped
// cookie, and the browser is redirected to the IdP's authz endpoint.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	subdomain, err := SubdomainFromHost(r.Host, h.DomainTLD)
	if err != nil {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}
	// Resolve the workspace early so we 404 unknown subdomains before
	// emitting any cookie or redirect.
	if _, err := h.Store.WorkspaceBySlug(r.Context(), subdomain); err != nil {
		if errors.Is(err, ErrWorkspaceNotFound) {
			http.Error(w, "unknown workspace", http.StatusNotFound)
			return
		}
		h.error(w, "workspace lookup", err)
		return
	}

	req, authURL, err := h.Provider.BuildAuthURL(subdomain)
	if err != nil {
		h.error(w, "build auth url", err)
		return
	}
	signed, err := EncodeAuthRequest(req, h.Provider.StateSecret)
	if err != nil {
		h.error(w, "encode auth request", err)
		return
	}
	c, err := BuildStateCookie(CookieScope{
		WorkspaceSubdomain: subdomain,
		DomainTLD:          h.DomainTLD,
		Secure:             h.CookieSecure,
	}, signed)
	if err != nil {
		h.error(w, "build state cookie", err)
		return
	}
	http.SetCookie(w, c)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback completes the OIDC flow: verify state, exchange code for
// tokens, upsert (issuer, subject) into core.users, ensure workspace
// membership, and set the workspace-scoped session cookie.
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	subdomain, err := SubdomainFromHost(r.Host, h.DomainTLD)
	if err != nil {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}

	stateCookie, err := r.Cookie(StateCookieName)
	if err != nil {
		http.Error(w, "missing state cookie", http.StatusBadRequest)
		return
	}
	req, err := DecodeAuthRequest(stateCookie.Value, h.Provider.StateSecret)
	if err != nil {
		http.Error(w, "invalid state cookie", http.StatusBadRequest)
		return
	}
	if req.WorkspaceSubdomain != subdomain {
		// The state cookie was issued for a different workspace —
		// reject and let the next ADR-007 §3 audit emission capture
		// the workspace_id_mismatch event (wired in Sprint 7).
		http.Error(w, "workspace mismatch", http.StatusBadRequest)
		return
	}
	query := r.URL.Query()
	if e := query.Get("error"); e != "" {
		http.Error(w, "oidc error: "+e, http.StatusBadRequest)
		return
	}
	if got := query.Get("state"); got != req.State {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		return
	}
	code := query.Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	claims, err := h.Provider.Exchange(r.Context(), code, req.CodeVerifier)
	if err != nil {
		h.error(w, "code exchange", err)
		return
	}

	workspaceID, err := h.Store.WorkspaceBySlug(r.Context(), subdomain)
	if err != nil {
		h.error(w, "workspace lookup", err)
		return
	}
	email, _ := claims.Claims["email"].(string)
	displayName, _ := claims.Claims["name"].(string)
	userID, err := h.Store.UpsertUser(r.Context(), claims.Issuer, claims.Subject, email, displayName)
	if err != nil {
		h.error(w, "upsert user", err)
		return
	}
	if err := h.Store.EnsureMember(r.Context(), workspaceID, userID); err != nil {
		h.error(w, "ensure member", err)
		return
	}

	sessionValue, err := EncodeSession(Session{UserID: userID, WorkspaceID: workspaceID}, h.SessionSecret)
	if err != nil {
		h.error(w, "encode session", err)
		return
	}
	sessionCookie, err := BuildSessionCookie(CookieScope{
		WorkspaceSubdomain: subdomain,
		DomainTLD:          h.DomainTLD,
		Secure:             h.CookieSecure,
	}, sessionValue)
	if err != nil {
		h.error(w, "build session cookie", err)
		return
	}
	http.SetCookie(w, sessionCookie)
	// Clear the now-spent state cookie.
	clearState, err := BuildStateCookie(CookieScope{
		WorkspaceSubdomain: subdomain,
		DomainTLD:          h.DomainTLD,
		Secure:             h.CookieSecure,
	}, "")
	if err == nil {
		clearState.MaxAge = -1
		http.SetCookie(w, clearState)
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// Me returns the logged-in user's identity and workspace, derived
// purely from the session cookie. No DB lookup beyond the cookie's HMAC
// verification.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	s, ok := SessionFromRequest(r, h.SessionSecret)
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	body := map[string]string{
		"user_id":      s.UserID.String(),
		"workspace_id": s.WorkspaceID.String(),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// Logout clears the session cookie with a matching Domain so the
// browser actually drops it.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	subdomain, err := SubdomainFromHost(r.Host, h.DomainTLD)
	if err != nil {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}
	c, err := ClearSessionCookie(CookieScope{
		WorkspaceSubdomain: subdomain,
		DomainTLD:          h.DomainTLD,
		Secure:             h.CookieSecure,
	})
	if err != nil {
		h.error(w, "clear cookie", err)
		return
	}
	http.SetCookie(w, c)
	w.WriteHeader(http.StatusNoContent)
}

// SessionFromRequest reads and verifies the session cookie. Returns
// (Session{}, false) on any failure — callers do not need to
// distinguish "no cookie" from "tampered cookie" from "expired".
func SessionFromRequest(r *http.Request, secret []byte) (Session, bool) {
	c, err := r.Cookie(SessionCookieName)
	if err != nil {
		return Session{}, false
	}
	s, err := DecodeSession(c.Value, secret)
	if err != nil {
		return Session{}, false
	}
	return s, true
}

func (h *Handler) error(w http.ResponseWriter, ctx string, err error) {
	if h.Logger != nil {
		h.Logger.Error("auth handler error", "ctx", ctx, "err", err)
	}
	http.Error(w, fmt.Sprintf("%s: %v", ctx, err), http.StatusInternalServerError)
}

// _ guards against unused-import drift while the Sprint 7 audit wiring
// is still pending; remove when context propagation lands.
var _ = context.Background
