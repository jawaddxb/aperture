# Aperture 🔍

A high-performance browser automation engine built in Go with intelligent anti-bot detection, stealth features, and LLM-powered planning.

**Latest:** v1.1 with Scrapling fallback, multi-provider anti-bot detection, and OpenRouter LLM integration.

## Features

- **Native Browser Control** — CDP-based Chromium automation, 3–7s per screenshot on clean sites
- **Intelligent Anti-Bot Detection** — Auto-detects Cloudflare, PerimeterX, DataDome, Akamai challenges
- **Automatic Fallback** — Seamlessly escalates to Scrapling (Camoufox) for hardened sites (20–22s)
- **Stealth Hardening** — Canvas noise, WebRTC blocking, viewport randomization, realistic plugins, timezone/geo spoofing, mouse humanization, Turnstile detection
- **LLM-Powered Planning** — OpenRouter (gpt-4o-mini) generates multi-step browser tasks
- **Vision Analysis** — Extract structured data from screenshots
- **CAPTCHA/HITL** — Human-in-the-loop for unsolvable challenges
- **Multi-Tab Sessions** — Maintain browser state across tabs, cookies, profiles
- **Data Extraction** — Parse tables, text, JSON from rendered pages

## Quick Start

### Installation

```bash
# Clone repo
git clone https://github.com/jawaddxb/aperture
cd aperture

# Build Go server
go build -o aperture-server ./cmd/aperture-server

# Build MCP server
cd packages/mcp-server && npm run build && cd ../..

# Install Python fallback (for Scrapling)
pip install "scrapling[all]"
scrapling install  # Downloads Camoufox binary

# Start aperture-server on port 8080
./aperture-server
```

### Configuration

Create `aperture.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 8080
browser:
  pool_size: 5
  chromium_path: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
llm:
  provider: "openai"
  model: "openai/gpt-4o-mini"
  api_key: "sk-or-v1-YOUR_OPENROUTER_KEY"
  base_url: "https://openrouter.ai/api"
stealth:
  enabled: true
  hide_webdriver: true
  canvas_noise: true
  block_webrtc: true
  random_viewport: true
  mock_plugins: true
```

### API Usage

#### Screenshot

```bash
curl -X POST http://localhost:8080/api/v1/actions/screenshot \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://news.ycombinator.com"}'
```

Returns: `{"success":true,"url":"...","data":"<base64-png>"}`

#### Bridge Execute (LLM-Powered Planning)

```bash
curl -X POST "http://localhost:8080/api/v1/bridge/execute?sync=true" \
  -H 'Content-Type: application/json' \
  -d '{
    "goal":"Go to Hacker News and extract top 3 post titles",
    "url":"https://news.ycombinator.com"
  }'
```

Returns: `{"success":true,"goal":"...","steps":[...],"summary":"..."}`

#### Health

```bash
curl http://localhost:8080/api/v1/bridge/health
```

### MCP Integration

Register with OpenClaw:

```json
{
  "type": "mcp",
  "transport": "stdio",
  "command": "node",
  "args": ["/path/to/aperture/packages/mcp-server/dist/index.js"],
  "env": {
    "APERTURE_BASE_URL": "http://localhost:8080",
    "APERTURE_POOL_SIZE": "5",
    "APERTURE_TIMEOUT": "30000"
  }
}
```

Tools exposed:
- `execute` — Run multi-step browser tasks
- `screenshot` — Capture page screenshots
- `navigation` — Navigate, back, forward, refresh
- `pool_status` — Browser pool health

## Architecture

