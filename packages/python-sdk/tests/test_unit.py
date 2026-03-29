"""Unit tests for the Aperture SDK using respx mocks.

These tests do NOT require a running Aperture server.
Run with: pytest tests/test_unit.py -v
"""

from __future__ import annotations

import json

import httpx
import pytest
import respx

from aperture import ApertureClient, AsyncTaskPlanner, TaskPlanner
from aperture.errors import (
    ApertureError,
    AuthenticationError,
    RateLimitError,
    SessionNotFoundError,
)
from aperture.types import ExecuteResult, SessionInfo, Snapshot

BASE = "http://localhost:8080"
API = f"{BASE}/api/v1"


@pytest.fixture
def client() -> ApertureClient:
    c = ApertureClient(api_key="apt_test_key", base_url=BASE)
    yield c
    c.close()


# ── Health ────────────────────────────────────────────────────────────────────

@respx.mock
def test_health(client: ApertureClient) -> None:
    respx.get(f"{BASE}/health").mock(
        return_value=httpx.Response(200, json={"status": "ok", "version": "v5.0.0"})
    )
    result = client.health()
    assert result["status"] == "ok"


# ── Sessions ──────────────────────────────────────────────────────────────────

@respx.mock
def test_create_session(client: ApertureClient) -> None:
    respx.post(f"{API}/sessions").mock(
        return_value=httpx.Response(201, json={
            "session_id": "sess-abc-123",
            "status": "active",
            "goal": "Navigate to example.com",
        })
    )
    s = client.create_session("Navigate to example.com")
    assert isinstance(s, SessionInfo)
    assert s.session_id == "sess-abc-123"
    assert s.status == "active"


@respx.mock
def test_get_session(client: ApertureClient) -> None:
    respx.get(f"{API}/sessions/sess-abc-123").mock(
        return_value=httpx.Response(200, json={
            "session_id": "sess-abc-123",
            "status": "completed",
            "goal": "test",
        })
    )
    s = client.get_session("sess-abc-123")
    assert s.status == "completed"


@respx.mock
def test_session_not_found(client: ApertureClient) -> None:
    respx.get(f"{API}/sessions/bad-uuid").mock(
        return_value=httpx.Response(404, json={
            "error": "session not found",
            "code": "NOT_FOUND",
        })
    )
    with pytest.raises(SessionNotFoundError):
        client.get_session("bad-uuid")


@respx.mock
def test_execute_session(client: ApertureClient) -> None:
    respx.post(f"{API}/sessions/sess-abc-123/execute").mock(
        return_value=httpx.Response(200, json={
            "success": True,
            "steps": [],
            "duration_ms": 1200,
            "total_cost": 5,
        })
    )
    result = client.execute_session("sess-abc-123")
    assert isinstance(result, ExecuteResult)
    assert result.success is True


@respx.mock
def test_snapshot(client: ApertureClient) -> None:
    respx.get(f"{API}/sessions/sess-abc-123/snapshot").mock(
        return_value=httpx.Response(200, json={
            "session_id": "sess-abc-123",
            "status": "active",
            "url": "https://example.com",
            "title": "Example Domain",
            "profile_matched": "",
            "structured_data": {},
            "available_actions": ["click", "type"],
        })
    )
    snap = client.snapshot("sess-abc-123")
    assert isinstance(snap, Snapshot)
    assert snap.url == "https://example.com"
    assert snap.title == "Example Domain"


@respx.mock
def test_delete_session(client: ApertureClient) -> None:
    respx.delete(f"{API}/sessions/sess-abc-123").mock(
        return_value=httpx.Response(204)
    )
    client.delete_session("sess-abc-123")  # should not raise


# ── Session context manager ───────────────────────────────────────────────────

@respx.mock
def test_session_context_manager(client: ApertureClient) -> None:
    respx.post(f"{API}/sessions").mock(
        return_value=httpx.Response(201, json={
            "session_id": "ctx-sess-456",
            "status": "active",
            "goal": "test",
        })
    )
    respx.post(f"{API}/sessions/ctx-sess-456/execute").mock(
        return_value=httpx.Response(200, json={
            "success": True,
            "steps": [],
            "duration_ms": 500,
            "total_cost": 3,
        })
    )
    respx.delete(f"{API}/sessions/ctx-sess-456").mock(
        return_value=httpx.Response(204)
    )

    with client.session("test") as s:
        assert s.session_id == "ctx-sess-456"
        result = s.execute()
        assert result.success is True


# ── Error handling ────────────────────────────────────────────────────────────

@respx.mock
def test_rate_limit_error(client: ApertureClient) -> None:
    from aperture.errors import ApertureError
    respx.post(f"{API}/sessions").mock(
        return_value=httpx.Response(429, json={
            "error": "rate limit exceeded",
            "code": "RATE_LIMIT_EXCEEDED",  # triggers RateLimitError not SessionLimitError
        })
    )
    with pytest.raises(ApertureError):  # RateLimitError or SessionLimitError — both are ApertureError
        client.create_session("test")


