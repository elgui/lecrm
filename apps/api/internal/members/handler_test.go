package members

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	"github.com/gbconsult/lecrm/apps/api/internal/rbac"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

var testWS = uuid.MustParse("22222222-2222-2222-2222-222222222222")

// memStore is an in-memory Store for handler tests.
type memStore struct {
	members      map[uuid.UUID]Member
	invited      []string
	updateErr    error
	removeErr    error
	lastRoleSet  rbac.Role
	lastRemoveID uuid.UUID
}

func newMemStore() *memStore { return &memStore{members: map[uuid.UUID]Member{}} }

func (m *memStore) ListMembers(_ context.Context, _ uuid.UUID) ([]Member, error) {
	out := make([]Member, 0, len(m.members))
	for _, v := range m.members {
		out = append(out, v)
	}
	return out, nil
}

func (m *memStore) Invite(_ context.Context, _ uuid.UUID, email string, role rbac.Role) (Member, error) {
	m.invited = append(m.invited, email)
	id := uuid.New()
	mem := Member{UserID: id, Email: &email, Role: role.String(), InvitedAt: time.Now(), Pending: true}
	m.members[id] = mem
	return mem, nil
}

func (m *memStore) UpdateRole(_ context.Context, _ uuid.UUID, userID uuid.UUID, role rbac.Role) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.lastRoleSet = role
	return nil
}

func (m *memStore) RemoveMember(_ context.Context, _ uuid.UUID, userID uuid.UUID) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	m.lastRemoveID = userID
	return nil
}

