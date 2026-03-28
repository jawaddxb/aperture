"""Integration tests for the Aperture Python SDK.

These tests require a running Aperture server at http://localhost:8080.
Run with: pytest tests/ -v
"""

import os
import sys

import pytest

# Add parent to path for local dev
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from aperture import ApertureClient
from aperture.errors import ApertureError, SessionNotFoundError
from aperture.types import PolicyConfig


BASE_URL = os.environ.get("APERTURE_URL", "http://localhost:8080")
API_KEY = os.environ.get("APERTURE_API_KEY", "")


@pytest.fixture
def client():
    c = ApertureClient(api_key=API_KEY or None, base_url=BASE_URL)
    yield c
    c.close()


class TestHealth:
    def test_health_check(self, client: ApertureClient):
        result = client.health()
        assert result["status"] == "ok"


class TestProfiles:
    def test_list_profiles(self, client: ApertureClient):
        profiles = client.list_profiles()
        assert len(profiles) >= 20
        domains = [p.domain for p in profiles]
        assert "*.amazon.com" in domains
        assert "*.linkedin.com" in domains
        assert "*.github.com" in domains


class TestSessionLifecycle:
    def test_create_execute_delete(self, client: ApertureClient):
        """Full session lifecycle: create → execute → snapshot → delete."""
        with client.session("Navigate to https://example.com") as s:
            assert s.session_id
            result = s.execute()
            assert result.success is True
            snap = s.snapshot()
            assert "example.com" in (snap.url or "")

    def test_session_not_found(self, client: ApertureClient):
        with pytest.raises(SessionNotFoundError):
            client.get_session("nonexistent-uuid")


class TestKVStore:
    def test_set_get_delete(self, client: ApertureClient):
        agent = "test-agent-python"
        client.set_memory(agent, "test_key", {"hello": "world"})
        val = client.get_memory(agent, "test_key")
        assert val is not None
        client.delete_memory(agent, "test_key")


class TestPolicies:
    def test_set_get_delete_policy(self, client: ApertureClient):
        agent = "test-agent-python"
        client.set_policy(agent, PolicyConfig(
            blocked_domains=["evil.com"],
            max_actions=10,
        ))
        policy = client.get_policy(agent)
        assert policy is not None
        client.delete_policy(agent)


class TestTaskPlanner:
    def test_stream_events(self, client: ApertureClient):
        events = []
        for event in client.task("Get the title of https://example.com"):
            events.append(event)
        assert len(events) > 0
        # Should have at least a complete event
        event_types = [e.event for e in events]
        assert "complete" in event_types or "message" in event_types


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
