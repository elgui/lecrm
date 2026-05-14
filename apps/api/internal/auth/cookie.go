// Package auth implements the OIDC relying-party flow and session
// management for lecrm-api.
//
// Cookie scoping (ADR-009 §5.2, binding):
//
//	Session cookies MUST be scoped to the specific workspace subdomain
//	(e.g. Domain=acme.lecrm.fr). A wildcard Domain=lecrm.fr would leak
//	sessions across workspaces. The helpers in this file are the ONLY
//	place cookies are constructed; reviews should reject any call site
//	that hand-rolls http.Cookie with a different Domain shape.
package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// SessionCookieName is the name of the signed-session cookie. Bound
	// to the workspace subdomain via Domain= per ADR-009 §5.2.
	SessionCookieName = "lecrm_session"

	// StateCookieName carries the short-lived state+PKCE-verifier blob
	// between /auth/login and /auth/callback. Scoped to the workspace
	// subdomain identically.
	StateCookieName = "lecrm_oauth_state"

	// MaxAge is the session lifetime. 12h matches "log in once per
	// working day" — short enough that a lost device window is bounded;
	// long enough that re-prompts don't dominate UX.
	MaxAge = 12 * time.Hour
)

// CookieScope captures the inputs to constructing a session/state cookie.
//
// The constructor refuses to issue a cookie with a parent-domain
// wildcard, which is the load-bearing check from ADR-009 §5.2.
type CookieScope struct {
	WorkspaceSubdomain string // e.g. "acme"; must be non-empty and not contain '.'
	DomainTLD          string // e.g. "lecrm.fr"; must not start with '.'
	Secure             bool
}

// BuildSessionCookie returns a session cookie scoped to
// "<WorkspaceSubdomain>.<DomainTLD>" with SameSite=Strict, HttpOnly, and
// the configured Secure bit. It returns an error rather than silently
// producing a parent-domain cookie.
func BuildSessionCookie(s CookieScope, signedValue string) (*http.Cookie, error) {
	domain, err := scopedDomain(s)
	if err != nil {
		return nil, err
	}
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    signedValue,
		Path:     "/",
		Domain:   domain,
		MaxAge:   int(MaxAge.Seconds()),
		Secure:   s.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}, nil
}

// BuildStateCookie returns a short-lived cookie carrying the OAuth state
// blob between /auth/login and /auth/callback, scoped identically to
// the session cookie.
func BuildStateCookie(s CookieScope, signedValue string) (*http.Cookie, error) {
	domain, err := scopedDomain(s)
	if err != nil {
		return nil, err
	}
	return &http.Cookie{
		Name:     StateCookieName,
		Value:    signedValue,
		Path:     "/",
		Domain:   domain,
		MaxAge:   int((10 * time.Minute).Seconds()),
		Secure:   s.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}, nil
}

// ClearSessionCookie returns a cookie that, when sent, instructs the
// browser to delete the session cookie. The Domain MUST match the
// originally-set cookie or the browser ignores it.
func ClearSessionCookie(s CookieScope) (*http.Cookie, error) {
	domain, err := scopedDomain(s)
	if err != nil {
		return nil, err
	}
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   domain,
		MaxAge:   -1,
		Secure:   s.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}, nil
}

// scopedDomain composes "<subdomain>.<tld>" and rejects shapes that
// would leak across workspaces.
func scopedDomain(s CookieScope) (string, error) {
	if s.WorkspaceSubdomain == "" {
		return "", fmt.Errorf("workspace subdomain is required (no parent-domain cookies, ADR-009 §5.2)")
	}
	if strings.ContainsAny(s.WorkspaceSubdomain, ".*") {
		return "", fmt.Errorf("workspace subdomain %q must not contain '.' or '*'", s.WorkspaceSubdomain)
	}
	if s.DomainTLD == "" {
		return "", fmt.Errorf("domain TLD is required")
	}
	if strings.HasPrefix(s.DomainTLD, ".") {
		return "", fmt.Errorf("domain TLD %q must not start with '.'", s.DomainTLD)
	}
	return s.WorkspaceSubdomain + "." + s.DomainTLD, nil
}

// SubdomainFromHost extracts the workspace subdomain from an inbound
// Host header by stripping a trailing ".<DomainTLD>" suffix. It is the
// canonical way the server learns which workspace a request belongs to.
func SubdomainFromHost(host, domainTLD string) (string, error) {
	// strip port if present
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	host = strings.ToLower(strings.TrimSpace(host))
	domainTLD = strings.ToLower(strings.TrimSpace(domainTLD))
	suffix := "." + domainTLD
	if !strings.HasSuffix(host, suffix) {
		return "", fmt.Errorf("host %q is not a subdomain of %q", host, domainTLD)
	}
	sub := strings.TrimSuffix(host, suffix)
	if sub == "" || strings.Contains(sub, ".") {
		return "", fmt.Errorf("host %q does not have a single workspace subdomain", host)
	}
	return sub, nil
}
