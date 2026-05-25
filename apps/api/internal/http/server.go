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
	"github.com/gbconsult/lecrm/apps/api/internal/logging"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

// RouterDeps bundles the long-lived collaborators NewRouter needs.
type RouterDeps struct {
	Logger          *slog.Logger
	AuthHandler     *auth.Handler
	Resolver        workspace.Resolver
	TestList        *workspace.TestListHandler
	CookieDomainTLD string
}

// NewRouter assembles the v0 HTTP router.
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

	deps.AuthHandler.Register(r)

	if deps.TestList != nil && deps.Resolver != nil {
		r.Group(func(r chi.Router) {
			r.Use(workspace.Middleware(deps.Logger, deps.Resolver, deps.CookieDomainTLD))
			r.Get("/v1/_test/workspaces", deps.TestList.ServeHTTP)
		})
	}

	return r
}

// slogMiddleware attaches a per-request logger (with request_id) to the
// context and emits one structured log line per completed request.
// Workspace fields are populated by workspace.Middleware via the shared
// logging.RequestLog pointer.
func slogMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqID := chimw.GetReqID(r.Context())
			reqLogger := logger.With("request_id", reqID)

			rl := &logging.RequestLog{}
			ctx := logging.WithRequestLog(r.Context(), rl)
			ctx = logging.WithLogger(ctx, reqLogger)
			r = r.WithContext(ctx)

			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"host", r.Host,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"ms", time.Since(start).Milliseconds(),
				"request_id", reqID,
			}

			if rl.Workspace != "" {
				attrs = append(attrs, "workspace", rl.Workspace, "workspace_id", rl.WorkspaceID)
			}

			reqLogger.Info("http", attrs...)
		})
	}
}
