package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Handler bundles the dependencies for the /auth/* routes.
type Handler struct {
	Provider      *Provider
	Store         *Store
	SessionSecret []byte
	DomainTLD     string // e.g. "lecrm.fr"
	CookieSecure  bool   // true in prod, false in plain-http dev
	CallbackPath  string // e.g. "/auth/callback"
	Logger        *slog.Logger
	Revocations   RevocationChecker // nil = no revocation checking
}

// redirectURIFor returns the workspace-specific OAuth callback URL the
// IdP must redirect back to. Scheme is https when CookieSecure (which
// tracks "are we behind TLS") is true, http otherwise — the http case
// is dev-only.
func (h *Handler) redirectURIFor(subdomain string, hostFromRequest string) string {
	// Preserve port from inbound Host (e.g. dev "acme.lecrm.test:8080").
	host := subdomain + "." + h.DomainTLD
	if idx := indexByte(hostFromRequest, ':'); idx >= 0 && !h.CookieSecure {
		host += hostFromRequest[idx:]
	}
	scheme := "https"
	if !h.CookieSecure {
		scheme = "http"
	}
	return scheme + "://" + host + h.CallbackPath
}

// indexByte is the inverse of strings.IndexByte without importing strings here.
func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// Register mounts the auth routes onto m at the conventional paths.
func (h *Handler) Register(m interface {
	Get(pattern string, handlerFn http.HandlerFunc)
	Post(pattern string, handlerFn http.HandlerFunc)
}) {
	m.Get("/auth/login", h.Login)
	m.Get("/auth/callback", h.Callback)
	m.Get("/auth/me", h.Me)
	m.Get("/auth/workspaces", h.Workspaces)
	m.Post("/auth/logout", h.Logout)
	m.Post("/auth/revoke", h.Revoke)
	m.Post("/auth/revoke-all", h.RevokeAll)
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

	redirectURI := h.redirectURIFor(subdomain, r.Host)
	req, authURL, err := h.Provider.BuildAuthURL(subdomain, redirectURI)
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

	claims, err := h.Provider.Exchange(r.Context(), code, req.CodeVerifier, req.RedirectURI)
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
	// Login-time integrator elevation: if a pending grant exists for this
	// workspace + email, materialize the membership as 'integrator' instead
	// of the default 'member'. A grant-lookup failure must NOT lock the user
	// out — fail open to the least-privileged role (never over-elevate, and
	// EnsureMemberWithRole never downgrades an existing higher role anyway).
	role := "member"
	if granted, gErr := h.Store.IntegratorGrantExists(r.Context(), workspaceID, email); gErr != nil {
		if h.Logger != nil {
			h.Logger.Warn("integrator grant check failed; defaulting to member",
				"err", gErr, "workspace_id", workspaceID, "user_id", userID)
		}
	} else if granted {
		role = "integrator"
	}
	if err := h.Store.EnsureMemberWithRole(r.Context(), workspaceID, userID, role); err != nil {
		h.error(w, "ensure member", err)
		return
	}

	sessionValue, err := EncodeSessionV2(Session{UserID: userID, WorkspaceID: workspaceID}, subdomain, h.SessionSecret)
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
	subdomain, err := SubdomainFromHost(r.Host, h.DomainTLD)
	if err != nil {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}
	s, needsUpgrade, ok := SessionFromRequestV2(r, subdomain, h.SessionSecret)
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	if h.isRevoked(r.Context(), s) {
		http.Error(w, "session revoked", http.StatusUnauthorized)
		return
	}
	if needsUpgrade {
		if upgraded, encErr := EncodeSessionV2(s, subdomain, h.SessionSecret); encErr == nil {
			if cookie, buildErr := BuildSessionCookie(CookieScope{
				WorkspaceSubdomain: subdomain,
				DomainTLD:          h.DomainTLD,
				Secure:             h.CookieSecure,
			}, upgraded); buildErr == nil {
				http.SetCookie(w, cookie)
			}
		}
		if h.Logger != nil {
			h.Logger.Info("session upgraded from V1 to V2",
				"user_id", s.UserID,
				"workspace_slug", subdomain)
		}
	}
	// Enrich with identity for the UI (avatar initial, footer name/email,
	// workspace label). Best-effort: a lookup error (or absent Store) must
	// not fail /auth/me — the session is already authenticated.
	var email, displayName string
	if h.Store != nil {
		var profErr error
		email, displayName, profErr = h.Store.GetUserProfile(r.Context(), s.UserID)
		if profErr != nil && h.Logger != nil {
			h.Logger.Warn("auth/me: profile lookup failed", "user_id", s.UserID, "err", profErr)
		}
	}
	body := map[string]string{
		"user_id":        s.UserID.String(),
		"workspace_id":   s.WorkspaceID.String(),
		"workspace_slug": subdomain,
		"email":          email,
		"name":           displayName,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// workspaceEntry is one switch-able workspace returned by GET /auth/workspaces.
type workspaceEntry struct {
	Slug string `json:"slug"`
	Role string `json:"role"`
	URL  string `json:"url"`
}

// Workspaces lists the workspaces the authenticated caller can switch into:
// the UNION of their memberships and their pending integrator grants
// (auth.Store.ListAccessibleWorkspaces). It is the data source for the
// frontend workspace switcher, including freshly-provisioned tenants the
// integrator has never logged into.
//
// SECURITY (ADR-009 §5.2): this is session-scoped and returns ONLY the
// caller's own access — it reads core via the auth Store pool, never a
// workspace role connection, and there is no slug-enumeration surface. It
// does NOT mint cross-workspace sessions; each returned url is a full
// navigation target that obtains its own per-subdomain cookie after SSO.
func (h *Handler) Workspaces(w http.ResponseWriter, r *http.Request) {
	subdomain, err := SubdomainFromHost(r.Host, h.DomainTLD)
	if err != nil {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}
	s, _, ok := SessionFromRequestV2(r, subdomain, h.SessionSecret)
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	if h.isRevoked(r.Context(), s) {
		http.Error(w, "session revoked", http.StatusUnauthorized)
		return
	}
	if h.Store == nil {
		http.Error(w, "store unavailable", http.StatusInternalServerError)
		return
	}
	// The grant union is keyed on the user's email; look it up best-effort.
	email, _, profErr := h.Store.GetUserProfile(r.Context(), s.UserID)
	if profErr != nil && h.Logger != nil {
		h.Logger.Warn("auth/workspaces: profile lookup failed", "user_id", s.UserID, "err", profErr)
	}
	accessible, err := h.Store.ListAccessibleWorkspaces(r.Context(), s.UserID, email)
	if err != nil {
		h.error(w, "list accessible workspaces", err)
		return
	}
	out := make([]workspaceEntry, 0, len(accessible))
	for _, a := range accessible {
		out = append(out, workspaceEntry{
			Slug: a.Slug,
			Role: a.Role,
			URL:  h.workspaceURL(a.Slug, r.Host),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": out})
}

// workspaceURL builds the absolute switch URL for a workspace slug from the
// configured DomainTLD. Scheme follows CookieSecure (https in prod, http in
// dev); the inbound Host's port is preserved in dev so links work behind the
// dev proxy (e.g. acme.lecrm.test:8080).
func (h *Handler) workspaceURL(slug, hostFromRequest string) string {
	host := slug + "." + h.DomainTLD
	scheme := "https"
	if !h.CookieSecure {
		scheme = "http"
		if idx := indexByte(hostFromRequest, ':'); idx >= 0 {
			host += hostFromRequest[idx:]
		}
	}
	return scheme + "://" + host + "/"
}

// Logout clears the session cookie and revokes the current session's JTI
// so replayed copies of the cookie are rejected.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	subdomain, err := SubdomainFromHost(r.Host, h.DomainTLD)
	if err != nil {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}
	s, _, ok := SessionFromRequestV2(r, subdomain, h.SessionSecret)
	if ok && s.JTI != (uuid.UUID{}) {
		h.revokeJTI(r.Context(), s)
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

// Revoke invalidates a specific session by JTI. The caller must be
// authenticated and can only revoke their own sessions.
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	subdomain, err := SubdomainFromHost(r.Host, h.DomainTLD)
	if err != nil {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}
	s, _, ok := SessionFromRequestV2(r, subdomain, h.SessionSecret)
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	if h.isRevoked(r.Context(), s) {
		http.Error(w, "session revoked", http.StatusUnauthorized)
		return
	}

	var body struct {
		JTI string `json:"jti"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	targetJTI, err := uuid.Parse(body.JTI)
	if err != nil {
		http.Error(w, "invalid jti", http.StatusBadRequest)
		return
	}

	if h.Store != nil && h.Store.Pool() != nil {
		if err := RevokeSession(r.Context(), h.Store.Pool(), targetJTI, s.UserID, time.Unix(s.ExpiresAt, 0)); err != nil {
			h.error(w, "revoke session", err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// RevokeAll invalidates all sessions for the authenticated user.
func (h *Handler) RevokeAll(w http.ResponseWriter, r *http.Request) {
	subdomain, err := SubdomainFromHost(r.Host, h.DomainTLD)
	if err != nil {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}
	s, _, ok := SessionFromRequestV2(r, subdomain, h.SessionSecret)
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	if h.isRevoked(r.Context(), s) {
		http.Error(w, "session revoked", http.StatusUnauthorized)
		return
	}

	if h.Store != nil && h.Store.Pool() != nil {
		if err := RevokeAllUserSessions(r.Context(), h.Store.Pool(), s.UserID); err != nil {
			h.error(w, "revoke all sessions", err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// SessionFromRequestV2 reads the session cookie and decodes it using the
// two-layer V2 format bound to workspaceSlug. If the cookie is a V1
// token, it falls back to V1 decoding and sets needsUpgrade=true so the
// caller can re-issue a V2 cookie.
func SessionFromRequestV2(r *http.Request, workspaceSlug string, secret []byte) (s Session, needsUpgrade bool, ok bool) {
	c, err := r.Cookie(SessionCookieName)
	if err != nil {
		return Session{}, false, false
	}
	s, err = DecodeSessionV2(c.Value, workspaceSlug, secret)
	if err == nil {
		return s, false, true
	}
	// V2 failed — try V1 (transparent migration window).
	s, err = DecodeSession(c.Value, secret)
	if err != nil {
		return Session{}, false, false
	}
	return s, true, true
}

// SessionFromRequest reads and verifies the session cookie. Returns
// (Session{}, false) on any failure — callers do not need to
// distinguish "no cookie" from "tampered cookie" from "expired".
//
// Deprecated: use SessionFromRequestV2 which validates workspace binding.
// Retained for callers that don't have workspace slug in scope.
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

// isRevoked checks whether the session has been revoked. Returns false
// (allow) when no RevocationChecker is configured.
//
// SECURITY NOTE: fail-open by design — if the revocation DB query fails,
// the session is allowed through. This prioritizes availability over
// strict security. A DB outage should not lock out all users. The error
// is logged at WARN level with "revocation_check_failed" for alerting.
func (h *Handler) isRevoked(ctx context.Context, s Session) bool {
	if h.Revocations == nil {
		return false
	}
	revoked, err := h.Revocations.IsRevoked(ctx, s.JTI, s.UserID, s.IssuedAt)
	if err != nil {
		if h.Logger != nil {
			h.Logger.WarnContext(ctx, "revocation_check_failed: fail-open, session allowed",
				"err", err,
				"jti", s.JTI,
				"user_id", s.UserID,
			)
		}
		return false
	}
	return revoked
}

// revokeJTI records the session's JTI in the revocation store. Failures
// are logged but do not block the response (logout still clears the cookie).
func (h *Handler) revokeJTI(ctx context.Context, s Session) {
	if h.Store == nil || h.Store.Pool() == nil {
		return
	}
	if err := RevokeSession(ctx, h.Store.Pool(), s.JTI, s.UserID, time.Unix(s.ExpiresAt, 0)); err != nil {
		if h.Logger != nil {
			h.Logger.Error("failed to revoke JTI on logout", "err", err, "jti", s.JTI)
		}
	}
}

func (h *Handler) error(w http.ResponseWriter, ctx string, err error) {
	if h.Logger != nil {
		h.Logger.Error("auth handler error", "ctx", ctx, "err", err)
	}
	http.Error(w, fmt.Sprintf("%s: %v", ctx, err), http.StatusInternalServerError)
}
