package crm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadIdempotencyKey_AbsentHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/contacts", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	got, ok := readIdempotencyKey(w, r)
	if !ok {
		t.Fatalf("expected ok=true for absent header, body=%s", w.Body.String())
	}
	if got != "" {
		t.Errorf("expected empty key for absent header, got %q", got)
	}
}

func TestReadIdempotencyKey_PresentHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/contacts", strings.NewReader("{}"))
	r.Header.Set("Idempotency-Key", "abc-123")
	w := httptest.NewRecorder()
	got, ok := readIdempotencyKey(w, r)
	if !ok {
		t.Fatal("expected ok=true for valid header")
	}
	if got != "abc-123" {
		t.Errorf("got %q, want %q", got, "abc-123")
	}
}

func TestReadIdempotencyKey_TrimsWhitespace(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/contacts", nil)
	r.Header.Set("Idempotency-Key", "  spaced-key  ")
	w := httptest.NewRecorder()
	got, _ := readIdempotencyKey(w, r)
	if got != "spaced-key" {
		t.Errorf("got %q, want %q (header should be trimmed)", got, "spaced-key")
	}
}

func TestReadIdempotencyKey_TooLong(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/contacts", nil)
	r.Header.Set("Idempotency-Key", strings.Repeat("x", maxIdempotencyKeyLen+1))
	w := httptest.NewRecorder()
	_, ok := readIdempotencyKey(w, r)
	if ok {
		t.Fatal("expected ok=false for oversize key")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", w.Code)
	}
}

func TestWriteReplay_SetsHeadersAndBody(t *testing.T) {
	w := httptest.NewRecorder()
	body := []byte(`{"id":"x"}`)
	writeReplay(w, http.StatusCreated, body)

	if w.Code != http.StatusCreated {
		t.Errorf("status: got %d want 201", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type: got %q want application/json", got)
	}
	if got := w.Header().Get("Idempotency-Replayed"); got != "true" {
		t.Errorf("Idempotency-Replayed: got %q want true", got)
	}
	if w.Body.String() != string(body) {
		t.Errorf("body: got %q want %q", w.Body.String(), body)
	}
}

func TestWriteRaw_SetsContentTypeAndBody(t *testing.T) {
	w := httptest.NewRecorder()
	writeRaw(w, http.StatusOK, []byte(`{"ok":true}`))
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d want 200", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type: got %q want application/json", got)
	}
	// Must NOT set Idempotency-Replayed on a fresh response.
	if got := w.Header().Get("Idempotency-Replayed"); got != "" {
		t.Errorf("Idempotency-Replayed: got %q want empty on fresh response", got)
	}
}
