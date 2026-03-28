"""Type definitions for the Aperture SDK."""

from __future__ import annotations

from datetime import datetime
from typing import Any, Optional

from pydantic import BaseModel, Field


class SessionConfig(BaseModel):
    """Configuration for creating a new session."""

    goal: str
    agent_id: Optional[str] = None
    mode: Optional[str] = None  # "research", "hardened", "max"
    metadata: Optional[dict[str, str]] = None


class SessionInfo(BaseModel):
    """Session metadata returned by the API."""

    session_id: str
    status: str
    goal: Optional[str] = None
    browser_id: Optional[str] = None
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None


class StepResult(BaseModel):
    """Result of a single execution step."""

    action: Optional[str] = None
    target: Optional[str] = None
    result: Optional[dict[str, Any]] = None
    cost: Optional[int] = None
    duration_ms: Optional[int] = None


class ExecuteResult(BaseModel):
    """Result of session execution."""

    success: bool
    steps: list[StepResult] = Field(default_factory=list)
    total_cost: Optional[int] = None
    duration_ms: Optional[int] = None
    error: Optional[str] = None


class Snapshot(BaseModel):
    """Page snapshot (current state of the browser)."""

    url: Optional[str] = None
    title: Optional[str] = None
    cookies: Optional[int] = None


class ImportConfig(BaseModel):
    """Configuration for importing an authenticated session."""

    cookies: list[dict[str, Any]]
    trust_mode: str = "standard"  # "standard" or "preserve"
    source_url: Optional[str] = None


class ImportResult(BaseModel):
    """Result of session import."""

    session_id: str
    profile_id: str
    trust_mode: str
    cookies_imported: int


class PolicyConfig(BaseModel):
    """xBPP policy configuration."""

    blocked_domains: list[str] = Field(default_factory=list)
    allowed_domains: list[str] = Field(default_factory=list)
    allowed_actions: list[str] = Field(default_factory=list)
    max_actions: Optional[int] = None
    rate_limit_rpm: Optional[int] = None


class Credential(BaseModel):
    """Stored credential (password is never returned in plaintext)."""

    domain: str
    username: str
    has_password: bool = True


class TaskEvent(BaseModel):
    """Server-sent event from the task planner."""

    event: str  # "progress", "complete", "error"
    data: dict[str, Any] = Field(default_factory=dict)


class AccountInfo(BaseModel):
    """Billing account information."""

    id: str
    name: str
    email: str
    plan: str
    credits_remaining: int
    is_admin: bool = False


class SiteProfile(BaseModel):
    """Available site profile."""

    domain: str
    version: Optional[str] = None
    pages: Optional[list[str]] = None
