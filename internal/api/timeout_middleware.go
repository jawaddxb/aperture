package api

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// sseSkipSuffixes are URL path suffixes for SSE/streaming endpoints
// that must not be subject to the global request timeout.
var sseSkipSuffixes = []string{"/stream", "/tasks/execute", "/tasks/resume"}

// RequestTimeout returns middleware that sets a context deadline on each request.
// SSE/streaming endpoints (identified by path suffix) are excluded.
// A seconds value of 0 or less disables the timeout.
func RequestTimeout(seconds int) func(http.Handler) http.Handler {
	if seconds <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	timeout := time.Duration(seconds) * time.Second

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, suffix := range sseSkipSuffixes {
				if strings.HasSuffix(r.URL.Path, suffix) {
					next.ServeHTTP(w, r)
					return
				}
			}
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
