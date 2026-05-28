// Package http wires the Chi router with the v0 routes.
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/gbconsult/lecrm/apps/api/internal/admin"
	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	"github.com/gbconsult/lecrm/apps/api/internal/crm"
	"github.com/gbconsult/lecrm/apps/api/internal/email"
	"github.com/gbconsult/lecrm/apps/api/internal/logging"
	"github.com/gbconsult/lecrm/apps/api/internal/metadata"
	"github.com/gbconsult/lecrm/apps/api/internal/reports"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

// RouterDeps bundles the long-lived collaborators NewRouter needs.
type RouterDeps struct {
	Logger          *slog.Logger
	AuthHandler     *auth.Handler
	Resolver        workspace.Resolver
	TestList        *workspace.TestListHandler
	Metadata        *metadata.Handler
	CRM             *crm.Handler
	Email           *email.Handler
	Admin           *admin.AuditHandler
	Reports         *reports.Handler
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

	// Brevo inbound webhook lives OUTSIDE the workspace-middleware group:
	// auth is the HMAC over the request body, not a session cookie.
	if deps.Email != nil {
		deps.Email.RegisterWebhookRoute(r)
	}

	// /admin/audit lives outside the workspace-middleware group too:
	// Léo passes ?tenant=X explicitly and authenticates with a shared
	// bearer token, not a workspace-bound session cookie.
	if deps.Admin != nil {
		deps.Admin.Register(r)
	}

	if deps.Resolver != nil {
		r.Group(func(r chi.Router) {
			r.Use(workspace.Middleware(deps.Logger, deps.Resolver, deps.CookieDomainTLD))
			if deps.TestList != nil {
				r.Get("/v1/_test/workspaces", deps.TestList.ServeHTTP)
			}
			if deps.Metadata != nil {
				deps.Metadata.RegisterRoutes(r)
			}
			if deps.CRM != nil {
				deps.CRM.RegisterRoutes(r)
				deps.CRM.RegisterANTRoutes(r)
			}
			if deps.Email != nil {
				deps.Email.RegisterRoutes(r)
			}
			if deps.Reports != nil {
				deps.Reports.RegisterRoutes(r)
			}
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
