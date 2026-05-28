package reports

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

type recordingAudit struct {
	calls       int
	gotWS       uuid.UUID
	gotActor    uuid.UUID
	returnError error
}

func (r *recordingAudit) WriteEmbedTokenAudit(_ context.Context, ws, actor uuid.UUID) error {
	r.calls++
	r.gotWS = ws
	r.gotActor = actor
	return r.returnError
}

func newTestHandler(t *testing.T, audit AuditWriter, decode SessionDecoder) http.Handler {
	t.Helper()
	h := &Handler{
		JWTSecret:     testSecret,
		TTL:           5 * time.Minute,
		DecodeSession: decode,
		Audit:         audit,
	}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

func newReqWithWorkspace(method, path string, body io.Reader, ws *workspace.Context) *http.Request {
	r := httptest.NewRequest(method, path, body)
	r = r.WithContext(workspace.WithWorkspace(r.Context(), ws))
	return r
}

func TestEmbedToken_HappyPath(t *testing.T) {
	wsID := uuid.New()
	userID := uuid.New()
	ws := &workspace.Context{ID: wsID, Slug: "acme", RoleName: "workspace_x"}

	audit := &recordingAudit{}
	decode := func(_ *http.Request, slug string) (auth.Session, bool) {
		if slug != "acme" {
			t.Errorf("decode got slug %q want %q", slug, "acme")
		}
		return auth.Session{UserID: userID, WorkspaceID: wsID}, true
	}
	srv := newTestHandler(t, audit, decode)

	w := httptest.NewRecorder()
	req := newReqWithWorkspace(http.MethodPost, "/v1/reports/embed-token", nil, ws)
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200: body=%s", w.Code, w.Body.String())
	}

	var resp embedTokenResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("empty token")
	}
	if resp.ExpiresAt.IsZero() {
		t.Fatal("zero expires_at")
	}

	claims, err := VerifyEmbedToken(resp.Token, testSecret)
	if err != nil {
		t.Fatalf("verify minted token: %v", err)
	}
	if claims.WorkspaceID != wsID {
		t.Errorf("claims workspace_id: got %s want %s", claims.WorkspaceID, wsID)
	}

	if audit.calls != 1 {
		t.Errorf("audit called %d times, want 1", audit.calls)
	}
	if audit.gotWS != wsID {
		t.Errorf("audit workspace_id: got %s want %s", audit.gotWS, wsID)
	}
	if audit.gotActor != userID {
		t.Errorf("audit actor_id: got %s want %s", audit.gotActor, userID)
	}
}

func TestEmbedToken_CrossWorkspaceForbidden(t *testing.T) {
	requestWS := uuid.New()  // workspace from subdomain
	sessionWS := uuid.New()  // workspace from session cookie — mismatched
	ws := &workspace.Context{ID: requestWS, Slug: "acme"}

	audit := &recordingAudit{}
	decode := func(_ *http.Request, _ string) (auth.Session, bool) {
		return auth.Session{UserID: uuid.New(), WorkspaceID: sessionWS}, true
	}
	srv := newTestHandler(t, audit, decode)

	w := httptest.NewRecorder()
	req := newReqWithWorkspace(http.MethodPost, "/v1/reports/embed-token", nil, ws)
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status: got %d want 403", w.Code)
	}
	if audit.calls != 0 {
		t.Errorf("audit was called %d times; cross-workspace requests must not mint tokens", audit.calls)
	}
}

func TestEmbedToken_NoSessionUnauthorized(t *testing.T) {
	ws := &workspace.Context{ID: uuid.New(), Slug: "acme"}

	audit := &recordingAudit{}
	decode := func(_ *http.Request, _ string) (auth.Session, bool) {
		return auth.Session{}, false
	}
	srv := newTestHandler(t, audit, decode)

	w := httptest.NewRecorder()
	req := newReqWithWorkspace(http.MethodPost, "/v1/reports/embed-token", nil, ws)
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", w.Code)
	}
	if audit.calls != 0 {
		t.Errorf("audit called for unauthenticated request: %d", audit.calls)
	}
}

func TestEmbedToken_MissingWorkspaceContext(t *testing.T) {
	audit := &recordingAudit{}
	srv := newTestHandler(t, audit,
		func(_ *http.Request, _ string) (auth.Session, bool) {
			return auth.Session{UserID: uuid.New(), WorkspaceID: uuid.New()}, true
		})

	// Note: NO workspace.WithWorkspace — simulates middleware misconfig.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/reports/embed-token", nil)
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d want 500", w.Code)
	}
}

func TestEmbedToken_AuditFailureRejects(t *testing.T) {
	wsID := uuid.New()
	ws := &workspace.Context{ID: wsID, Slug: "acme"}

	audit := &recordingAudit{returnError: errors.New("db down")}
	decode := func(_ *http.Request, _ string) (auth.Session, bool) {
		return auth.Session{UserID: uuid.New(), WorkspaceID: wsID}, true
	}
	srv := newTestHandler(t, audit, decode)

	w := httptest.NewRecorder()
	req := newReqWithWorkspace(http.MethodPost, "/v1/reports/embed-token", nil, ws)
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d want 500 (fail-closed on audit failure)", w.Code)
	}
}

func TestEmbedToken_SecretMisconfigured(t *testing.T) {
	ws := &workspace.Context{ID: uuid.New(), Slug: "acme"}

	h := &Handler{
		JWTSecret: []byte(""),
		Audit:     &recordingAudit{},
		DecodeSession: func(_ *http.Request, _ string) (auth.Session, bool) {
			return auth.Session{UserID: uuid.New(), WorkspaceID: ws.ID}, true
		},
	}
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	w := httptest.NewRecorder()
	req := newReqWithWorkspace(http.MethodPost, "/v1/reports/embed-token", nil, ws)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d want 503 (missing JWT secret)", w.Code)
	}
}