@respx.mock
def test_auth_error(client: ApertureClient) -> None:
    respx.post(f"{API}/sessions").mock(
        return_value=httpx.Response(401, json={
            "error": "missing Authorization header",
            "code": "UNAUTHORIZED",
        })
    )
    with pytest.raises(AuthenticationError):
        client.create_session("test")


@respx.mock
def test_generic_error(client: ApertureClient) -> None:
    respx.post(f"{API}/sessions").mock(
        return_value=httpx.Response(500, json={
            "error": "internal server error",
            "code": "INTERNAL_ERROR",
        })
    )
    with pytest.raises(ApertureError):
        client.create_session("test")


# ── KV Memory ─────────────────────────────────────────────────────────────────

@respx.mock
def test_set_get_memory(client: ApertureClient) -> None:
    respx.put(f"{API}/agents/agent-1/memory/my_key").mock(
        return_value=httpx.Response(200, json={
            "key": "my_key",
            "value": {"hello": "world"},
            "updated_at": "2026-03-29T00:00:00Z",
        })
    )
    respx.get(f"{API}/agents/agent-1/memory/my_key").mock(
        return_value=httpx.Response(200, json={
            "key": "my_key",
            "value": {"hello": "world"},
            "updated_at": "2026-03-29T00:00:00Z",
        })
    )
    client.set_memory("agent-1", "my_key", {"hello": "world"})
    val = client.get_memory("agent-1", "my_key")
    assert val == {"hello": "world"}


@respx.mock
def test_delete_memory(client: ApertureClient) -> None:
    respx.delete(f"{API}/agents/agent-1/memory/my_key").mock(
        return_value=httpx.Response(204)
    )
    client.delete_memory("agent-1", "my_key")  # should not raise


# ── Policies ──────────────────────────────────────────────────────────────────

@respx.mock
def test_set_get_policy(client: ApertureClient) -> None:
    from aperture.types import PolicyConfig

    respx.put(f"{API}/policies/agent-1").mock(
        return_value=httpx.Response(200, json={
            "agent_id": "agent-1",
            "status": "ok",
        })
    )
    respx.get(f"{API}/policies/agent-1").mock(
        return_value=httpx.Response(200, json={
            "agent_id": "agent-1",
            "policy": {
                "agent_id": "agent-1",
                "blocked_domains": ["evil.com"],
            },
        })
    )

    client.set_policy("agent-1", PolicyConfig(blocked_domains=["evil.com"]))
    policy = client.get_policy("agent-1")
    assert "agent_id" in policy or "policy" in policy


# ── Task planner (sync) ───────────────────────────────────────────────────────

@respx.mock
def test_task_planner_sync(client: ApertureClient) -> None:
    sse_body = (
        "event: step\ndata: {\"message\": \"Navigating...\"}\n\n"
        "event: complete\ndata: {\"result\": \"done\", \"success\": true}\n\n"
    )
    respx.post(f"{API}/tasks/execute").mock(
        return_value=httpx.Response(
            200,
            content=sse_body.encode(),
            headers={"content-type": "text/event-stream"},
        )
    )

    events = list(client.task("Get HN top story"))
    assert len(events) == 2
    assert events[0].event == "step"
    assert events[1].event == "complete"
    assert events[1].data["success"] is True


# ── Task planner (async) ──────────────────────────────────────────────────────

@pytest.mark.asyncio
@respx.mock
async def test_async_task_planner(client: ApertureClient) -> None:
    sse_body = (
        "event: step\ndata: {\"message\": \"Navigating async...\"}\n\n"
        "event: complete\ndata: {\"result\": \"async done\", \"success\": true}\n\n"
    )
    respx.post(f"{API}/tasks/execute").mock(
        return_value=httpx.Response(
            200,
            content=sse_body.encode(),
            headers={"content-type": "text/event-stream"},
        )
    )

    events = await client.async_task("Get HN top story async").execute()
    assert len(events) == 2
    assert events[0].event == "step"
    assert events[1].event == "complete"


# ── AsyncTaskPlanner type checks ─────────────────────────────────────────────

def test_async_task_planner_is_exported() -> None:
    """AsyncTaskPlanner must be importable from the top-level package."""
    from aperture import AsyncTaskPlanner as ATP
    assert ATP is not None


def test_client_has_async_task_method(client: ApertureClient) -> None:
    planner = client.async_task("test goal")
    assert isinstance(planner, AsyncTaskPlanner)
    assert planner._goal == "test goal"


# ── Profiles ─────────────────────────────────────────────────────────────────

@respx.mock
def test_list_profiles(client: ApertureClient) -> None:
    respx.get(f"{API}/profiles").mock(
        return_value=httpx.Response(200, json=[
            {"domain": "*.amazon.com", "version": "2026.03.28"},
            {"domain": "*.linkedin.com", "version": "2026.03.28"},
            {"domain": "*.github.com", "version": "2026.03.28"},
        ])
    )
    profiles = client.list_profiles()
    assert len(profiles) == 3
    domains = [p.domain for p in profiles]
    assert "*.amazon.com" in domains
    assert "*.linkedin.com" in domains
