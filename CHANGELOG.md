# Changelog

All notable changes to Aperture are documented here.

## [2.0.0] — 2026-03-28

### Added
- **xBPP Policy Engine** — Cross-platform Behavior Policy Protocol with 7-check rule evaluator
  - Domain blocklist/allowlist with glob matching (`*.example.com`)
  - Action type allowlisting
  - Max actions per session enforcement
  - Sliding-window rate limiting (RPM)
  - Custom rule DSL (`domain == X AND action == Y`)
  - Financial transaction threshold escalation
  - CRUD API: `GET/PUT/DELETE /api/v1/policies/{agent_id}`
  - Sub-10ms evaluation latency, wired into session execution
- **Site Profiles** — YAML-defined domain intelligence for structured extraction
  - 3 built-in profiles: Amazon (product + search), Google (search), Shopify (product)
  - Auto-matching on navigate: URL pattern → profile → structured data + available actions
  - Embedded via Go `embed.FS` — zero filesystem access at runtime
  - Profile listing API: `GET /api/v1/profiles`
  - `PageState` enriched with `profile_matched`, `structured_data`, `available_actions`
- **Credential Vault** — AES-256-GCM encrypted per-agent credential storage
  - File-based storage at `~/.aperture/credentials/`
  - Key derivation: HKDF-SHA256 from `APERTURE_VAULT_KEY` env var or generated `vault.key`
  - Wildcard domain matching for credentials (`*.amazon.com`)
  - Auto-login: detects login forms, fills username/password, submits
  - CRUD API: `GET/PUT/DELETE /api/v1/agents/{agent_id}/credentials[/{domain}]`
  - Passwords never logged or returned in API responses
- **TypeScript SDK** (`@aperture/sdk` v0.1.0) — Fluent developer surface
  - `Aperture` → `ApertureSession` fluent API
  - Actions: `navigate`, `click`, `type`, `select`, `scroll`, `wait`, `extract`, `screenshot`, `close`
  - Zod schema validation on `extract()` for type-safe structured data
  - Typed error hierarchy: `PolicyBlockedError`, `DisambiguationError`, `BudgetExhaustedError`, `SessionExpiredError`
  - Zero external HTTP dependencies (native `fetch`)
  - 6 Jest tests with mocked fetch

### Changed
- `.gitignore` now uses `/aperture-server` to avoid ignoring `cmd/aperture-server/` directory
- `NavigateExecutor` uses functional options pattern for optional dependencies
- `session.Config` extended with `PolicyEngine` field
- `RouterConfig` extended with `PolicyEngine`, `ProfileManager`, `CredentialVault` fields

## [1.1.0] — 2026-03-27

### Added
- **LLM Planner** — OpenRouter (gpt-4o-mini) integration for multi-step browser task planning
- **Data Extraction** — Schema-based LLM extraction via `extract` MCP tool and Go executor
- **Scrapling Fallback** — Auto-bypass Cloudflare Turnstile, PerimeterX via Camoufox subprocess
- **Multi-Provider Anti-Bot Detection** — Cloudflare, PerimeterX, DataDome, Akamai auto-detection
- **Stealth Hardening (12 layers):**
  - Canvas fingerprint noise injection
  - WebRTC IP leak blocking
  - Viewport dimension randomization
  - Realistic browser plugin mocking
  - `chrome.runtime` object injection
  - Timezone/geolocation CDP spoofing
  - Bézier curve mouse humanization
  - Turnstile challenge detector
  - Hardened Chrome launch flags
- **API Key Authentication** — Bearer token middleware with configurable key prefix
- **CORS Middleware** — Configurable cross-origin request handling
- **Rate Limiting** — Token-bucket per-IP rate limiter
- **5 MCP Tools** — execute, screenshot, navigation, pool_status, extract
- **README.md** — Comprehensive documentation with architecture diagram
- **Landing Page** — Full product website at `/website/`
- **Anti-Bot Research** — `docs/ANTI-BOT-RESEARCH.md` with stealth bypass analysis

### Changed
- Planner system prompt now includes full action schema (extract + wait strategies)
- Health endpoint accurately reports LLM client status
- Screenshot endpoint returns JSON with base64 data (was raw PNG)
- Browser pool pre-warms with stealth config applied

### Fixed
- Double `/v1` in OpenRouter URL path causing 404
- PM2 ecosystem.config.js env vars overriding YAML config
- MCP client `full_page` field mismatch (snake_case vs camelCase)
- Fallback planner for server startup without LLM key

## [1.0.0] — 2026-03-27

### Added
- **Core Browser Engine** — CDP-based Chromium automation via `chromedp`
- **Browser Pool** — 5 pre-warmed instances with automatic recycling
- **10 Action Executors** — navigate, click, type, screenshot, scroll, hover, select, wait, file, tabs
- **AX Tree Resolution** — Accessibility tree-based element targeting
- **DOM Resolution** — CSS selector + semantic ID resolution
- **Session Management** — Create, execute, destroy browser sessions
- **LLM Client** — OpenAI and Anthropic provider support
- **Vision Analyzer** — Screenshot analysis via vision-capable LLMs
- **HITL Manager** — Human-in-the-loop intervention for CAPTCHAs
- **Auth Persistence** — Cookie save/restore across sessions
- **Proxy Support** — Static proxy provider with HTTP/SOCKS5
- **Metrics Collector** — Action timing and success rate tracking
- **Progress Streaming** — WebSocket-based task progress events
- **Error Recovery** — Automatic retry with exponential backoff
- **Input Validation** — Schema validation for all action params
- **MCP Server** — TypeScript stdio bridge with 4 tools
- **Makefile** — build, test, lint, run, docker targets
- **250+ Tests** — All 20 packages with race detection
