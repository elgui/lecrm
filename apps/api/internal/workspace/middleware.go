package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// Resolver looks up a workspace by its subdomain slug. It is the
// production seam that the middleware depends on; the live binding is
// in package auth (Store.WorkspaceBySlug). Tests can pass a stub
// without spinning Postgres.
type Resolver interface {
	WorkspaceBySlugFull(ctx context.Context, slug string) (id uuid.UUID, roleName string, err error)
}

// ErrUnknownWorkspace is the resolver-shaped "no such slug" sentinel.
// Resolvers MUST return this (not a wrapped pgx.ErrNoRows) so the
// middleware can return 404 without leaking driver detail.
var ErrUnknownWorkspace = errors.New("workspace: unknown slug")

// Middleware reads the first label of r.Host as the workspace slug,
// resolves it via Resolver, and attaches a *Context to the request
// context. CookieDomainTLD is the bare TLD (e.g. "lecrm.fr" or
// "lecrm.test") used to peel the leading subdomain label.
//
// Requests whose Host does not have a workspace subdomain (root domain,
// no leading label) get a 400. Unknown slugs get a 404 — NOT 401, to
// avoid an enumeration oracle per ADR-009 §5.2.
func Middleware(logger *slog.Logger, resolver Resolver, cookieDomainTLD string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slug, ok := subdomainOf(r.Host, cookieDomainTLD)
			if !ok {
				writeJSONError(w, http.StatusBadRequest, "workspace subdomain required")
				return
			}

			id, roleName, err := resolver.WorkspaceBySlugFull(r.Context(), slug)
			switch {
			case errors.Is(err, ErrUnknownWorkspace):
				writeJSONError(w, http.StatusNotFound, "workspace not found")
				return
			case err != nil:
				logger.ErrorContext(r.Context(), "workspace resolve failed", "err", err, "slug", slug)
				writeJSONError(w, http.StatusInternalServerError, "workspace resolve failed")
				return
			}

			ws := &Context{ID: id, Slug: slug, RoleName: roleName}
			next.ServeHTTP(w, r.WithContext(WithWorkspace(r.Context(), ws)))
		})
	}
}

// subdomainOf returns the leading label of host when host ends in
// "."+domainTLD. "acme.lecrm.fr" + "lecrm.fr" → "acme", true. Multi-
// label subdomains (a.b.lecrm.fr) are intentionally rejected at v0:
// the workspace namespace is a flat single-label space.
func subdomainOf(host, domainTLD string) (string, bool) {
	host = strings.ToLower(stripPort(host))
	domainTLD = strings.ToLower(domainTLD)
	if host == "" || domainTLD == "" {
		return "", false
	}
	if !strings.HasSuffix(host, "."+domainTLD) {
		return "", false
	}
	label := strings.TrimSuffix(host, "."+domainTLD)
	if label == "" || strings.Contains(label, ".") {
		return "", false
	}
	return label, true
}

func stripPort(host string) string {
	if i := strings.LastIndex(host, ":"); i > 0 && !strings.Contains(host[i:], "]") {
		return host[:i]
	}
	return host
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
