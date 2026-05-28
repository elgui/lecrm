package email

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/internal/email/brevo"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

func newTestHandler(t *testing.T) (*Handler, *fakeProvider, *fakeSuppression, *fakeAudit) {
	t.Helper()
	prov := &fakeProvider{}
	supp := &fakeSuppression{}
	aud := &fakeAudit{}
	svc := &Service{
		Provider:    prov,
		Suppression: supp,
		Audit:       aud,
	}
	return &Handler{
		Service:       svc,
		Logger:        slog.Default(),
		WebhookSource: StaticWebhookSecret([]byte("test-secret")),
	}, prov, supp, aud
}

func TestHandleSend_WorkspaceMismatch(t *testing.T) {
	h, _, _, _ := newTestHandler(t)
	urlID := uuid.New()
	ctxID := uuid.New()

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body := `{"from":{"email":"a@b"},"to":[{"email":"x@y"}],"subject":"s","text_content":"t"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+urlID.String()+"/emails",
		strings.NewReader(body))
	req = req.WithContext(workspace.WithWorkspace(req.Context(), &workspace.Context{
		ID: ctxID, Slug: "ws", RoleName: "workspace_x",
	}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status: got %d want 403, body=%s", w.Code, w.Body.String())
	}
}

func TestHandleSend_HappyPath(t *testing.T) {
	h, prov, _, aud := newTestHandler(t)
	wsID := uuid.New()

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body := `{"from":{"email":"from@lecrm"},"to":[{"email":"x@y"}],"subject":"hi","text_content":"there"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+wsID.String()+"/emails",
		strings.NewReader(body))
	req.Header.Set("X-Lecrm-Actor", "internal_service")
	req = req.WithContext(workspace.WithWorkspace(req.Context(), &workspace.Context{
		ID: wsID, Slug: "ws", RoleName: "workspace_xyz",
	}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status: got %d want 202, body=%s", w.Code, w.Body.String())
	}
	if len(prov.sent) != 1 {
		t.Errorf("provider not invoked")
	}
	if len(aud.events) != 1 || aud.events[0].Event != "email.send.requested" {
		t.Errorf("audit: %+v", aud.events)
	}
	if aud.events[0].ActorType != ActorInternalService {
		t.Errorf("actor_type: %q", aud.events[0].ActorType)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["message_id"] == "" {
		t.Errorf("no message_id in response: %s", w.Body.String())
	}
}

func TestHandleSend_RejectsBadActor(t *testing.T) {
	h, _, _, _ := newTestHandler(t)
	wsID := uuid.New()
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body := `{"from":{"email":"a@b"},"to":[{"email":"x@y"}],"subject":"s","text_content":"t"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+wsID.String()+"/emails",
		strings.NewReader(body))
	req.Header.Set("X-Lecrm-Actor", "mcp_agent")
	req = req.WithContext(workspace.WithWorkspace(req.Context(), &workspace.Context{
		ID: wsID, RoleName: "workspace_x",
	}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401, body=%s", w.Code, w.Body.String())
	}
}

func TestHandleSend_ValidationErrors(t *testing.T) {
	h, _, _, _ := newTestHandler(t)
	wsID := uuid.New()
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	tests := []struct {
		name string
		body string
	}{
		{"missing subject", `{"from":{"email":"a@b"},"to":[{"email":"x@y"}],"text_content":"t"}`},
		{"missing from", `{"to":[{"email":"x@y"}],"subject":"s","text_content":"t"}`},
		{"missing to", `{"from":{"email":"a@b"},"subject":"s","text_content":"t"}`},
		{"missing content", `{"from":{"email":"a@b"},"to":[{"email":"x@y"}],"subject":"s"}`},
		{"bad json", `{nope}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+wsID.String()+"/emails",
				strings.NewReader(tt.body))
			req = req.WithContext(workspace.WithWorkspace(req.Context(), &workspace.Context{
				ID: wsID, RoleName: "workspace_x",
			}))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status: got %d want 400, body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleWebhook_Success(t *testing.T) {
	h, _, supp, aud := newTestHandler(t)

	wsID := uuid.New()
	body := []byte(`{"event":"hardBounce","email":"x@y","message-id":"<m1>"}`)
	mac := hmac.New(sha256.New, []byte("test-secret"))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	r := chi.NewRouter()
	h.RegisterWebhookRoute(r)
	req := httptest.NewRequest(http.MethodPost,
		"/v1/email/webhooks/brevo?workspace="+wsID.String(), bytes.NewReader(body))
	req.Header.Set(brevo.SignatureHeader, sig)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200, body=%s", w.Code, w.Body.String())
	}
	if !supp.suppressed["x@y"] {
		t.Errorf("suppression not upserted")
	}
	if len(aud.events) == 0 || aud.events[0].Event != "email.event.received" {
		t.Errorf("audit: %+v", aud.events)
	}
}

func TestHandleWebhook_InvalidSignature(t *testing.T) {
	h, _, _, _ := newTestHandler(t)
	wsID := uuid.New()
	body := []byte(`{"event":"hardBounce","email":"x@y"}`)

	r := chi.NewRouter()
	h.RegisterWebhookRoute(r)
	req := httptest.NewRequest(http.MethodPost,
		"/v1/email/webhooks/brevo?workspace="+wsID.String(), bytes.NewReader(body))
	req.Header.Set(brevo.SignatureHeader, hex.EncodeToString(make([]byte, 32)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401, body=%s", w.Code, w.Body.String())
	}
}

func TestHandleWebhook_MissingSignature(t *testing.T) {
	h, _, _, _ := newTestHandler(t)
	wsID := uuid.New()
	r := chi.NewRouter()
	h.RegisterWebhookRoute(r)
	body := []byte(`{"event":"delivered","email":"x@y"}`)
	req := httptest.NewRequest(http.MethodPost,
		"/v1/email/webhooks/brevo?workspace="+wsID.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", w.Code)
	}
}

func TestHandleWebhook_MissingWorkspace(t *testing.T) {
	h, _, _, _ := newTestHandler(t)
	r := chi.NewRouter()
	h.RegisterWebhookRoute(r)
	req := httptest.NewRequest(http.MethodPost,
		"/v1/email/webhooks/brevo", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", w.Code)
	}
}

func TestWorkspaceSchemaName_MatchesProvisioner(t *testing.T) {
	id := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	want := "workspace_0123456789abcdef0123456789abcdef"
	if got := workspaceSchemaName(id); got != want {
		t.Errorf("workspaceSchemaName: got %q want %q", got, want)
	}
}

// Sanity check that the brevo.Client satisfies the Provider interface.
func TestBrevoClient_IsProvider(t *testing.T) {
	var _ Provider = brevo.New("k", "", nil)
}

// Sanity check that the handler-level workspace ctx wiring round-trips.
func TestWorkspaceCtxRoundTrip(t *testing.T) {
	ws := &workspace.Context{ID: uuid.New(), Slug: "s", RoleName: "workspace_x"}
	ctx := workspace.WithWorkspace(context.Background(), ws)
	got, err := workspace.WorkspaceFromContext(ctx)
	if err != nil || got.ID != ws.ID {
		t.Fatalf("roundtrip: %v %+v", err, got)
	}
}

// Sanity check that the request body is bounded.
func TestSendBody_TooLarge_RespondsCleanly(t *testing.T) {
	h, _, _, _ := newTestHandler(t)
	wsID := uuid.New()
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	bigBody := `{"from":{"email":"a@b"},"to":[{"email":"x@y"}],"subject":"s","text_content":"` +
		strings.Repeat("x", maxSendBodyBytes+1) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+wsID.String()+"/emails",
		strings.NewReader(bigBody))
	req = req.WithContext(workspace.WithWorkspace(req.Context(), &workspace.Context{
		ID: wsID, RoleName: "workspace_x",
	}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status: got %d want 400 or 413", w.Code)
	}
	_, _ = io.Copy(io.Discard, w.Body)
}
