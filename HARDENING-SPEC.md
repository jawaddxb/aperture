# Aperture P1 Security Hardening Spec

## Overview
10 concrete hardening items to move Aperture from "production-ready" to "production-hardened".
All changes must maintain 60/60 E2E test pass rate. All changes must be testable.

## 1. Rate Limiting (enable default)

**File:** `aperture.yaml` + `internal/api/ratelimit_middleware.go`
**Change:** Set default RPM to 120 in config defaults. Add `APERTURE_API_RATE_LIMIT_RPM` env var override.
**Config:**
```yaml
api:
  rate_limit_rpm: 120
```
**Note:** The middleware already works (tested in E2E). Just need to enable it with a sane default.
Update `internal/config/config.go` setDefaults to include `api.rate_limit_rpm: 120`.

## 2. CORS Origin Allowlist

**File:** `internal/api/cors_middleware.go` + config
**Change:** When `cors_origins` is empty AND `require_auth` is true (production mode), default to rejecting unknown origins instead of allowing all. Add a `cors_reject_unknown` bool config option.
**Config:**
```yaml
api:
  cors_origins: []  # empty = allow all in dev mode
  cors_reject_unknown: true  # when true + no origins configured, reject non-empty Origin headers
```
**Behavior:**
- Dev mode (require_auth=false): Allow all (current behavior)
- Prod mode (require_auth=true, no cors_origins): Reject requests with Origin header (API-only, no browser needed)
- Prod mode with cors_origins: Only allow listed origins

## 3. Request Body Size Limit

**File:** NEW `internal/api/bodysize_middleware.go`
**Change:** Add middleware that wraps `r.Body` with `http.MaxBytesReader`. Default 1MB. Configurable.
**Config:**
```yaml
api:
  max_body_bytes: 1048576  # 1MB default
```
**Apply:** In `NewRouter()`, add `r.Use(BodySizeLimit(cfg.MaxBodyBytes))` BEFORE route handlers.
**Error response:** `{"error": "request body too large", "code": "BODY_TOO_LARGE"}` with 413 status.

## 4. Per-Account Session Limit

**File:** `internal/session/manager.go`
**Change:** In `Create()`, count active sessions for the requesting account (from billing context). Reject if over limit.
**Config:** Add `max_sessions_per_account` to config (default 10).
**Error:** Return 429 with `{"error": "account session limit exceeded", "code": "SESSION_LIMIT"}`.
**Note:** The global `maxConcurrent` stays as pool protection. Per-account is an additional check.
Add field to DefaultSessionManager: `maxPerAccount int`
Add to Config struct: `MaxPerAccount int`
In Create(): count sessions where session.AccountID == current account ID and status == "active".

## 5. Security Headers Middleware

**File:** NEW `internal/api/security_headers.go`
**Change:** Add middleware that sets:
```
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
X-XSS-Protection: 0
Referrer-Policy: strict-origin-when-cross-origin
Content-Security-Policy: default-src 'none'; frame-ancestors 'none'
Permissions-Policy: camera=(), microphone=(), geolocation=()
```
For `/website/*` paths, use a more permissive CSP:
```
Content-Security-Policy: default-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://cdn.tailwindcss.com; font-src 'self' https://fonts.gstatic.com; script-src 'self' 'unsafe-inline' https://cdn.tailwindcss.com; img-src 'self' data:; media-src 'self' https://*.cloudfront.net
```
**Apply:** In `NewRouter()`, add `r.Use(SecurityHeaders())` early in the chain.
If the request path starts with `/website/`, use the permissive CSP. Otherwise use the strict API CSP.

## 6. API Key from Environment Variable

**File:** `internal/config/config.go`
**Change:** Add explicit `viper.BindEnv("api.keys", "APERTURE_API_KEYS")` (comma-separated).
Also add `viper.BindEnv("llm.api_key", "APERTURE_LLM_API_KEY")` if not already there.
**Goal:** Users can set `APERTURE_API_KEYS=apt_key1,apt_key2` instead of putting keys in YAML.
**Note:** The LLM key already has env binding. Just need API keys.
Remove the plaintext `api_key` from the example YAML and add a comment pointing to env vars.

## 7. LLM Cost Guardrails

