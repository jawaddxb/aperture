// Package api provides HTTP handlers and middleware for Aperture.
// This file implements CORS middleware for cross-origin requests.
package api

import (
	"net/http"
	"strings"
)

// CORSMiddleware returns middleware that sets CORS headers.
// When origins is empty and rejectUnknown is false, all origins are allowed (dev mode).
// When origins is empty and rejectUnknown is true, requests with an Origin header
// are rejected (production API-only mode — no browser callers expected).
// When origins is non-empty, only listed origins are allowed.
func CORSMiddleware(origins []string, rejectUnknown bool) func(http.Handler) http.Handler {
	allowAll := len(origins) == 0
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[strings.TrimRight(o, "/")] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if allowAll {
				if rejectUnknown && origin != "" {
					writeError(w, http.StatusForbidden, "CORS_REJECTED", "origin not allowed")
					return
				}
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if allowed[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			} else if origin != "" {
				writeError(w, http.StatusForbidden, "CORS_REJECTED", "origin not allowed")
				return
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
