package rbac

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

// --- pure-logic tests -------------------------------------------------

func TestParseRole(t *testing.T) {
	cases := []struct {
		in    string
		want  Role
		valid bool
	}{
		{"member", RoleMember, true},
		{"admin", RoleAdmin, true},
		{"owner", RoleOwner, true},
		{"OWNER", RoleOwner, true},
		{"  admin  ", RoleAdmin, true},
		{"", RoleNone, false},
		{"superuser", RoleNone, false},
	}
	for _, c := range cases {
		got, ok := ParseRole(c.in)
		if got != c.want || ok != c.valid {
			t.Errorf("ParseRole(%q) = (%v,%v), want (%v,%v)", c.in, got, ok, c.want, c.valid)
		}
	}
}

func TestRoleAtLeast(t *testing.T) {
	cases := []struct {
		r    Role
		min  Role
		want bool
	}{
		{RoleMember, RoleMember, true},
		{RoleMember, RoleAdmin, false},
		{RoleMember, RoleOwner, false},
		{RoleAdmin, RoleMember, true},
		{RoleAdmin, RoleAdmin, true},
		{RoleAdmin, RoleOwner, false},
		{RoleOwner, RoleMember, true},
		{RoleOwner, RoleAdmin, true},
		{RoleOwner, RoleOwner, true},
		{RoleNone, RoleMember, false},
	}
	for _, c := range cases {
		if got := c.r.AtLeast(c.min); got != c.want {
			t.Errorf("%v.AtLeast(%v) = %v, want %v", c.r, c.min, got, c.want)
		}
	}
}

func TestPermissionsFor(t *testing.T) {
	if p := PermissionsFor(RoleMember); !p.CanRead || p.CanWrite || p.CanManageMembers {
		t.Errorf("member perms wrong: %+v", p)
	}
	if p := PermissionsFor(RoleAdmin); !p.CanRead || !p.CanWrite || p.CanManageMembers {
		t.Errorf("admin perms wrong: %+v", p)
	}
	if p := PermissionsFor(RoleOwner); !p.CanRead || !p.CanWrite || !p.CanManageMembers || !p.CanManageTokens || !p.CanDeleteWorkspace {
		t.Errorf("owner perms wrong: %+v", p)
	}
}

func TestRoleFromScopes(t *testing.T) {
	cases := []struct {
		scopes []string
		want   Role
	}{
		{[]string{"*"}, RoleAdmin},                            // wildcard capped at admin, never owner
		{[]string{"contacts:write"}, RoleAdmin},               // connector write-only token
		{[]string{"write"}, RoleAdmin},                        // bare write scope
		{[]string{"contacts:read"}, RoleMember},               // read-only
		{[]string{"read"}, RoleMember},                        // read-only
		{nil, RoleMember},                                     // unscoped → least privilege
		{[]string{"contacts:read", "deals:write"}, RoleAdmin}, // any write wins
	}
	for _, c := range cases {
		if got := roleFromScopes(c.scopes); got != c.want {
			t.Errorf("roleFromScopes(%v) = %v, want %v", c.scopes, got, c.want)
		}
	}
}

// --- HTTP matrix tests ------------------------------------------------

var testWorkspaceID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

// stubStore maps user IDs to roles. found is false for unknown users
// (mirrors a missing workspace_members row).
type stubStore map[uuid.UUID]Role

func (s stubStore) LookupRole(_ context.Context, _ uuid.UUID, userID uuid.UUID) (Role, bool, error) {
	r, ok := s[userID]
	return r, ok, nil
}

// userHeaderDecoder reads "X-Test-User" as the session user UUID, returning
// a session bound to the test workspace. Empty header → no session.
func userHeaderDecoder(r *http.Request, _ string) (auth.Session, bool) {
	v := r.Header.Get("X-Test-User")
	if v == "" {
		return auth.Session{}, false
	}
	uid, err := uuid.Parse(v)
	if err != nil {
		return auth.Session{}, false
	}
	return auth.Session{UserID: uid, WorkspaceID: testWorkspaceID}, true
}

// buildTestRouter mirrors the production grouping in internal/http/server.go:
// a CRM-style mixed-CRUD group (member+ reads, admin+ writes), an owner-only
// member-management group, and a member+ self-service route.
func buildTestRouter(store stubStore) http.Handler {
	resolver := &Resolver{Store: store, DecodeSession: userHeaderDecoder}

	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

	r := chi.NewRouter()
	// Inject a workspace context for every request (workspace.Middleware
	// does this in production from the subdomain).
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := workspace.WithWorkspace(req.Context(), &workspace.Context{
				ID: testWorkspaceID, Slug: "acme", RoleName: "ws_role",
			})
			// Promote a bearer-scopes test header into a BearerActor, the
			// way workspace.MiddlewareWithBearer would after verification.
			if scopes := req.Header.Values("X-Test-Bearer-Scope"); len(scopes) > 0 {
				ctx = auth.WithBearerActor(ctx, &auth.BearerActor{
					TokenID: uuid.New(), ActorType: "connector", Scopes: scopes,
				})
			}
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})

	// CRM CRUD group.
	r.Group(func(r chi.Router) {
		r.Use(resolver.Resolve)
		r.Use(RequireRoleByMethod(RoleMember, RoleAdmin))
		r.Get("/v1/contacts", ok)
		r.Post("/v1/contacts", ok)
		r.Delete("/v1/contacts/{id}", ok)
	})
	// Member self-service.
	r.Group(func(r chi.Router) {
		r.Use(resolver.Resolve)
		r.Use(RequireRole(RoleMember))
		r.Get("/v1/workspace/me", ok)
	})
	// Owner-only member management.
	r.Group(func(r chi.Router) {
		r.Use(resolver.Resolve)
		r.Use(RequireRole(RoleOwner))
		r.Delete("/v1/workspace/members/{id}", ok)
		r.Patch("/v1/workspace/members/{id}/role", ok)
	})
	return r
}

