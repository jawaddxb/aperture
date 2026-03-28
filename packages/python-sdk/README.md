# Aperture Python SDK

Python client for the [Aperture](https://github.com/jawaddxb/aperture) browser automation engine.

## Installation

```bash
pip install aperture-sdk
```

## Quick Start

```python
from aperture import ApertureClient

client = ApertureClient(
    api_key="apt_your_key_here",
    base_url="http://localhost:8080"
)

# Simple session — navigate and extract
with client.session("Navigate to https://example.com") as s:
    result = s.execute()
    print(f"Success: {result.success}")
    snap = s.snapshot()
    print(f"Title: {snap.title}")
```

## Task Planner (SSE Streaming)

```python
# Stream real-time progress from the LLM task planner
for event in client.task("Get the #1 story title from Hacker News"):
    if event.event == "progress":
        print(f"  Step: {event.data.get('message')}")
    elif event.event == "complete":
        print(f"  Result: {event.data}")
```

## Async Support

```python
import asyncio
from aperture import ApertureClient

async def main():
    client = ApertureClient(api_key="apt_...", base_url="http://localhost:8080")
    
    async with client.session("Navigate to example.com") as s:
        result = await s.aexecute()
        print(result.success)
    
    await client.aclose()

asyncio.run(main())
```

## Credential Vault

```python
# Store credentials for auto-login
client.store_credential("my-agent", "linkedin.com", "user@email.com", "password123")

# List stored credentials (passwords never returned)
creds = client.list_credentials("my-agent")
for c in creds:
    print(f"{c.domain}: {c.username}")
```

## xBPP Policy Engine

```python
from aperture.types import PolicyConfig

# Set agent governance rules
client.set_policy("my-agent", PolicyConfig(
    blocked_domains=["facebook.com", "twitter.com"],
    allowed_actions=["navigate", "extract", "screenshot"],
    max_actions=20,
    rate_limit_rpm=30,
))

# Check policy
policy = client.get_policy("my-agent")
print(policy)
```

## Agent Memory (KV Store)

```python
# Store and retrieve agent state across sessions
client.set_memory("my-agent", "last_url", "https://example.com")
client.set_memory("my-agent", "preferences", {"theme": "dark", "lang": "en"})

url = client.get_memory("my-agent", "last_url")
all_keys = client.list_memory("my-agent")
```

## Session Import (Authenticated Sessions)

```python
from aperture.types import ImportConfig

# Import browser cookies for authenticated access
result = client.import_session(ImportConfig(
    cookies=[
        {"name": "li_at", "value": "AQ...", "domain": ".linkedin.com", "path": "/"},
    ],
    trust_mode="preserve",  # Skip fingerprint randomization
    source_url="https://www.linkedin.com",
))
print(f"Session: {result.session_id}, Cookies: {result.cookies_imported}")
```

## Site Profiles

```python
# List available extraction profiles
profiles = client.list_profiles()
for p in profiles:
    print(f"{p.domain} (v{p.version})")
# *.amazon.com, *.linkedin.com, *.github.com, ... (20 total)
```

## Error Handling

```python
from aperture import ApertureClient
from aperture.errors import (
    AuthenticationError,
    RateLimitError,
    SessionNotFoundError,
    PolicyBlockedError,
    BudgetExhaustedError,
)

try:
    with client.session("Navigate to blocked-site.com") as s:
        result = s.execute()
except PolicyBlockedError as e:
    print(f"Blocked by xBPP: {e}")
except RateLimitError:
    print("Too many requests — back off")
except BudgetExhaustedError:
    print("Out of credits")
except AuthenticationError:
    print("Invalid API key")
```

## API Reference

### `ApertureClient`

| Method | Description |
|--------|-------------|
| `session(goal)` | Context manager for session lifecycle |
| `create_session(goal)` | Create a session |
| `execute_session(id)` | Execute a session's goal |
| `snapshot(id)` | Get page snapshot (URL, title) |
| `delete_session(id)` | Delete session + release browser |
| `import_session(config)` | Import authenticated session |
| `task(goal)` | SSE streaming task planner |
| `get_policy(agent_id)` | Get xBPP policy |
| `set_policy(agent_id, config)` | Set xBPP policy |
| `store_credential(...)` | Store vault credential |
| `list_credentials(agent_id)` | List credentials |
| `set_memory(agent_id, key, val)` | Set KV pair |
| `get_memory(agent_id, key)` | Get KV value |
| `list_profiles()` | List site profiles |
| `health()` | Health check |

### Async variants

All methods have `a`-prefixed async variants: `acreate_session`, `aexecute_session`, `adelete_session`.

## License

MIT