**File:** `internal/session/manager.go` (Execute method) + `internal/planner/` 
**Change:** Add two limits:
- `max_steps_per_task: 20` — Planner rejects plans with >20 steps
- `max_llm_calls_per_session: 50` — Session tracks total LLM API calls, rejects when exceeded
**Config:**
```yaml
llm:
  max_steps_per_task: 20
  max_calls_per_session: 50
```
**Implementation:**
- In planner: after generating plan, check step count. If > max, truncate and log warning.
- In session manager Execute(): track LLM calls via a counter on Session. Check before each LLM call.
- Add `LLMCallCount int` field to `domain.Session`.

## 8. Browser State Cleanup on Release

**File:** `internal/browser/pool.go` (Release method)
**Change:** Before putting browser back in available channel, navigate to `about:blank` and clear cookies/storage.
**Implementation:**
```go
func (p *Pool) Release(inst domain.BrowserInstance) {
    // Clean browser state before reuse
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    inst.Navigate(ctx, "about:blank")
    inst.ClearCookies(ctx)
    inst.ClearLocalStorage(ctx)
    p.available <- inst
}
```
**Note:** Need to add `ClearCookies(ctx)` and `ClearLocalStorage(ctx)` to BrowserInstance interface if not present. If the underlying chromedp doesn't support it directly, use `network.ClearBrowserCookies` and `runtime.Evaluate("localStorage.clear()")`.
Check what methods exist on the instance first. If Navigate/ClearCookies don't exist, add them.

## 9. Checkpoint TTL Cleanup

**File:** NEW `internal/checkpoint/cleaner.go` or in main.go startup
**Change:** On server startup, spawn a goroutine that every hour scans `checkpoint_dir` and deletes files older than 24h.
**Config:**
```yaml
checkpoint_ttl_hours: 24
```
**Implementation:**
```go
func StartCheckpointCleaner(dir string, ttl time.Duration, interval time.Duration) {
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        for range ticker.C {
            cleanCheckpoints(dir, ttl)
        }
    }()
}

func cleanCheckpoints(dir string, ttl time.Duration) {
    entries, _ := os.ReadDir(dir)
    cutoff := time.Now().Add(-ttl)
    for _, e := range entries {
        info, _ := e.Info()
        if info != nil && info.ModTime().Before(cutoff) {
            os.Remove(filepath.Join(dir, e.Name()))
        }
    }
}
```

## 10. Global Request Timeout

**File:** `internal/api/router.go`
**Change:** Add `middleware.Timeout(60 * time.Second)` from chi middleware.
**Config:** `api.request_timeout_seconds: 60`
**Note:** This is a backstop — individual handlers may have shorter timeouts. This prevents any single request from hanging the server forever.
IMPORTANT: chi's middleware.Timeout uses context cancellation. Long-running handlers (Execute, Task planner) already have their own timeouts. The global timeout should be generous enough (120s for API, separate for SSE streaming).
Actually — DON'T apply to SSE streaming endpoints (`/tasks/execute`, `/tasks/resume`, `/sessions/{id}/stream`). Those are long-lived by design.
Use chi route-level middleware: apply timeout only to non-streaming routes.

## Config Changes Summary

```yaml
api:
  rate_limit_rpm: 120          # NEW default (was 0)
  cors_origins: []             # existing
  cors_reject_unknown: false   # NEW (default false for backward compat)
  max_body_bytes: 1048576      # NEW (1MB)
  max_sessions_per_account: 10 # NEW
  request_timeout_seconds: 60  # NEW

llm:
  max_steps_per_task: 20       # NEW
  max_calls_per_session: 50    # NEW

checkpoint_ttl_hours: 24       # NEW
```

## Testing
After all changes:
1. `go build ./...` must succeed
2. `go vet ./...` must be clean
3. Existing 60/60 E2E tests must still pass (some may need minor adjustment for rate limiting — the test creates many sessions rapidly)
4. Add rate limit header check to E2E
5. Verify body size limit with oversized payload
6. Verify security headers present in responses

## Important Notes
- The E2E test script creates ~25 sessions during a full run. At 120 RPM that's fine.
- Rate limiting is per-IP, so localhost tests won't hit issues unless running >2 req/sec sustained.
- CORS changes should NOT break the E2E tests since curl doesn't send Origin headers.
- The `max_sessions_per_account` only applies when billing/auth is active. Dev mode (no auth) uses global limit only.
