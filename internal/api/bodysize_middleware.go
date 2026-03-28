package api

import "net/http"

// BodySizeLimit returns middleware that limits request body size using
// http.MaxBytesReader. When Content-Length exceeds maxBytes, the request is
// rejected immediately with 413. For chunked/streaming bodies the limit is
// enforced lazily by MaxBytesReader.
func BodySizeLimit(maxBytes int64) func(http.Handler) http.Handler {
	if maxBytes <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				writeError(w, http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", "request body too large")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
