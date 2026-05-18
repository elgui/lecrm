package http

import "net/http"

// cspPolicy is the Content-Security-Policy applied to all responses.
//
// 'unsafe-inline' for style-src is required by shadcn/Tailwind; scripts
// must remain 'self'-only per ADR-009 §5.2.
const cspPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'"

// cspMiddleware injects the CSP header on every response.
func cspMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", cspPolicy)
		next.ServeHTTP(w, r)
	})
}
