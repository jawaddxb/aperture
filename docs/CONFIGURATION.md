# Aperture Configuration Reference

Aperture reads configuration from `aperture.yaml` in the working directory. All values can be overridden via environment variables with the `APERTURE_` prefix.

## Example Configuration

```yaml
server:
  host: "0.0.0.0"
  port: 8080

browser:
  pool_size: 5
  chromium_path: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
  proxy_url: ""              # Optional: "http://user:pass@proxy:8080"

llm:
  provider: "openai"         # "openai" or "anthropic"
  model: "openai/gpt-4o-mini"
  api_key: ""                # Your OpenRouter/OpenAI/Anthropic key
  base_url: "https://openrouter.ai/api"  # Override for OpenRouter

api:
  key_prefix: "apt_"         # Expected API key prefix
  require_auth: false         # Set true for production
  keys:                       # List of valid API keys
    - "apt_your_key_here"
  rate_limit_rpm: 0           # Requests per minute per IP (0 = unlimited)
  cors_origins: []            # Allowed CORS origins (empty = allow all)

bridge:
  max_concurrent_tasks: 10
  task_timeout_seconds: 120

stealth:
  enabled: true
  hide_webdriver: true
  canvas_noise: true
  block_webrtc: true
  random_viewport: true
  mock_plugins: true
  timezone: ""               # Override: "America/New_York"
  geo_latitude: 0            # Override: 40.7128
  geo_longitude: 0           # Override: -74.0060

log:
  level: "info"              # "debug", "info", "warn", "error"
```

## Field Reference

### server

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | `0.0.0.0` | Bind address |
| `port` | int | `8080` | Listen port |

### browser

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `pool_size` | int | `5` | Number of pre-warmed Chromium instances |
| `chromium_path` | string | *required* | Path to Chrome/Chromium binary |
| `proxy_url` | string | `""` | HTTP/SOCKS5 proxy for all browser traffic |
| `skip_pre_warm` | bool | `false` | Skip pool pre-warming (for tests) |

### llm

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `provider` | string | `openai` | LLM backend: `openai` or `anthropic` |
| `model` | string | `gpt-4o` | Model ID (use `openai/gpt-4o-mini` for OpenRouter) |
| `api_key` | string | `""` | Provider API key (required for planning/extraction) |
| `base_url` | string | `""` | Override API endpoint (e.g., `https://openrouter.ai/api`) |

**OpenRouter note:** Set `base_url` to `https://openrouter.ai/api` (not `/api/v1` — the client appends `/v1/chat/completions` automatically).

### api

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `key_prefix` | string | `apt_` | Expected API key prefix |
| `require_auth` | bool | `false` | Require API key for `/api/v1/*` routes |
| `keys` | []string | `[]` | List of valid API keys |
| `rate_limit_rpm` | int | `0` | Max requests per minute per IP (0 = unlimited) |
| `cors_origins` | []string | `[]` | Allowed CORS origins (empty = allow all) |

### bridge

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_concurrent_tasks` | int | `10` | Max simultaneous bridge tasks |
| `task_timeout_seconds` | int | `120` | Default per-task timeout |

### stealth

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable stealth hardening |
| `hide_webdriver` | bool | `true` | Remove navigator.webdriver flag |
| `canvas_noise` | bool | `true` | Inject canvas fingerprint noise |
| `block_webrtc` | bool | `true` | Block WebRTC IP leaks |
| `random_viewport` | bool | `true` | Randomize viewport dimensions |
| `mock_plugins` | bool | `true` | Inject realistic browser plugins |
| `timezone` | string | `""` | Override timezone (e.g., `America/New_York`) |
| `geo_latitude` | float | `0` | Override geolocation latitude |
| `geo_longitude` | float | `0` | Override geolocation longitude |

### log

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `level` | string | `info` | Log level: `debug`, `info`, `warn`, `error` |

## Environment Variables

All config fields can be set via environment variables:

```bash
APERTURE_SERVER_PORT=8080
APERTURE_BROWSER_CHROMIUM_PATH=/usr/bin/chromium
APERTURE_LLM_API_KEY=sk-xxx
APERTURE_API_REQUIRE_AUTH=true
```

**Note:** Viper's env binding works for top-level and simple nested fields. For deeply nested struct fields (like `llm.base_url`), use the YAML file instead.
