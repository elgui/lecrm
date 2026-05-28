// Package spa embeds the built React SPA (apps/web/dist) into the Go
// binary and serves it for every non-API path, with a client-side
// routing fallback to index.html (ADR-009 §5.1 — single-binary deploy).
//
// Build flow: `bun run build` (apps/web) produces apps/web/dist, which
// scripts/embed-spa.sh copies into this package's dist/ directory before
// `go build`. A committed dist/.gitkeep keeps the embed directive
// compilable in a fresh checkout where no SPA has been built yet; in
// that case the handler serves a clear placeholder rather than 404ing.
package spa

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
)

// dist holds the built SPA. `all:` is required so the .gitkeep sentinel
// (and any dot-prefixed Vite assets) are embedded — a plain `embed dist`
// would match no files in a fresh checkout and fail to compile.
//
//go:embed all:dist
var dist embed.FS

// apiPrefixes are request paths owned by the API/auth layer. An unmatched
// path under one of these returns a JSON 404 instead of the SPA shell, so
// a typo'd endpoint never silently returns HTML to an API client.
var apiPrefixes = []string{"/v1/", "/auth/", "/admin/", "/admin", "/healthz"}

// Handler serves embedded SPA assets with an index.html fallback.
type Handler struct {
	fsys   fs.FS
	logger *slog.Logger
	// hasIndex is false when no SPA was built into the binary (fresh
	// checkout with only dist/.gitkeep). The handler then emits a
	// placeholder so the failure mode is legible, not a blank 404.
	hasIndex bool
}

// New builds a Handler over the embedded dist/ tree.
func New(logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// fs.Sub only errors on a malformed path; "dist" is a constant.
		logger.Error("spa: fs.Sub failed", "err", err)
		sub = dist
	}
	_, statErr := fs.Stat(sub, "index.html")
	return &Handler{fsys: sub, logger: logger, hasIndex: statErr == nil}
}

// HasSPA reports whether a real SPA build is embedded (vs. just the
// .gitkeep sentinel). Callers can log a startup warning when false.
func (h *Handler) HasSPA() bool { return h.hasIndex }

// ServeHTTP serves a static asset when one matches the request path,
// otherwise falls back to index.html for client-side routing. It is
// intended to be mounted as the router's NotFound handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	reqPath := r.URL.Path
	for _, p := range apiPrefixes {
		if reqPath == p || strings.HasPrefix(reqPath, p) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}
	}

	if !h.hasIndex {
		// No SPA embedded — make the misconfiguration obvious.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("leCRM API is running, but no SPA build is embedded in this binary.\n" +
			"Run scripts/embed-spa.sh (bun run build + copy into internal/spa/dist) and rebuild.\n"))
		return
	}

	clean := path.Clean(strings.TrimPrefix(reqPath, "/"))
	if clean == "." || clean == "/" {
		clean = "index.html"
	}

	// Serve a real asset when it exists; otherwise serve the SPA shell so
	// deep links (e.g. /contacts/<uuid>) resolve client-side.
	if f, err := h.fsys.Open(clean); err == nil {
		info, statErr := f.Stat()
		_ = f.Close()
		if statErr == nil && !info.IsDir() {
			h.setCacheHeaders(w, clean)
			http.ServeFileFS(w, r, h.fsys, clean)
			return
		}
	}

	h.serveIndex(w, r)
}

// serveIndex writes index.html with no-cache so a redeploy is picked up
// immediately (the hashed assets it references are immutable).
func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, h.fsys, "index.html")
}

// setCacheHeaders marks Vite's content-hashed assets immutable and leaves
// everything else uncached.
func (h *Handler) setCacheHeaders(w http.ResponseWriter, name string) {
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
}
