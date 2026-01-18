"""Parallel Works SDK - Official Python client for the ACTIVATE platform API."""

__version__ = "6.12.0"

from parallelworks_client.auth import (
    API_KEY_PREFIX,
    Client,
    CredentialError,
    SyncClient,
    extract_platform_host,
    is_api_key,
    is_token,
)

__all__ = [
    "API_KEY_PREFIX",
    "Client",
    "CredentialError",
    "SyncClient",
    "extract_platform_host",
    "is_api_key",
    "is_token",
    "__version__",
]
