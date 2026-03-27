# Aperture × OpenClaw Integration

**Phase 3 — MCP Server Integration**  
**Status:** Complete  
**Date:** 2026-03-27

---

## Architecture

```
OpenClaw CLI / Agent
        │
        │  MCP stdio transport (JSON-RPC 2.0)
        ▼
aperture-mcp (Node.js / TypeScript)
  packages/mcp-server/dist/index.js
        │
        │  HTTP REST API
        ▼
Aperture Go Server  :8080
        │
        │  CDP (Chrome DevTools Protocol)
        ▼
Chrome Browser Pool
```

### Components

| Component | Location | Role |
|-----------|----------|------|
| MCP Server | `packages/mcp-server/` | Translates MCP tool calls → Aperture HTTP API |
| Plugin Config | `~/.openclaw/plugins/aperture-mcp.json` | Registers MCP server with OpenClaw |
| Aperture Server | `cmd/aperture-server/` | Go HTTP API managing browser pool |
| Browser Pool | `internal/browser/` | CDP-connected Chrome instances |

---

## Setup

### 1. Start the Aperture Go server

```bash
# Copy and edit config
cp aperture.yaml.example aperture.yaml
# Set browser.chromium_path to your Chrome/Chromium binary

# Start server (default: http://localhost:8080)
go run ./cmd/aperture-server
```

### 2. Build the MCP server

```bash
cd packages/mcp-server
npm install
npm run build
```

### 3. Register the OpenClaw plugin

The plugin file is already created at `~/.openclaw/plugins/aperture-mcp.json`.

Verify it:

```bash
cat ~/.openclaw/plugins/aperture-mcp.json
```

Expected:
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

### 4. Verify with mcporter

```bash
mcporter list
# Should show: execute, screenshot, navigation, pool_status
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `APERTURE_BASE_URL` | `http://localhost:8080` | Aperture Go server URL |
| `APERTURE_POOL_SIZE` | `5` | Browser pool size hint |
| `APERTURE_TIMEOUT` | `30000` | Per-call timeout in milliseconds |

---

## Tool Reference

### `execute`

Run a browser automation task using the Aperture executor pipeline.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `goal` | string | ✅ | Natural-language task description |
| `url` | string | ❌ | Starting URL to navigate to |
| `timeout_seconds` | number | ❌ | Max execution time (default: 30, max: 300) |
| `screenshots` | boolean | ❌ | Capture screenshots at each step |

**Example:**
```json
{
  "name": "execute",
  "arguments": {
    "goal": "Find the price of the first product on the page",
    "url": "https://shop.example.com",
    "timeout_seconds": 60
  }
}
```

**Response:** JSON `TaskResponse` with `id`, `success`, `final_url`, `final_title`, and `steps[]`.

---

### `screenshot`

Capture a PNG screenshot of a URL.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `url` | string | ✅ | Page to screenshot |
| `full_page` | boolean | ❌ | Capture full scrollable height (default: false) |

**Example:**
```json
{
  "name": "screenshot",
  "arguments": {
    "url": "https://example.com",
    "full_page": true
  }
}
```

**Response:** `image` content item with base64 PNG + `text` item with the captured URL.

---

### `navigation`

Execute a browser navigation action on an existing session.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `action` | enum | ✅ | `navigate` \| `goBack` \| `goForward` \| `reload` |
| `session_id` | string | ✅ | Session ID from a prior `execute` call |
| `url` | string | ✅ for `navigate` | Target URL |

**Example:**
```json
{
  "name": "navigation",
  "arguments": {
    "action": "navigate",
    "url": "https://example.com/page2",
    "session_id": "task-id-from-execute"
  }
}
```

**Response:** JSON `TaskResponse` with navigation result.

---

### `pool_status`

Get the current health and availability of the Aperture browser pool.

**Parameters:** None

**Example:**
```json
{
  "name": "pool_status",
  "arguments": {}
}
```

**Response:**
```json
{
  "status": "ok",
  "browser_pool": "available",
  "llm_client": "not configured",
  "active_tasks": 2
}
```

---

## Testing

### Unit tests (MCP server)

```bash
cd packages/mcp-server
npm test
# 40 tests, 4 suites
```

### Smoke tests (no Aperture server required)

```bash
cd tests
npm test
# 3 smoke tests: binary check, startup message, tools/list
```

### Full E2E tests (requires Aperture server + Chrome)

```bash
# In one terminal:
go run ./cmd/aperture-server

# In another:
cd tests
APERTURE_E2E=1 npm test
# 5 E2E tests + 3 smoke tests = 8 total
```

---

## Troubleshooting

### Pool exhaustion

**Symptom:** `execute` returns `"bridge: concurrent task limit of N reached"`

**Fix:** Increase `APERTURE_POOL_SIZE` or wait for tasks to complete. The default pool is 5 browsers.

```json
"env": { "APERTURE_POOL_SIZE": "10" }
```

### Timeout errors

**Symptom:** Tasks return `"context deadline exceeded"`

**Fix:** Increase `APERTURE_TIMEOUT` (ms) for the MCP server and `timeout_seconds` per call.

```json
"env": { "APERTURE_TIMEOUT": "60000" }
```

Per-call override:
```json
{ "goal": "slow task", "timeout_seconds": 120 }
```

### Screenshot conflicts

The MCP `screenshot` tool and Aperture's step-level screenshots use different namespaces. If you need both, use `execute` with `screenshots: true` for step screenshots, and the `screenshot` tool for standalone captures.

### Server not reachable

```bash
# Verify Aperture is running
curl http://localhost:8080/health
# Expected: {"status":"ok"}

# Check MCP server can reach it
APERTURE_BASE_URL=http://localhost:8080 node packages/mcp-server/dist/index.js
# Expected stderr: Aperture MCP server started (base_url=http://localhost:8080)
```

### OpenClaw plugin not loading

Verify the plugin JSON is valid and the path to `index.js` is absolute:

```bash
node -e "JSON.parse(require('fs').readFileSync(process.env.HOME+'/.openclaw/plugins/aperture-mcp.json', 'utf8'))" && echo "valid JSON"
```

---

## Shutdown

The MCP server handles `SIGTERM` gracefully. OpenClaw will send `SIGTERM` on shutdown, allowing the Node.js process to exit cleanly. The Aperture Go server drains its browser pool on shutdown — do not `kill -9` it.
