package workspace

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type stubResolver struct {
	id       uuid.UUID
	roleName string
	err      error
	calls    int
	lastSlug string
}

func (s *stubResolver) WorkspaceBySlugFull(_ context.Context, slug string) (uuid.UUID, string, error) {
	s.calls++
	s.lastSlug = slug
	if s.err != nil {
		return uuid.Nil, "", s.err
	}
	return s.id, s.roleName, nil
}

// silentLogger keeps middleware's error paths from polluting -v output.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestMiddleware_AttachesWorkspace(t *testing.T) {
	id := uuid.New()
	res := &stubResolver{id: id, roleName: "workspace_" + strings.ReplaceAll(id.String(), "-", "")}

	var observed *Context
	terminal := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ws, err := WorkspaceFromContext(r.Context())
		if err != nil {
			t.Fatalf("expected workspace in context, got err: %v", err)
		}
		observed = ws
	})

	h := Middleware(silentLogger(), res, "lecrm.test")(terminal)

	req := httptest.NewRequest(http.MethodGet, "/v1/_test/workspaces", nil)
	req.Host = "acme.lecrm.test:8080"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if res.lastSlug != "acme" {
		t.Fatalf("resolver got slug %q want %q", res.lastSlug, "acme")
	}
	if observed == nil || observed.ID != id || observed.Slug != "acme" {
		t.Fatalf("observed workspace: %+v", observed)
	}
}

func TestMiddleware_RejectsRootDomain(t *testing.T) {
	res := &stubResolver{}
	h := Middleware(silentLogger(), res, "lecrm.test")(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("terminal handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/_test/workspaces", nil)
	req.Host = "lecrm.test"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", rr.Code)
	}
	if res.calls != 0 {
		t.Fatalf("resolver should not be called: calls=%d", res.calls)
	}
}

func TestMiddleware_UnknownSlugIs404(t *testing.T) {
	res := &stubResolver{err: ErrUnknownWorkspace}
	h := Middleware(silentLogger(), res, "lecrm.test")(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("terminal handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/_test/workspaces", nil)
	req.Host = "ghost.lecrm.test"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want 404", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("body decode: %v", err)
	}
	if body["error"] == "" {
		t.Fatalf("expected error field, got %+v", body)
	}
}

func TestMiddleware_MultiLabelSubdomainRejected(t *testing.T) {
	res := &stubResolver{}
	h := Middleware(silentLogger(), res, "lecrm.test")(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("terminal handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/_test/workspaces", nil)
	req.Host = "evil.acme.lecrm.test"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", rr.Code)
	}
}

func TestMiddleware_TombstonedSlugIs404(t *testing.T) {
	res := &stubResolver{err: ErrUnknownWorkspace}
	h := Middleware(silentLogger(), res, "lecrm.test")(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("terminal handler should not run for tombstoned slug")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/_test/workspaces", nil)
	req.Host = "tombstoned-tenant.lecrm.test"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want 404", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("body decode: %v", err)
	}
	if body["error"] != "workspace not found" {
		t.Fatalf("expected generic 'workspace not found' error (no info leak), got %+v", body)
	}
}

func TestSubdomainOf(t *testing.T) {
	cases := []struct {
		host    string
		tld     string
		want    string
		wantOK  bool
	}{
		{"acme.lecrm.fr", "lecrm.fr", "acme", true},
		{"acme.lecrm.fr:8080", "lecrm.fr", "acme", true},
		{"ACME.LECRM.FR", "lecrm.fr", "acme", true},
		{"lecrm.fr", "lecrm.fr", "", false},
		{"a.b.lecrm.fr", "lecrm.fr", "", false},
		{"", "lecrm.fr", "", false},
		{"acme.example.com", "lecrm.fr", "", false},
	}
	for _, tc := range cases {
		got, ok := subdomainOf(tc.host, tc.tld)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("subdomainOf(%q,%q)=(%q,%v) want (%q,%v)", tc.host, tc.tld, got, ok, tc.want, tc.wantOK)
		}
	}
}
