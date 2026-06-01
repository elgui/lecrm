package reports

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

// newNativeHandler builds a Handler whose Pool is non-nil but never connected.
// The auth + validation guards under test short-circuit before any DB access,
// so the pool is never dereferenced. (Tests that need a live DB are in
// run_integration_test.go, tagged `integration`.)
func newNativeHandler(decode SessionDecoder) (http.Handler, *Handler) {
	h := &Handler{
		DecodeSession: decode,
		Pool:          &pgxpool.Pool{}, // non-nil sentinel; unused on guarded paths
	}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r, h
}

func okSession(wsID uuid.UUID) SessionDecoder {
	return func(_ *http.Request, _ string) (auth.Session, bool) {
		return auth.Session{UserID: uuid.New(), WorkspaceID: wsID}, true
	}
}

func TestNative_PoolUnconfigured503(t *testing.T) {
	// newTestHandler (from handler_test.go) leaves Pool nil.
	srv := newTestHandler(t, &recordingAudit{}, okSession(uuid.New()))
	ws := &workspace.Context{ID: uuid.New(), Slug: "acme", RoleName: "workspace_x"}

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, newReqWithWorkspace(http.MethodGet, "/v1/reports/definitions", nil, ws))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d want 503 (pool unconfigured): %s", w.Code, w.Body.String())
	}
}

func TestNative_CrossWorkspaceForbidden(t *testing.T) {
	requestWS := uuid.New()
	sessionWS := uuid.New() // mismatched
	srv, _ := newNativeHandler(okSession(sessionWS))
	ws := &workspace.Context{ID: requestWS, Slug: "acme", RoleName: "workspace_x"}

	for _, path := range []string{"/v1/reports/run", "/v1/reports/definitions"} {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, newReqWithWorkspace(http.MethodPost, path, strings.NewReader(`{}`), ws))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: got %d want 403 (cross-workspace)", path, w.Code)
		}
	}
}

func TestNative_NoSessionUnauthorized(t *testing.T) {
	srv, _ := newNativeHandler(func(_ *http.Request, _ string) (auth.Session, bool) {
		return auth.Session{}, false
	})
	ws := &workspace.Context{ID: uuid.New(), Slug: "acme", RoleName: "workspace_x"}

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, newReqWithWorkspace(http.MethodGet, "/v1/reports/definitions", nil, ws))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want 401", w.Code)
	}
}

func TestNative_RunInvalidDefinition400(t *testing.T) {
	wsID := uuid.New()
	srv, _ := newNativeHandler(okSession(wsID))
	ws := &workspace.Context{ID: wsID, Slug: "acme", RoleName: "workspace_x"}

	// Unknown metric is rejected by BuildRunQuery before any DB access.
	body := `{"metric":"evil","dimension":"none","period":"all"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, newReqWithWorkspace(http.MethodPost, "/v1/reports/run", strings.NewReader(body), ws))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d want 400 (invalid metric): %s", w.Code, w.Body.String())
	}
}
