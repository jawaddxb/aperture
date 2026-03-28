package billing

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const (
	accountKey contextKey = "billing_account"
	adminKey   contextKey = "billing_is_admin"
)

// AccountFromContext extracts the billing account from the request context.
// Returns nil when billing is not active (dev mode).
func AccountFromContext(ctx context.Context) *Account {
	a, _ := ctx.Value(accountKey).(*Account)
	return a
}

// IsAdminFromContext returns whether the current request was made with an admin key.
func IsAdminFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(adminKey).(bool)
	return v
}

// AuthMiddleware returns HTTP middleware that resolves Bearer tokens via the AccountService.
// When the AccountService has no accounts (fresh install / dev mode), all requests pass through.
func AuthMiddleware(svc *AccountService, skipPaths []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Dev mode: no accounts exist → no auth required.
			if !svc.HasAccounts() {
				next.ServeHTTP(w, r)
				return
			}

			// Skip auth for configured paths.
			for _, prefix := range skipPaths {
				if strings.HasPrefix(r.URL.Path, prefix) {
					next.ServeHTTP(w, r)
					return
				}
			}

			key := extractBearerToken(r)
			if key == "" {
				writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing Authorization header")
				return
			}

			acct, isAdmin, err := svc.ResolveAPIKey(key)
			if err != nil {
				writeJSONError(w, http.StatusForbidden, "FORBIDDEN", "invalid API key")
				return
			}

			// Block requests when credits are exhausted (non-free plans).
			if acct.CreditBalance <= 0 && acct.Plan != "free" {
				writeJSONError(w, http.StatusPaymentRequired, "PAYMENT_REQUIRED", "credit balance exhausted")
				return
			}

			ctx := context.WithValue(r.Context(), accountKey, acct)
			ctx = context.WithValue(ctx, adminKey, isAdmin)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminOnly returns middleware that requires the is_admin flag on the API key.
func AdminOnly(svc *AccountService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// In dev mode (no accounts), allow admin routes for initial setup.
			if !svc.HasAccounts() {
				next.ServeHTTP(w, r)
				return
			}

			if !IsAdminFromContext(r.Context()) {
				writeJSONError(w, http.StatusForbidden, "FORBIDDEN", "admin access required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

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

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Inline JSON to avoid importing encoding/json for a simple error response.
	_, _ = w.Write([]byte(`{"error":"` + message + `","code":"` + code + `"}`))
}
