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
	"github.com/gbconsult/lecrm/apps/api/internal/members"
	"github.com/gbconsult/lecrm/apps/api/internal/metadata"
	"github.com/gbconsult/lecrm/apps/api/internal/rbac"
	"github.com/gbconsult/lecrm/apps/api/internal/reports"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

// RouterDeps bundles the long-lived collaborators NewRouter needs.
type RouterDeps struct {
	Logger        *slog.Logger
	AuthHandler   *auth.Handler
	ServiceTokens *auth.ServiceTokenHandler
	BearerAuth    workspace.BearerAuthenticator
	Resolver      workspace.Resolver
	TestList      *workspace.TestListHandler
	Metadata      *metadata.Handler
	CRM           *crm.Handler
	Email         *email.Handler
	Admin         *admin.AuditHandler
	Reports       *reports.Handler
	// RBAC resolves and injects the per-request role principal. When nil,
	// the workspace group runs without role enforcement (back-compat for
	// tests that don't exercise authorization).
	RBAC *rbac.Resolver
	// Members serves the owner-only member-management endpoints and the
	// member+ /v1/workspace/me self-service endpoint. Mounted only when
	// RBAC is also configured.
	Members *members.Handler
	// SPA serves the embedded React build for every non-API path, with a
	// client-side routing fallback to index.html (ADR-009 §5.1). Mounted
	// as the router's NotFound handler. When nil, unmatched paths get
	// chi's default plain-text 404 (test seam).
	SPA             http.Handler
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
			r.Use(workspace.MiddlewareWithBearer(deps.Logger, deps.Resolver, deps.CookieDomainTLD, deps.BearerAuth))
			if deps.TestList != nil {
				r.Get("/v1/_test/workspaces", deps.TestList.ServeHTTP)
			}
			if deps.Metadata != nil {
				deps.Metadata.RegisterRoutes(r)
			}
			if deps.CRM != nil {
				// Apply role-based access control to CRM CRUD when an RBAC
				// resolver is wired: reads require member+, writes admin+.
				// Without a resolver the routes mount unguarded (test seam).
				if deps.RBAC != nil {
					r.Group(func(r chi.Router) {
						r.Use(deps.RBAC.Resolve)
						r.Use(rbac.RequireRoleByMethod(rbac.RoleMember, rbac.RoleAdmin))
						deps.CRM.RegisterRoutes(r)
						deps.CRM.RegisterANTRoutes(r)
					})
				} else {
					deps.CRM.RegisterRoutes(r)
					deps.CRM.RegisterANTRoutes(r)
				}
				// Connector ingestion (ADR-011) is authenticated by a
				// service token with the connector.push_events scope, NOT
				// by an RBAC role — so it mounts in its own group guarded
				// by the bearer-scope check rather than RequireRoleByMethod.
				r.Group(func(r chi.Router) {
					r.Use(crm.RequireConnectorScope)
					deps.CRM.RegisterConnectorRoutes(r)
				})
			}
			if deps.Email != nil {
				deps.Email.RegisterRoutes(r)
			}
			if deps.Reports != nil {
				deps.Reports.RegisterRoutes(r)
			}
			if deps.ServiceTokens != nil {
				deps.ServiceTokens.RegisterRoutes(r)
			}
			// Member management (owner-only) + self-service (member+).
			// Both require the RBAC resolver to inject the principal.
			if deps.RBAC != nil && deps.Members != nil {
				r.Group(func(r chi.Router) {
					r.Use(deps.RBAC.Resolve)
					r.Use(rbac.RequireRole(rbac.RoleMember))
					deps.Members.RegisterMeRoute(r)
				})
				r.Group(func(r chi.Router) {
					r.Use(deps.RBAC.Resolve)
					r.Use(rbac.RequireRole(rbac.RoleOwner))
					deps.Members.RegisterRoutes(r)
				})
			}
		})
	}

	// Embedded SPA: any request that matched no API route falls through to
	// the SPA handler, which serves a static asset or the index.html shell
	// for client-side routing. It runs the root middleware stack (incl.
	// CSP) but NOT the workspace middleware — serving the app shell never
	// requires a resolved workspace.
	if deps.SPA != nil {
		r.NotFound(deps.SPA.ServeHTTP)
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
