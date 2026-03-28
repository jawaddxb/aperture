"""Task planner with SSE streaming support."""

from __future__ import annotations

import json
from typing import TYPE_CHECKING, Any, Iterator

from aperture.types import TaskEvent

if TYPE_CHECKING:
    from aperture.client import ApertureClient


class TaskPlanner:
    """Stream task planner results via Server-Sent Events.

    Usage::

        for event in client.task("Get HN #1 story title"):
            print(event.event, event.data)

    Or with explicit configuration::

        planner = TaskPlanner(client, "Search for X", mode="research")
        for event in planner:
            if event.event == "complete":
                print(event.data)
    """

    def __init__(
        self,
        client: ApertureClient,
        goal: str,
        mode: str | None = None,
        checkpoint_id: str | None = None,
    ):
        self._client = client
        self._goal = goal
        self._mode = mode
        self._checkpoint_id = checkpoint_id

    def __iter__(self) -> Iterator[TaskEvent]:
        """Stream SSE events from the task planner."""
        payload: dict[str, Any] = {"goal": self._goal}
        if self._mode:
            payload["mode"] = self._mode

        endpoint = "/tasks/execute"
        if self._checkpoint_id:
            endpoint = "/tasks/resume"
            payload["checkpoint_id"] = self._checkpoint_id

        with self._client._sync.stream("POST", endpoint, json=payload) as response:
            if response.status_code >= 400:
                body = json.loads(response.read())
                from aperture.errors import raise_for_status
                raise_for_status(response.status_code, body)

            buffer = ""
            current_event = "message"
            current_data = ""

            for chunk in response.iter_text():
                buffer += chunk
                while "\n" in buffer:
                    line, buffer = buffer.split("\n", 1)
                    line = line.rstrip("\r")

                    if line.startswith("event:"):
                        current_event = line[6:].strip()
                    elif line.startswith("data:"):
                        current_data += line[5:].strip()
                    elif line == "":
                        if current_data:
                            try:
                                data = json.loads(current_data)
                            except json.JSONDecodeError:
                                data = {"raw": current_data}
                            yield TaskEvent(event=current_event, data=data)
                        current_event = "message"
                        current_data = ""

    def execute(self) -> list[TaskEvent]:
        """Execute the task and collect all events (non-streaming)."""
        return list(self)

    @property
    def events(self) -> list[TaskEvent]:
        """Alias for execute()."""
        return self.execute()
