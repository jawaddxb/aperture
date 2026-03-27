// Package api provides HTTP handlers and middleware for Aperture.
// This file implements API key authentication middleware.
package api

import (
	"net/http"
	"strings"
)

// AuthConfig holds settings for the API key auth middleware.
type AuthConfig struct {
	// Keys is the set of valid API keys. Empty set disables auth.
	Keys map[string]bool
	// KeyPrefix is the expected prefix (e.g. "apt_"). Empty allows any.
	KeyPrefix string
	// SkipPaths are path prefixes that skip auth (e.g. "/health", "/website").
	SkipPaths []string
}

// APIKeyAuth returns middleware that validates Bearer tokens against a key set.
// When cfg.Keys is empty, all requests are allowed (dev mode).
func APIKeyAuth(cfg AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Dev mode — no keys configured, allow all.
			if len(cfg.Keys) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			// Skip auth for configured paths.
			for _, prefix := range cfg.SkipPaths {
				if strings.HasPrefix(r.URL.Path, prefix) {
					next.ServeHTTP(w, r)
					return
				}
			}

			key := extractBearerToken(r)
			if key == "" {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing Authorization header")
				return
			}

			if cfg.KeyPrefix != "" && !strings.HasPrefix(key, cfg.KeyPrefix) {
				writeError(w, http.StatusUnauthorized, "INVALID_KEY", "invalid API key format")
				return
			}

			if !cfg.Keys[key] {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "invalid API key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractBearerToken extracts the token from "Authorization: Bearer <token>".
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