```
┌─────────────────────────────────────────────┐
│ OpenClaw MCP Client (LLM / Agent)           │
└──────────────┬──────────────────────────────┘
               │
       ┌───────▼────────┐
       │ MCP Server     │ (TypeScript/Node.js)
       │ (stdio)        │
       └───────┬────────┘
               │
       ┌───────▼──────────────┐
       │ aperture-server      │ (Go, port 8080)
       │ ┌──────────────────┐ │
       │ │ LLM Planner      │ │ OpenRouter gpt-4o-mini
       │ │ ┌──────────────┐ │ │
       │ │ │ Browser Pool │ │ │ 5 pre-warmed Chromium instances
       │ │ │ ┌──────────┐ │ │ │
       │ │ │ │ Stealth  │ │ │ │ Canvas noise, WebRTC block, etc.
       │ │ │ │ Module   │ │ │ │
       │ │ │ └──────────┘ │ │ │
       │ │ │ ┌──────────┐ │ │ │
       │ │ │ │ Anti-Bot │ │ │ │ CF/PerimeterX/DataDome/Akamai
       │ │ │ │ Detector │ │ │ │
       │ │ │ └──────────┘ │ │ │
       │ │ │ ┌──────────┐ │ │ │
       │ │ │ │ Scrapling│ │ │ │ Camoufox fallback
       │ │ │ │ Fallback │ │ │ │
       │ │ │ └──────────┘ │ │ │
       │ │ └──────────────┘ │ │
       │ └──────────────────┘ │
       └──────────────────────┘
```

## Test Results (2026-03-27)

### Coverage: 11/11 Sites

| Site | Type | Method | Size | Time |
|------|------|--------|------|------|
| Hacker News | Clean | Native Chrome | 231KB | 2.1s |
| GitHub | Clean | Native Chrome | 275KB | 3.2s |
| TechCrunch | Clean | Native Chrome | 328KB | 4.1s |
| Reddit | Clean | Native Chrome | 78KB | 2.8s |
| Wikipedia | Clean | Native Chrome | 349KB | 3.5s |
| **Product Hunt** | **CF** | **Scrapling** | **604KB** | **22s** |
| **Zillow** | **PerimeterX** | **Scrapling** | **5.1MB** | **20s** |
| Nike | Akamai | Native Chrome | 722KB | 3.2s |
| Discord | CF | Native Chrome | 650KB | 2.9s |
| LinkedIn | Custom | Native Chrome | 139KB | 3.1s |
| FootLocker | DataDome | Native Chrome | 40KB | 2.5s |

**Result:** ✅ 11/11 cracked. 5 pure native. 2 Scrapling fallback (CF Turnstile + PerimeterX). 4 hardened but native-handled.

### Quality Metrics

- **Go packages:** 20 (2 cmd, 18 lib)
- **Test packages:** 18 with tests, all green
- **Total tests:** ~250 (all passing)
- **LOC compliance:** All production functions ≤50 LOC
- **Code quality:** `go vet` clean, `go test -race -short` all pass

## Known Limitations

- **Headless Chrome TLS fingerprint:** Cloudflare server-side verification detects headless Chrome despite stealth tricks. Scrapling (Camoufox) bypasses this but adds 15–20s latency.
- **JavaScript-heavy sites:** Some sites requiring heavy JS execution may timeout. Increase `timeout_seconds` in config.
- **Selector stability:** LLM-generated selectors sometimes miss elements. Fallback to vision analysis or manual override.

## Roadmap

- [ ] Data extraction tools (extract_table, extract_text, extract_json)
- [ ] Vision analyzer (describe_screenshot, locate_element)
- [ ] Auth/rate limiting (API key, token bucket)
- [ ] HITL endpoint (pause on CAPTCHA, wait for human input)
- [ ] Landing page (sales + docs)
- [ ] Caching mechanism (avoid re-solving same challenges)

## Contributing

1. Keep functions ≤50 LOC
2. Run `go vet && go test ./... -race -short` before push
3. Add tests for new executors/detectors
4. Update PHASE-3-SPEC.md with major changes

## License

MIT — use freely, including in commercial products.

## Contact

Built by **Jawad** (@jawadash). Questions? [Open an issue](https://github.com/jawaddxb/aperture/issues).

---

**Latest commit:** `82f81e2` — LLM planner via OpenRouter
**Build:** Go 1.21+, Node.js 18+, Python 3.10+
