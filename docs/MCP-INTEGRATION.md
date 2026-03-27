# Aperture MCP Integration Guide

Aperture exposes 5 tools via the Model Context Protocol (MCP) for AI agent integration.

## Setup

### 1. Build the MCP Server

```bash
cd packages/mcp-server
npm install
npm run build
```

### 2. Register with OpenClaw

Create `~/.openclaw/plugins/aperture-mcp.json`:

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

### 3. Register with mcporter (alternative)

```bash
mcporter add aperture \
  --command "node /path/to/aperture/packages/mcp-server/dist/index.js" \
  --env APERTURE_BASE_URL=http://localhost:8080
```

### 4. Verify

```bash
# Test tools/list handshake
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | \
  APERTURE_BASE_URL=http://localhost:8080 \
  node packages/mcp-server/dist/index.js
```

## Available Tools

### `execute`
Run a multi-step browser task using LLM planning.

```json
{
  "tool": "execute",
  "arguments": {
    "goal": "Go to GitHub trending and list the top 5 repos",
    "url": "https://github.com/trending",
    "timeout_seconds": 60,
    "screenshots": false
  }
}
```

### `screenshot`
Capture a screenshot of any URL.

```json
{
  "tool": "screenshot",
  "arguments": {
    "url": "https://producthunt.com",
    "full_page": false
  }
}
```

Returns an MCP image content block (base64 PNG).

### `extract`
Extract structured data from a web page.

```json
{
  "tool": "extract",
  "arguments": {
    "url": "https://news.ycombinator.com",
    "schema": "Top 5 posts with title, score, author, and comment count",
    "format": "json",
    "selector": ".itemlist"
  }
}
```

### `navigation`
Execute browser navigation actions within a session.

```json
{
  "tool": "navigation",
  "arguments": {
    "action": "navigate",
    "url": "https://example.com",
    "session_id": "session-uuid-from-execute"
  }
}
```

Actions: `navigate`, `goBack`, `goForward`, `reload`.

### `pool_status`
Check browser pool health.

```json
{
  "tool": "pool_status",
  "arguments": {}
}
```

## Anti-Bot Behavior

Aperture automatically handles anti-bot detection:

1. **Clean sites** (HN, GitHub, Wikipedia) → Native Chrome, 3-7 seconds
2. **Cloudflare Turnstile** → 4s wait, then Scrapling/Camoufox fallback (~22s)
3. **PerimeterX** → Auto-detect, Scrapling fallback
4. **DataDome** → Auto-detect, native Chrome usually sufficient
5. **Akamai** → Auto-detect, native Chrome usually sufficient

The agent never needs to know which method was used — Aperture decides internally.

## Example: Claude Using Aperture

When registered as an MCP tool, Claude can use Aperture naturally:

> **User:** "What are the top posts on Hacker News right now?"
>
> **Claude:** *calls `extract` tool with url="https://news.ycombinator.com" and schema="top 5 posts with title, score, and author"*

The agent sees structured JSON data, never raw HTML.

## Troubleshooting

| Issue | Fix |
|-------|-----|
| "Connection refused" | Ensure `aperture-server` is running on the configured port |
| "Tool not found" | Rebuild MCP server: `cd packages/mcp-server && npm run build` |
| "APERTURE_BASE_URL not set" | Pass env var to the MCP server process |
| Screenshot returns empty | Check `browser.chromium_path` in `aperture.yaml` |
| Cloudflare still blocking | Ensure Scrapling is installed: `pip install "scrapling[all]"` |
