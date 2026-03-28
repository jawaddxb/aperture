"""Core HTTP client for the Aperture API."""

from __future__ import annotations

from typing import Any, Optional

import httpx

from aperture.errors import raise_for_status
from aperture.session import ApertureSession
from aperture.task import TaskPlanner
from aperture.types import (
    AccountInfo,
    Credential,
    ExecuteResult,
    ImportConfig,
    ImportResult,
    PolicyConfig,
    SessionInfo,
    SiteProfile,
    Snapshot,
)


class ApertureClient:
    """Synchronous + async client for the Aperture browser automation API.

    Usage::

        from aperture import ApertureClient

        client = ApertureClient(api_key="apt_...", base_url="http://localhost:8080")

        # Create and execute a session
        with client.session("Navigate to example.com") as s:
            result = s.execute()
            print(result.success)

        # Or use the task planner
        for event in client.task("Get HN #1 story title"):
            print(event)
    """

    def __init__(
        self,
        api_key: Optional[str] = None,
        base_url: str = "http://localhost:8080",
        timeout: float = 120.0,
    ):
        self.base_url = base_url.rstrip("/")
        self.api_url = f"{self.base_url}/api/v1"
        headers: dict[str, str] = {"Content-Type": "application/json"}
        if api_key:
            headers["Authorization"] = f"Bearer {api_key}"
        self._sync = httpx.Client(base_url=self.api_url, headers=headers, timeout=timeout)
        self._async = httpx.AsyncClient(base_url=self.api_url, headers=headers, timeout=timeout)

    def close(self) -> None:
        """Close underlying HTTP connections."""
        self._sync.close()

    async def aclose(self) -> None:
        """Close underlying async HTTP connections."""
        await self._async.aclose()

    # ── Helpers ──────────────────────────────────────────────────────────

    def _request(self, method: str, path: str, **kwargs: Any) -> dict[str, Any]:
        resp = self._sync.request(method, path, **kwargs)
        if resp.status_code == 204:
            return {}
        body = resp.json() if resp.content else {}
        raise_for_status(resp.status_code, body)
        return body

    async def _arequest(self, method: str, path: str, **kwargs: Any) -> dict[str, Any]:
        resp = await self._async.request(method, path, **kwargs)
        if resp.status_code == 204:
            return {}
        body = resp.json() if resp.content else {}
        raise_for_status(resp.status_code, body)
        return body

    # ── Sessions ─────────────────────────────────────────────────────────

    def session(self, goal: str, **kwargs: Any) -> ApertureSession:
        """Create a session context manager (sync).

        Usage::

            with client.session("Navigate to example.com") as s:
                result = s.execute()
        """
        return ApertureSession(self, goal, **kwargs)

    def create_session(self, goal: str, **kwargs: Any) -> SessionInfo:
        """Create a new browser session."""
        payload: dict[str, Any] = {"goal": goal}
        if "agent_id" in kwargs:
            payload["agent_id"] = kwargs["agent_id"]
        if "mode" in kwargs:
            payload["mode"] = kwargs["mode"]
        if "metadata" in kwargs:
            payload["metadata"] = kwargs["metadata"]
        data = self._request("POST", "/sessions", json=payload)
        return SessionInfo(**data)

    async def acreate_session(self, goal: str, **kwargs: Any) -> SessionInfo:
        """Create a new browser session (async)."""
        payload: dict[str, Any] = {"goal": goal}
        if "agent_id" in kwargs:
            payload["agent_id"] = kwargs["agent_id"]
        if "mode" in kwargs:
            payload["mode"] = kwargs["mode"]
        if "metadata" in kwargs:
            payload["metadata"] = kwargs["metadata"]
        data = await self._arequest("POST", "/sessions", json=payload)
        return SessionInfo(**data)

    def get_session(self, session_id: str) -> SessionInfo:
        """Get session details."""
        data = self._request("GET", f"/sessions/{session_id}")
        return SessionInfo(**data)

    def list_sessions(self) -> list[SessionInfo]:
        """List all sessions."""
        data = self._request("GET", "/sessions")
        return [SessionInfo(**s) for s in data]

    def execute_session(self, session_id: str) -> ExecuteResult:
        """Execute a session's goal."""
        data = self._request("POST", f"/sessions/{session_id}/execute")
        return ExecuteResult(**data)

    async def aexecute_session(self, session_id: str) -> ExecuteResult:
        """Execute a session's goal (async)."""
        data = await self._arequest("POST", f"/sessions/{session_id}/execute")
        return ExecuteResult(**data)

    def snapshot(self, session_id: str) -> Snapshot:
        """Get current page snapshot."""
        data = self._request("GET", f"/sessions/{session_id}/snapshot")
        return Snapshot(**data)

    def delete_session(self, session_id: str) -> None:
        """Delete a session and release its browser."""
        self._request("DELETE", f"/sessions/{session_id}")

    async def adelete_session(self, session_id: str) -> None:
        """Delete a session (async)."""
        await self._arequest("DELETE", f"/sessions/{session_id}")

    def import_session(self, config: ImportConfig) -> ImportResult:
        """Import an authenticated session with cookies."""
        data = self._request("POST", "/sessions/import", json=config.model_dump())
        return ImportResult(**data)

    # ── Task Planner ─────────────────────────────────────────────────────

    def task(self, goal: str, **kwargs: Any) -> TaskPlanner:
        """Create a task planner that streams SSE events.

        Usage::

            for event in client.task("Get HN #1 story"):
                print(event.event, event.data)
        """
        return TaskPlanner(self, goal, **kwargs)

    # ── xBPP Policies ────────────────────────────────────────────────────

    def get_policy(self, agent_id: str) -> dict[str, Any]:
        """Get xBPP policy for an agent."""
        return self._request("GET", f"/policies/{agent_id}")

    def set_policy(self, agent_id: str, policy: PolicyConfig) -> dict[str, Any]:
        """Set xBPP policy for an agent."""
        return self._request("PUT", f"/policies/{agent_id}", json=policy.model_dump(exclude_none=True))

    def delete_policy(self, agent_id: str) -> None:
        """Delete xBPP policy for an agent."""
        self._request("DELETE", f"/policies/{agent_id}")

    # ── Credential Vault ─────────────────────────────────────────────────

    def store_credential(
        self, agent_id: str, domain: str, username: str, password: str
    ) -> dict[str, Any]:
        """Store a credential in the vault."""
        return self._request(
            "PUT",
            f"/agents/{agent_id}/credentials/{domain}",
            json={"username": username, "password": password},
        )

    def list_credentials(self, agent_id: str) -> list[Credential]:
        """List credentials for an agent."""
        data = self._request("GET", f"/agents/{agent_id}/credentials")
        creds = data if isinstance(data, list) else data.get("credentials", [])
        return [Credential(**c) for c in creds]

    def delete_credential(self, agent_id: str, domain: str) -> None:
        """Delete a credential."""
        self._request("DELETE", f"/agents/{agent_id}/credentials/{domain}")

    # ── Agent State KV ───────────────────────────────────────────────────

    def set_memory(self, agent_id: str, key: str, value: Any) -> dict[str, Any]:
        """Set a key-value pair in agent memory."""
        return self._request("PUT", f"/agents/{agent_id}/memory/{key}", json={"value": value})

    def get_memory(self, agent_id: str, key: str) -> Any:
        """Get a value from agent memory."""
        data = self._request("GET", f"/agents/{agent_id}/memory/{key}")
        return data.get("value", data)

    def list_memory(self, agent_id: str) -> dict[str, Any]:
        """List all keys in agent memory."""
        return self._request("GET", f"/agents/{agent_id}/memory")

    def delete_memory(self, agent_id: str, key: str) -> None:
        """Delete a key from agent memory."""
        self._request("DELETE", f"/agents/{agent_id}/memory/{key}")

    # ── Site Profiles ────────────────────────────────────────────────────

    def list_profiles(self) -> list[SiteProfile]:
        """List available site profiles."""
        data = self._request("GET", "/profiles")
        return [SiteProfile(**p) for p in data]

    # ── Health ───────────────────────────────────────────────────────────

    def health(self) -> dict[str, Any]:
        """Check API health."""
        resp = httpx.get(f"{self.base_url}/health")
        return resp.json()
