"""Custom exceptions for the Aperture SDK."""

from __future__ import annotations

from typing import Any, Optional


class ApertureError(Exception):
    """Base exception for all Aperture SDK errors."""

    def __init__(
        self,
        message: str,
        status_code: Optional[int] = None,
        code: Optional[str] = None,
        response_body: Optional[dict[str, Any]] = None,
    ):
        super().__init__(message)
        self.status_code = status_code
        self.code = code
        self.response_body = response_body

    def __str__(self) -> str:
        parts = [super().__str__()]
        if self.status_code:
            parts.append(f"(HTTP {self.status_code})")
        if self.code:
            parts.append(f"[{self.code}]")
        return " ".join(parts)


class AuthenticationError(ApertureError):
    """Raised when API key is missing or invalid (401/403)."""

    pass


class RateLimitError(ApertureError):
    """Raised when rate limit is exceeded (429)."""

    def __init__(self, message: str = "Rate limit exceeded", **kwargs: Any):
        super().__init__(message, status_code=429, code="RATE_LIMITED", **kwargs)


class SessionNotFoundError(ApertureError):
    """Raised when a session ID doesn't exist (404)."""

    pass


class PolicyBlockedError(ApertureError):
    """Raised when xBPP policy blocks an action."""

    pass


class BudgetExhaustedError(ApertureError):
    """Raised when account credits are exhausted."""

    pass


class SessionLimitError(ApertureError):
    """Raised when per-account session limit is reached."""

    pass


class BodyTooLargeError(ApertureError):
    """Raised when request body exceeds size limit (413)."""

    pass


def raise_for_status(status_code: int, body: dict[str, Any]) -> None:
    """Raise the appropriate exception based on HTTP status code."""
    if status_code < 400:
        return

    message = body.get("error", body.get("message", "Unknown error"))
    code = body.get("code", "")

    if status_code == 401:
        raise AuthenticationError(message, status_code=status_code, code=code, response_body=body)
    if status_code == 403:
        if "policy" in message.lower():
            raise PolicyBlockedError(message, status_code=status_code, code=code, response_body=body)
        raise AuthenticationError(message, status_code=status_code, code=code, response_body=body)
    if status_code == 404:
        raise SessionNotFoundError(message, status_code=status_code, code=code, response_body=body)
    if status_code == 413:
        raise BodyTooLargeError(message, status_code=status_code, code=code, response_body=body)
    if status_code == 429:
        if "session" in message.lower() or "limit" in code.lower():
            raise SessionLimitError(message, status_code=status_code, code=code, response_body=body)
        raise RateLimitError(message, response_body=body)
    if status_code == 402 or "credit" in message.lower() or "budget" in message.lower():
        raise BudgetExhaustedError(message, status_code=status_code, code=code, response_body=body)

    raise ApertureError(message, status_code=status_code, code=code, response_body=body)
