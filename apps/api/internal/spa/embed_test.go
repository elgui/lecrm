package spa

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// testHandler builds a Handler over an in-memory dist tree so the serving
// logic is exercised without depending on whether a real SPA was built
// into the package at test time.
func testHandler() *Handler {
	fsys := fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html><div id=root></div>")},
		"assets/app.abc123.js": {Data: []byte("console.log(1)")},
		"vite.svg":             {Data: []byte("<svg/>")},
	}
	return &Handler{fsys: fsys, logger: slog.Default(), hasIndex: true}
}

func do(t *testing.T, h *Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestServesIndexAtRoot(t *testing.T) {
	rec := do(t, testHandler(), http.MethodGet, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if got := string(body); got == "" || got[:9] != "<!doctype" {
		t.Fatalf("GET / body = %q, want index.html", got)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("index Cache-Control = %q, want no-cache", cc)
	}
}

func TestServesStaticAsset(t *testing.T) {
	rec := do(t, testHandler(), http.MethodGet, "/assets/app.abc123.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET asset = %d, want 200", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("asset Cache-Control = %q, want immutable", cc)
	}
}

func TestDeepLinkFallsBackToIndex(t *testing.T) {
	// A client-side route with no matching file must return the SPA shell,
	// not a 404 — that's what makes browser-refresh on /contacts/<id> work.
	rec := do(t, testHandler(), http.MethodGet, "/contacts/3f2504e0-4f89-41d3-9a0c-0305e82c3301")
	if rec.Code != http.StatusOK {
		t.Fatalf("deep link = %d, want 200", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if string(body[:9]) != "<!doctype" {
		t.Fatalf("deep link body = %q, want index.html", string(body))
	}
}

func TestApiPrefixReturnsJSON404(t *testing.T) {
	for _, p := range []string{"/v1/does-not-exist", "/auth/whatever", "/admin/x"} {
		rec := do(t, testHandler(), http.MethodGet, p)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("GET %s Content-Type = %q, want application/json", p, ct)
		}
	}
}

func TestNonGetReturns404(t *testing.T) {
	rec := do(t, testHandler(), http.MethodPost, "/contacts")
	if rec.Code != http.StatusNotFound {
		t.Errorf("POST / = %d, want 404", rec.Code)
	}
}

func TestNoSPAEmbeddedServes503(t *testing.T) {
	h := &Handler{fsys: fstest.MapFS{}, logger: slog.Default(), hasIndex: false}
	rec := do(t, h, http.MethodGet, "/")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no-SPA GET / = %d, want 503", rec.Code)
	}
}

// TestRealEmbedCompiles asserts the package-level New() constructor works
// against whatever is embedded (at minimum the .gitkeep sentinel).
func TestRealEmbedCompiles(t *testing.T) {
	h := New(slog.Default())
	if h == nil {
		t.Fatal("New returned nil")
	}
}

// TestRealEmbedServesIndexWhenBuilt exercises the actually-embedded SPA when
// one has been staged (scripts/embed-spa.sh). It skips in a fresh checkout
// where only dist/.gitkeep is present, so CI without a SPA build stays green
// while a release binary's embedded SPA is verified end-to-end.
func TestRealEmbedServesIndexWhenBuilt(t *testing.T) {
	h := New(slog.Default())
	if !h.HasSPA() {
		t.Skip("no SPA embedded (fresh checkout) — run scripts/embed-spa.sh to exercise this path")
	}
	rec := do(t, h, http.MethodGet, "/some/client/route")
	if rec.Code != http.StatusOK {
		t.Fatalf("deep link against real embed = %d, want 200", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(strings.ToLower(string(body)), "<div id=\"root\"") &&
		!strings.Contains(strings.ToLower(string(body)), "<div id=root") {
		t.Errorf("real embedded index.html missing #root mount node")
	}
}
