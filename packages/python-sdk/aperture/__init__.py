"""Aperture SDK — Python client for the Aperture browser automation engine."""

from aperture.client import ApertureClient
from aperture.session import ApertureSession
from aperture.task import TaskPlanner
from aperture.errors import (
    ApertureError,
    AuthenticationError,
    RateLimitError,
    SessionNotFoundError,
    PolicyBlockedError,
    BudgetExhaustedError,
)

__version__ = "0.1.0"
__all__ = [
    "ApertureClient",
    "ApertureSession",
    "TaskPlanner",
    "ApertureError",
    "AuthenticationError",
    "RateLimitError",
    "SessionNotFoundError",
    "PolicyBlockedError",
    "BudgetExhaustedError",
]
