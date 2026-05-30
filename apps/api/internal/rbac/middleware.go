package rbac

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

// RoleLookup resolves a user's stored role within a workspace. The
// production binding is members.PgMemberStore (reading
// core.workspace_members); tests inject an in-memory stub.
//
// found is false when the user has no membership row in the workspace —
// the caller MUST deny (never default to member).
type RoleLookup interface {
	LookupRole(ctx context.Context, workspaceID, userID uuid.UUID) (role Role, found bool, err error)
}

// SessionDecoder reads and verifies the request's session cookie scoped to
// the given workspace slug. The production binding is
// auth.SessionFromRequestV2; tests inject a stub. Returns ok=false when no
// valid session cookie is present.
type SessionDecoder func(r *http.Request, slug string) (auth.Session, bool)

// Resolver resolves a request's effective Principal and deposits it into
// the context. It is the single seam both authentication paths funnel
// through before RequireRole enforcement.
//
// The zero value is unusable: Store and DecodeSession MUST be set.
type Resolver struct {
	Store         RoleLookup
	DecodeSession SessionDecoder
	Logger        *slog.Logger
}

// Resolve is the middleware that attaches a *Principal to the request
// context. It NEVER rejects a request on its own — authorization is the
// job of RequireRole. A request that resolves to no role simply carries no
// principal, and the downstream gate returns 401.
//
// Order of precedence:
//
//  1. A verified bearer-token actor (deposited upstream by
//     workspace.MiddlewareWithBearer) → service-token principal whose role
//     is derived from the token's scopes (capped at admin).
//  2. A valid session cookie whose embedded workspace matches the resolved
//     workspace → human principal whose role is looked up in
//     workspace_members.
//
// A missing workspace context is a 500 (Resolve was mounted without the
// workspace middleware in front of it).
func (rs *Resolver) Resolve(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := workspace.WorkspaceFromContext(r.Context())
		if err != nil {
			rs.log(r.Context()).Error("rbac.Resolve: no workspace in context (middleware not wired?)")
			writeJSONError(w, http.StatusInternalServerError, "workspace context missing")
			return
		}

		// (1) Service-token path — actor was verified upstream.
		if actor, ok := auth.BearerActorFromContext(r.Context()); ok {
			p := &Principal{
				Role:           roleFromScopes(actor.Scopes),
				ActorType:      actor.ActorType,
				IsServiceToken: true,
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
			return
		}

		// (2) Session-cookie path.
		if sess, ok := rs.DecodeSession(r, ws.Slug); ok && sess.WorkspaceID == ws.ID {
			role, found, lookupErr := rs.Store.LookupRole(r.Context(), ws.ID, sess.UserID)
			if lookupErr != nil {
				rs.log(r.Context()).Error("rbac.Resolve: role lookup failed", "err", lookupErr,
					"workspace_id", ws.ID, "user_id", sess.UserID)
				writeJSONError(w, http.StatusInternalServerError, "role lookup failed")
				return
			}
			if found {
				// Tag integrator-actor writes distinctly in the audit trail
				// (core.audit_log): an integrator session is owner-equivalent
				// but a separate, non-billable principal. Other human sessions
				// stay "human_api".
				actorType := "human_api"
				if role == RoleIntegrator {
					actorType = "integrator"
				}
				p := &Principal{
					Role:      role,
					UserID:    sess.UserID,
					ActorType: actorType,
				}
				next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
				return
			}
		}

		// No principal — downstream RequireRole returns 401.
		next.ServeHTTP(w, r)
	})
}

// RequireRole returns middleware that admits only principals whose role is
// at least min. Responses:
//
//   - 401 when no principal resolved (unauthenticated / non-member).
//   - 403 when the principal's role is below min.
func RequireRole(min Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFromContext(r.Context())
			if !ok || p.Role == RoleNone {
				writeJSONError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			if !p.Role.AtLeast(min) {
				writeJSONError(w, http.StatusForbidden, "insufficient role")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRoleByMethod gates safe HTTP methods (GET/HEAD/OPTIONS) at readMin
// and unsafe methods (POST/PUT/PATCH/DELETE) at writeMin. It is the single
// guard wrapped around the mixed-CRUD CRM route group: reads require
// member+, writes require admin+.
func RequireRoleByMethod(readMin, writeMin Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			min := writeMin
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				min = readMin
			}
			RequireRole(min)(next).ServeHTTP(w, r)
		})
	}
}

func (rs *Resolver) log(ctx context.Context) *slog.Logger {
	if rs.Logger != nil {
		return rs.Logger
	}
	return slog.Default()
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
