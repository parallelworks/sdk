"""Parallel Works SDK - Official Python client for the ACTIVATE platform API."""

from parallelworks_client.auth import (
    API_KEY_PREFIX,
    USER_TOKEN_PREFIX,
    Client,
    CredentialError,
    SyncClient,
    extract_platform_host,
    is_api_key,
    is_token,
)
from parallelworks_client.problem import (
    FieldError,
    ProblemError,
    accept_language_from_env,
    raise_for_problem,
)

__all__ = [
    "API_KEY_PREFIX",
    "Client",
    "CredentialError",
    "FieldError",
    "ProblemError",
    "SyncClient",
    "USER_TOKEN_PREFIX",
    "accept_language_from_env",
    "extract_platform_host",
    "is_api_key",
    "is_token",
    "raise_for_problem",
]
