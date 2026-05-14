// Package http wires the Chi router with the v0 routes.
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
)

// NewRouter assembles the v0 HTTP router. Only the /auth/* surface and
// a healthz probe are wired today; the /v1/* REST surface lands in
// Sprint 7.
func NewRouter(logger *slog.Logger, authH *auth.Handler) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(slogMiddleware(logger))
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Auth routes.
	authH.Register(r)

	return r
}

// slogMiddleware emits one structured log line per request. Keeps
// formatting consistent with the structured slog handler used in main.
func slogMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"host", r.Host,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"ms", time.Since(start).Milliseconds(),
				"req_id", chimw.GetReqID(r.Context()),
			)
		})
	}
}