func TestRBACMatrix(t *testing.T) {
	memberID := uuid.New()
	adminID := uuid.New()
	ownerID := uuid.New()
	strangerID := uuid.New() // valid session, no membership row

	store := stubStore{memberID: RoleMember, adminID: RoleAdmin, ownerID: RoleOwner}
	router := buildTestRouter(store)

	type tc struct {
		name   string
		method string
		path   string
		user   string   // X-Test-User
		scopes []string // X-Test-Bearer-Scope
		want   int
	}
	cases := []tc{
		// Anonymous (no session, no token).
		{"anon read contacts → 401", "GET", "/v1/contacts", "", nil, 401},
		{"anon write contacts → 401", "POST", "/v1/contacts", "", nil, 401},
		{"anon me → 401", "GET", "/v1/workspace/me", "", nil, 401},
		{"anon delete member → 401", "DELETE", "/v1/workspace/members/x", "", nil, 401},

		// Member: read yes, write no, owner-actions no.
		{"member read contacts → 200", "GET", "/v1/contacts", memberID.String(), nil, 200},
		{"member create contact → 403", "POST", "/v1/contacts", memberID.String(), nil, 403},
		{"member delete contact → 403", "DELETE", "/v1/contacts/x", memberID.String(), nil, 403},
		{"member me → 200", "GET", "/v1/workspace/me", memberID.String(), nil, 200},
		{"member delete member → 403", "DELETE", "/v1/workspace/members/x", memberID.String(), nil, 403},
		{"member change role → 403", "PATCH", "/v1/workspace/members/x/role", memberID.String(), nil, 403},

		// Admin: read + write yes, owner-actions no.
		{"admin read contacts → 200", "GET", "/v1/contacts", adminID.String(), nil, 200},
		{"admin create contact → 200", "POST", "/v1/contacts", adminID.String(), nil, 200},
		{"admin delete contact → 200", "DELETE", "/v1/contacts/x", adminID.String(), nil, 200},
		{"admin me → 200", "GET", "/v1/workspace/me", adminID.String(), nil, 200},
		{"admin delete member → 403", "DELETE", "/v1/workspace/members/x", adminID.String(), nil, 403},
		{"admin change role → 403", "PATCH", "/v1/workspace/members/x/role", adminID.String(), nil, 403},

		// Owner: everything.
		{"owner create contact → 200", "POST", "/v1/contacts", ownerID.String(), nil, 200},
		{"owner delete member → 200", "DELETE", "/v1/workspace/members/x", ownerID.String(), nil, 200},
		{"owner change role → 200", "PATCH", "/v1/workspace/members/x/role", ownerID.String(), nil, 200},

		// Valid session but no membership row → treated as unauthenticated.
		{"stranger read contacts → 401", "GET", "/v1/contacts", strangerID.String(), nil, 401},

		// Service tokens (scope-derived, capped at admin).
		{"connector write token read → 200", "GET", "/v1/contacts", "", []string{"contacts:write"}, 200},
		{"connector write token create → 200", "POST", "/v1/contacts", "", []string{"contacts:write"}, 200},
		{"connector read token create → 403", "POST", "/v1/contacts", "", []string{"contacts:read"}, 403},
		{"connector wildcard token delete member → 403", "DELETE", "/v1/workspace/members/x", "", []string{"*"}, 403},
		{"connector wildcard token create → 200", "POST", "/v1/contacts", "", []string{"*"}, 200},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, c.path, nil)
			if c.user != "" {
				req.Header.Set("X-Test-User", c.user)
			}
			for _, s := range c.scopes {
				req.Header.Add("X-Test-Bearer-Scope", s)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("%s %s: got %d, want %d (body %q)", c.method, c.path, rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

// TestResolveMissingWorkspace verifies Resolve 500s when mounted without the
// workspace middleware (defensive: a misconfigured router must fail loudly).
func TestResolveMissingWorkspace(t *testing.T) {
	resolver := &Resolver{Store: stubStore{}, DecodeSession: userHeaderDecoder}
	h := resolver.Resolve(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/contacts", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", rec.Code)
	}
}
