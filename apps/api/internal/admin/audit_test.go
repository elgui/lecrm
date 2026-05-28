package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestAuditHandler_DisabledWhenTokenEmpty(t *testing.T) {
	h := &AuditHandler{Token: ""}
	r := chi.NewRouter()
	h.Register(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/audit?tenant=acme", nil)
	req.Header.Set("Authorization", "Bearer anything")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 when token empty, got %d", w.Code)
	}
}

func TestAuditHandler_RejectsMissingToken(t *testing.T) {
	h := &AuditHandler{Token: "secret"}
	r := chi.NewRouter()
	h.Register(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/audit?tenant=acme", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without token, got %d", w.Code)
	}
}

func TestAuditHandler_RejectsWrongToken(t *testing.T) {
	h := &AuditHandler{Token: "secret"}
	r := chi.NewRouter()
	h.Register(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/audit?tenant=acme", nil)
	req.Header.Set("Authorization", "Bearer not-the-secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 with wrong token, got %d", w.Code)
	}
}

func TestAuditHandler_RequiresTenant(t *testing.T) {
	h := &AuditHandler{Token: "secret"}
	r := chi.NewRouter()
	h.Register(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/audit", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 without tenant, got %d", w.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if !strings.Contains(body["error"], "tenant") {
		t.Errorf("want error mentioning tenant, got %q", body["error"])
	}
}

func TestAuditHandler_RejectsBadSince(t *testing.T) {
	h := &AuditHandler{Token: "secret"}
	r := chi.NewRouter()
	h.Register(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/audit?tenant=acme&since=yesterday", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for bad since, got %d", w.Code)
	}
}

func TestAuditHandler_RejectsBadLimit(t *testing.T) {
	h := &AuditHandler{Token: "secret"}
	r := chi.NewRouter()
	h.Register(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/audit?tenant=acme&limit=-5", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for negative limit, got %d", w.Code)
	}
}

func TestParseTime_RFC3339(t *testing.T) {
	got, err := parseTime("2026-05-27T10:00:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("want %v got %v", want, got)
	}
}

func TestParseTime_Duration(t *testing.T) {
	before := time.Now().UTC()
	got, err := parseTime("24h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := before.Add(-24 * time.Hour)
	// Allow generous slop for test scheduling jitter.
	if got.Sub(expected) > 5*time.Second || expected.Sub(got) > 5*time.Second {
		t.Errorf("want ~%v got %v", expected, got)
	}
}

func TestParseTime_Days(t *testing.T) {
	before := time.Now().UTC()
	got, err := parseTime("7d")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := before.Add(-7 * 24 * time.Hour)
	if got.Sub(expected) > 5*time.Second || expected.Sub(got) > 5*time.Second {
		t.Errorf("want ~%v got %v", expected, got)
	}
}

func TestParseTime_Garbage(t *testing.T) {
	if _, err := parseTime("not-a-time"); err == nil {
		t.Fatal("want error for garbage, got nil")
	}
}

func TestBearerToken_ExtractsAfterPrefix(t *testing.T) {
	cases := map[string]string{
		"Bearer abc":       "abc",
		"Bearer  spaced  ": "spaced",
		"abc":              "",
		"":                 "",
	}
	for in, want := range cases {
		if got := bearerToken(in); got != want {
			t.Errorf("bearerToken(%q) = %q want %q", in, got, want)
		}
	}
}

func TestCheckAuth_ConstantTimeCompare(t *testing.T) {
	h := &AuditHandler{Token: "matching"}
	req := httptest.NewRequest(http.MethodGet, "/admin/audit", nil)
	req.Header.Set("Authorization", "Bearer matching")
	if !h.checkAuth(req) {
		t.Fatal("expected auth to pass with matching token")
	}
	req2 := httptest.NewRequest(http.MethodGet, "/admin/audit", nil)
	req2.Header.Set("Authorization", "Bearer different")
	if h.checkAuth(req2) {
		t.Fatal("expected auth to fail with different token")
	}
}
