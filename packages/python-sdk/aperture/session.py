"""Session context manager for Aperture."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from aperture.types import ExecuteResult, SessionInfo, Snapshot

if TYPE_CHECKING:
    from aperture.client import ApertureClient


class ApertureSession:
    """Context manager that creates a session and cleans up on exit.

    Usage::

        with client.session("Navigate to example.com") as s:
            result = s.execute()
            snap = s.snapshot()
            print(snap.url, snap.title)
    """

    def __init__(self, client: ApertureClient, goal: str, **kwargs: Any):
        self._client = client
        self._goal = goal
        self._kwargs = kwargs
        self._info: SessionInfo | None = None

    @property
    def session_id(self) -> str:
        """The session ID (available after entering the context)."""
        if self._info is None:
            raise RuntimeError("Session not created yet — use as context manager")
        return self._info.session_id

    @property
    def info(self) -> SessionInfo:
        """Session metadata."""
        if self._info is None:
            raise RuntimeError("Session not created yet — use as context manager")
        return self._info

    def __enter__(self) -> ApertureSession:
        self._info = self._client.create_session(self._goal, **self._kwargs)
        return self

    def __exit__(self, *exc: Any) -> None:
        if self._info:
            try:
                self._client.delete_session(self._info.session_id)
            except Exception:
                pass  # best-effort cleanup

    async def __aenter__(self) -> ApertureSession:
        self._info = await self._client.acreate_session(self._goal, **self._kwargs)
        return self

    async def __aexit__(self, *exc: Any) -> None:
        if self._info:
            try:
                await self._client.adelete_session(self._info.session_id)
            except Exception:
                pass

    def execute(self) -> ExecuteResult:
        """Execute the session's goal and return results."""
        return self._client.execute_session(self.session_id)

    async def aexecute(self) -> ExecuteResult:
        """Execute the session's goal (async)."""
        return await self._client.aexecute_session(self.session_id)

    def snapshot(self) -> Snapshot:
        """Get the current page snapshot."""
        return self._client.snapshot(self.session_id)

    def get(self) -> SessionInfo:
        """Refresh session info."""
        self._info = self._client.get_session(self.session_id)
        return self._info
