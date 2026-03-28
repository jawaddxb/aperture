package api

import (
	"net/http"
	"strings"
)

const (
	strictCSP     = "default-src 'none'; frame-ancestors 'none'"
	permissiveCSP = "default-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://cdn.tailwindcss.com; font-src 'self' https://fonts.gstatic.com; script-src 'self' 'unsafe-inline' https://cdn.tailwindcss.com; img-src 'self' data:; media-src 'self' https://*.cloudfront.net"
)

// SecurityHeaders returns middleware that sets security response headers.
// API routes get a strict CSP; /website/* paths get a permissive CSP for
// rendering the built-in dashboard.
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "0")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

			if strings.HasPrefix(r.URL.Path, "/website/") {
				w.Header().Set("Content-Security-Policy", permissiveCSP)
			} else {
				w.Header().Set("Content-Security-Policy", strictCSP)
			}

			next.ServeHTTP(w, r)
		})
	}
}
