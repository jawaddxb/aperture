# Aperture v1.1 — Stealth Hardening

## Architecture

All stealth lives in `internal/browser/`. No new packages.

- `stealth.go` — `ApplyStealth()` orchestrator + CDP calls (viewport, timezone, geo)
- `stealth_scripts.go` — JS injection constants (canvas, WebRTC, plugins, chrome.runtime)
- `humanize.go` — Bézier curve mouse movement via CDP Input domain
- `turnstile.go` — Cloudflare Turnstile iframe detector + auto-wait
- `opts.go` — Chrome launch flags (anti-detection hardened)
- `domain/browser.go` — Expanded `StealthConfig` struct

## Detection Surface Coverage

| Vector | v1.0 | v1.1 |
|--------|------|------|
| navigator.webdriver | ✅ | ✅ |
| WebGL vendor/renderer | ✅ | ✅ |
| User-Agent | ✅ | ✅ |
| Canvas fingerprint | ❌ | ✅ |
| WebRTC IP leak | ❌ | ✅ |
| Viewport/screen | ❌ | ✅ |
| Plugin list | ❌ | ✅ |
| chrome.runtime | ❌ | ✅ |
| Timezone | ❌ | ✅ |
| Geolocation | ❌ | ✅ |
| Mouse entropy | ❌ | ✅ |
| Cloudflare Turnstile | ❌ | ✅ |
| Launch flags | ⚠️ | ✅ |
