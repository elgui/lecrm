// Package http wires the Chi router with the v0 routes.
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

// RouterDeps bundles the long-lived collaborators NewRouter needs.
// Splitting out a deps struct keeps the wiring site (main) terse and
// makes the router's contract explicit at call time.
type RouterDeps struct {
	Logger          *slog.Logger
	AuthHandler     *auth.Handler
	Resolver        workspace.Resolver
	TestList        *workspace.TestListHandler
	CookieDomainTLD string
}

// NewRouter assembles the v0 HTTP router. The /auth/* surface and a
// healthz probe are always wired; the /v1/_test/workspaces handler is
// the ADR-009 §1.1 Week-2 Go-ramp checkpoint surface. The /v1/* REST
// surface proper lands in Sprint 7.
func NewRouter(deps RouterDeps) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(slogMiddleware(deps.Logger))
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(cspMiddleware)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Auth routes — host-bound, no workspace context required.
	deps.AuthHandler.Register(r)

	// Workspace-scoped surface. Only the Test 1 handler lives here at
	// v0; Sprint 7 mounts the full /v1/* CRUD tree under this group.
	if deps.TestList != nil && deps.Resolver != nil {
		r.Group(func(r chi.Router) {
			r.Use(workspace.Middleware(deps.Logger, deps.Resolver, deps.CookieDomainTLD))
			r.Get("/v1/_test/workspaces", deps.TestList.ServeHTTP)
		})
	}

	return r
}

// slogMiddleware emits one structured log line per request. Keeps
// formatting consistent with the structured slog handler used in main.
func slogMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"host", r.Host,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"ms", time.Since(start).Milliseconds(),
				"req_id", chimw.GetReqID(r.Context()),
			)
		})
	}
}