// harness wires the members handler with a workspace context and a session
// decoder that returns actorID. principalRole, when non-zero, is injected as
// the request principal (for /me).
func harness(t *testing.T, store Store, actorID uuid.UUID, principalRole rbac.Role) http.Handler {
	t.Helper()
	h := &Handler{
		Store: store,
		DecodeSession: func(_ *http.Request, _ string) (auth.Session, bool) {
			if actorID == uuid.Nil {
				return auth.Session{}, false
			}
			return auth.Session{UserID: actorID, WorkspaceID: testWS}, true
		},
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := workspace.WithWorkspace(req.Context(), &workspace.Context{ID: testWS, Slug: "acme"})
			if principalRole != rbac.RoleNone {
				ctx = rbac.WithPrincipal(ctx, &rbac.Principal{
					Role: principalRole, UserID: actorID, ActorType: "human_api",
				})
			}
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	h.RegisterRoutes(r)
	h.RegisterMeRoute(r)
	return r
}

func do(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rdr)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestMe_ReturnsRoleAndPermissions(t *testing.T) {
	owner := uuid.New()
	router := harness(t, newMemStore(), owner, rbac.RoleOwner)
	rec := do(t, router, "GET", "/v1/workspace/me", "")
	if rec.Code != 200 {
		t.Fatalf("got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var resp meResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Role != "owner" || !resp.Permissions.CanManageMembers || resp.UserID != owner.String() {
		t.Errorf("unexpected me response: %+v", resp)
	}
}

func TestMe_MemberHasNoManageMembers(t *testing.T) {
	member := uuid.New()
	router := harness(t, newMemStore(), member, rbac.RoleMember)
	rec := do(t, router, "GET", "/v1/workspace/me", "")
	var resp meResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Permissions.CanWrite || resp.Permissions.CanManageMembers {
		t.Errorf("member should not write or manage members: %+v", resp.Permissions)
	}
	if !resp.Permissions.CanRead {
		t.Error("member should be able to read")
	}
}

func TestInvite_Valid(t *testing.T) {
	store := newMemStore()
	router := harness(t, store, uuid.New(), rbac.RoleOwner)
	rec := do(t, router, "POST", "/v1/workspace/members/invite", `{"email":"new@acme.test","role":"admin"}`)
	if rec.Code != 201 {
		t.Fatalf("got %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	if len(store.invited) != 1 || store.invited[0] != "new@acme.test" {
		t.Errorf("invite not recorded: %v", store.invited)
	}
}

func TestInvite_DefaultsToMember(t *testing.T) {
	store := newMemStore()
	router := harness(t, store, uuid.New(), rbac.RoleOwner)
	rec := do(t, router, "POST", "/v1/workspace/members/invite", `{"email":"new@acme.test"}`)
	if rec.Code != 201 {
		t.Fatalf("got %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	var m Member
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m.Role != "member" {
		t.Errorf("default role = %q, want member", m.Role)
	}
}

func TestInvite_RejectsBadEmail(t *testing.T) {
	router := harness(t, newMemStore(), uuid.New(), rbac.RoleOwner)
	rec := do(t, router, "POST", "/v1/workspace/members/invite", `{"email":"notanemail"}`)
	if rec.Code != 400 {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestInvite_RejectsBadRole(t *testing.T) {
	router := harness(t, newMemStore(), uuid.New(), rbac.RoleOwner)
	rec := do(t, router, "POST", "/v1/workspace/members/invite", `{"email":"x@y.com","role":"superuser"}`)
	if rec.Code != 400 {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestUpdateRole_CannotDemoteSelf(t *testing.T) {
	owner := uuid.New()
	router := harness(t, newMemStore(), owner, rbac.RoleOwner)
	rec := do(t, router, "PATCH", "/v1/workspace/members/"+owner.String()+"/role", `{"role":"member"}`)
	if rec.Code != 400 {
		t.Fatalf("got %d, want 400 (self-demote must be rejected)", rec.Code)
	}
}

func TestUpdateRole_Valid(t *testing.T) {
	owner := uuid.New()
	target := uuid.New()
	store := newMemStore()
	router := harness(t, store, owner, rbac.RoleOwner)
	rec := do(t, router, "PATCH", "/v1/workspace/members/"+target.String()+"/role", `{"role":"admin"}`)
	if rec.Code != 200 {
		t.Fatalf("got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if store.lastRoleSet != rbac.RoleAdmin {
		t.Errorf("role set = %v, want admin", store.lastRoleSet)
	}
}

func TestUpdateRole_NotFound(t *testing.T) {
	owner := uuid.New()
	target := uuid.New()
	store := newMemStore()
	store.updateErr = ErrMemberNotFound
	router := harness(t, store, owner, rbac.RoleOwner)
	rec := do(t, router, "PATCH", "/v1/workspace/members/"+target.String()+"/role", `{"role":"admin"}`)
	if rec.Code != 404 {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestUpdateRole_InvalidRole(t *testing.T) {
	owner := uuid.New()
	target := uuid.New()
	router := harness(t, newMemStore(), owner, rbac.RoleOwner)
	rec := do(t, router, "PATCH", "/v1/workspace/members/"+target.String()+"/role", `{"role":"god"}`)
	if rec.Code != 400 {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestRemoveMember_CannotRemoveSelf(t *testing.T) {
	owner := uuid.New()
	router := harness(t, newMemStore(), owner, rbac.RoleOwner)
	rec := do(t, router, "DELETE", "/v1/workspace/members/"+owner.String(), "")
	if rec.Code != 400 {
		t.Fatalf("got %d, want 400 (self-remove must be rejected)", rec.Code)
	}
}

func TestRemoveMember_Valid(t *testing.T) {
	owner := uuid.New()
	target := uuid.New()
	store := newMemStore()
	router := harness(t, store, owner, rbac.RoleOwner)
	rec := do(t, router, "DELETE", "/v1/workspace/members/"+target.String(), "")
	if rec.Code != 204 {
		t.Fatalf("got %d, want 204 (%s)", rec.Code, rec.Body.String())
	}
	if store.lastRemoveID != target {
		t.Errorf("removed %v, want %v", store.lastRemoveID, target)
	}
}

func TestRemoveMember_NotFound(t *testing.T) {
	owner := uuid.New()
	target := uuid.New()
	store := newMemStore()
	store.removeErr = ErrMemberNotFound
	router := harness(t, store, owner, rbac.RoleOwner)
	rec := do(t, router, "DELETE", "/v1/workspace/members/"+target.String(), "")
	if rec.Code != 404 {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestRemoveMember_InvalidID(t *testing.T) {
	router := harness(t, newMemStore(), uuid.New(), rbac.RoleOwner)
	rec := do(t, router, "DELETE", "/v1/workspace/members/not-a-uuid", "")
	if rec.Code != 400 {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestListMembers(t *testing.T) {
	store := newMemStore()
	id := uuid.New()
	email := "a@b.com"
	store.members[id] = Member{UserID: id, Email: &email, Role: "member", Pending: true}
	router := harness(t, store, uuid.New(), rbac.RoleOwner)
	rec := do(t, router, "GET", "/v1/workspace/members", "")
	if rec.Code != 200 {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	var resp struct {
		Data []Member `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Role != "member" {
		t.Errorf("unexpected list: %+v", resp.Data)
	}
}
