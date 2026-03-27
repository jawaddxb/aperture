# Aperture API Reference

Base URL: `http://localhost:8080`

## Authentication

When `api.require_auth: true` in `aperture.yaml`, all `/api/v1/*` endpoints require:

```
Authorization: Bearer apt_your_api_key_here
```

Health (`/health`) and website (`/website/*`) are always public.

### Rate Limiting

When `api.rate_limit_rpm` is set, requests are throttled per client IP. Exceeding the limit returns `429 Too Many Requests` with `Retry-After: 60`.

---

## Endpoints

### Health

#### `GET /health`
Returns server status. Always public.

**Response:**
```json
{"status": "ok"}
```

#### `GET /api/v1/bridge/health`
Returns detailed health including browser pool and LLM status.

**Response:**
```json
{
  "status": "ok",
  "browser_pool": "available",
  "llm_client": "openai",
  "active_tasks": 0
}
```

---

### Screenshots

#### `POST /api/v1/actions/screenshot`
Capture a screenshot of any URL. Returns base64-encoded PNG.

**Request:**
```json
{
  "url": "https://news.ycombinator.com",
  "fullPage": false
}
```

**Response:**
```json
{
  "url": "https://news.ycombinator.com/",
  "data": "<base64-encoded-png>",
  "success": true
}
```

**Notes:**
- Stealth hardening is applied automatically
- Anti-bot detection triggers Scrapling fallback if needed
- Typical response: 200-700KB base64 for viewport screenshots

---

### Bridge Execute (LLM-Powered Tasks)

#### `POST /api/v1/bridge/execute?sync=true`
Execute a multi-step browser task using LLM planning.

**Request:**
```json
{
  "goal": "Go to Hacker News and extract the top 3 post titles",
  "url": "https://news.ycombinator.com",
  "config": {
    "timeout": 60
  }
}
```

**Response:**
```json
{
  "id": "uuid",
  "success": true,
  "goal": "...",
  "steps": [
    {
      "index": 0,
      "action": "navigate",
      "success": true,
      "duration_ms": 2100
    },
    {
      "index": 1,
      "action": "extract",
      "success": true,
      "duration_ms": 3200,
      "data": "[...]"
    }
  ],
  "final_url": "https://news.ycombinator.com/",
  "summary": "Extracted top 3 posts..."
}
```

**Async mode:** Omit `?sync=true` to get a task ID back immediately, then poll with `GET /api/v1/bridge/tasks/{id}`.

#### `GET /api/v1/bridge/tasks/{id}`
Get the status of an async task.

#### `DELETE /api/v1/bridge/tasks/{id}`
Cancel a running task.

#### `POST /api/v1/bridge/quick`
Execute a quick single-action task.

---

### Sessions

#### `POST /api/v1/sessions`
Create a new browser session with an optional starting URL.

**Request:**
```json
{
  "url": "https://example.com",
  "config": {
    "timeout": 30
  }
}
```

#### `GET /api/v1/sessions`
List all active sessions.

#### `GET /api/v1/sessions/{id}`
Get session details.

#### `POST /api/v1/sessions/{id}/execute`
Execute an action within an existing session (preserves cookies/state).

#### `DELETE /api/v1/sessions/{id}`
Close and destroy a session.

---

### Actions

#### `POST /api/v1/actions/execute`
Execute a single browser action (lower-level than bridge/execute).

---

### Metrics

#### `GET /api/v1/metrics`
Returns server metrics (when metrics collector is configured).

---

## Available Actions (for bridge/execute goals)

The LLM planner decomposes goals into these actions:

| Action | Required Params | Description |
|--------|----------------|-------------|
| `navigate` | `url:string` | Navigate to a URL |
| `click` | `target:string` | Click an element (CSS selector or label) |
| `type` | `target:string, text:string` | Type text into an input |
| `screenshot` | *(none)* | Capture current page as PNG |
| `scroll` | `direction:string, amount:int` | Scroll up/down by pixels |
| `select` | `target:string, value:string` | Select a dropdown option |
| `hover` | `target:string` | Hover over an element |
| `wait` | `strategy:string, value:string` | Wait for condition (selector/text/hidden/timeout/networkidle) |
| `extract` | `schema:string, format:string` | Extract structured data via LLM |

---

## Error Format

All errors return:
```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable description"
  }
}
```

Common codes:
- `UNAUTHORIZED` — Missing or invalid Authorization header
- `FORBIDDEN` — Invalid API key
- `RATE_LIMITED` — Too many requests (check Retry-After header)
- `NOT_CONFIGURED` — Feature not available (e.g., bridge without LLM key)
- `EXECUTE_FAILED` — Task execution error
