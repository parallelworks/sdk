"""Authentication utilities for the Parallel Works client."""

from __future__ import annotations

import base64
import json
from dataclasses import dataclass
from typing import TYPE_CHECKING

import httpx

if TYPE_CHECKING:
    from typing import Self

# Prefix for Parallel Works API keys
API_KEY_PREFIX = "pwt_"


class CredentialError(Exception):
    """Raised when a credential cannot be parsed."""

    pass


def is_api_key(credential: str) -> bool:
    """
    Check if a credential is an API key.

    API keys start with the prefix "pwt_".

    Args:
        credential: The credential string to check

    Returns:
        True if the credential appears to be an API key
    """
    return credential.strip().startswith(API_KEY_PREFIX)


def is_token(credential: str) -> bool:
    """
    Check if a credential is a JWT token.

    JWTs have three base64-encoded parts separated by dots.

    Args:
        credential: The credential string to check

    Returns:
        True if the credential appears to be a JWT token
    """
    credential = credential.strip()
    parts = credential.split(".")
    return len(parts) == 3 and not credential.startswith(API_KEY_PREFIX)


def extract_platform_host(credential: str) -> str:
    """
    Extract the platform host from an API key or JWT token.

    For API keys (pwt_xxxx.yyyy): decodes the first part after pwt_ to get the host
    For JWT tokens: decodes the payload (second segment) and reads platform_host field

    Args:
        credential: The API key or JWT token

    Returns:
        The platform host (e.g., "activate.parallel.works")

    Raises:
        CredentialError: If the credential format is invalid or host cannot be extracted
    """
    credential = credential.strip()
    if is_api_key(credential):
        return _extract_host_from_api_key(credential)
    if is_token(credential):
        return _extract_host_from_token(credential)
    raise CredentialError("Invalid credential format")


def _extract_host_from_api_key(api_key: str) -> str:
    """Extract platform host from an API key."""
    # Remove pwt_ prefix
    without_prefix = api_key[len(API_KEY_PREFIX) :]

    # Split by dot
    parts = without_prefix.split(".", 1)
    if len(parts) < 2:
        raise CredentialError("Invalid API key format")

    # Decode the first part (host) - try URL-safe then standard base64
    encoded_host = parts[0]
    try:
        # Add padding if needed
        padding = 4 - len(encoded_host) % 4
        if padding != 4:
            encoded_host += "=" * padding
        host = base64.urlsafe_b64decode(encoded_host).decode()
    except Exception:
        try:
            host = base64.b64decode(parts[0]).decode()
        except Exception as e:
            raise CredentialError(f"Could not decode API key host: {e}") from e

    if not host:
        raise CredentialError("No platform host in API key")

    return host


def _extract_host_from_token(token: str) -> str:
    """Extract platform host from a JWT token."""
    parts = token.split(".")
    if len(parts) != 3:
        raise CredentialError("Invalid JWT format")

    # Decode the payload (second part)
    payload = parts[1]
    # Add padding if needed
    padding = 4 - len(payload) % 4
    if padding != 4:
        payload += "=" * padding

    try:
        payload_bytes = base64.urlsafe_b64decode(payload)
        claims = json.loads(payload_bytes)
    except Exception as e:
        raise CredentialError(f"Could not decode JWT payload: {e}") from e

    host = claims.get("platform_host")
    if not host:
        raise CredentialError("No platform_host in JWT claims")

    return host


@dataclass
class ClientConfig:
    """Configuration for the Parallel Works API client."""

    base_url: str
    auth_header: str
    timeout: float = 30.0


class Client:
    """
    Parallel Works API client with authentication support.

    Use the class methods to create an authenticated client:

        # Using API Key (recommended for integrations)
        client = Client.with_api_key(
            "https://activate.parallel.works",
            "your-api-key"
        )

        # Using JWT Token (for scripts, expires in 24h)
        client = Client.with_token(
            "https://activate.parallel.works",
            "your-jwt-token"
        )

    The client can be used as a context manager:

        async with Client.with_api_key(base_url, api_key) as client:
            orgs = await client.get("/api/organizations")

    Or for synchronous usage:

        with Client.with_api_key(base_url, api_key).sync() as client:
            orgs = client.get("/api/organizations")
    """

    def __init__(self, config: ClientConfig) -> None:
        """Initialize client with configuration. Use class methods instead."""
        self._config = config
        self._async_client: httpx.AsyncClient | None = None
        self._sync_client: httpx.Client | None = None

    @classmethod
    def with_api_key(cls, base_url: str, api_key: str, *, timeout: float = 30.0) -> Self:
        """
        Create a client authenticated with an API key (Basic Auth).

        This is the recommended authentication method for long-running
        integrations with configurable expiration.

        API keys can be generated from your ACTIVATE account settings.

        Args:
            base_url: The Parallel Works platform URL (e.g., "https://activate.parallel.works")
            api_key: Your API key from account settings
            timeout: Request timeout in seconds (default: 30)

        Returns:
            An authenticated Client instance

        Example:
            client = Client.with_api_key(
                "https://activate.parallel.works",
                os.environ["PW_API_KEY"]
            )
        """
        # Strip whitespace to handle env vars with trailing newlines
        api_key = api_key.strip()
        encoded = base64.b64encode(f"{api_key}:".encode()).decode()
        config = ClientConfig(
            base_url=base_url.rstrip("/"),
            auth_header=f"Basic {encoded}",
            timeout=timeout,
        )
        return cls(config)

    @classmethod
    def with_token(cls, base_url: str, token: str, *, timeout: float = 30.0) -> Self:
        """
        Create a client authenticated with a Bearer token (JWT).

        This is best for scripts and CLI tools. Tokens expire after 24 hours
        and can be generated from your ACTIVATE account settings.

        Args:
            base_url: The Parallel Works platform URL (e.g., "https://activate.parallel.works")
            token: Your JWT token from account settings
            timeout: Request timeout in seconds (default: 30)

        Returns:
            An authenticated Client instance

        Example:
            client = Client.with_token(
                "https://activate.parallel.works",
                os.environ["PW_API_KEY"]
            )
        """
        # Strip whitespace to handle env vars with trailing newlines
        token = token.strip()
        config = ClientConfig(
            base_url=base_url.rstrip("/"),
            auth_header=f"Bearer {token}",
            timeout=timeout,
        )
        return cls(config)

    @classmethod
    def with_credential(cls, base_url: str, credential: str, *, timeout: float = 30.0) -> Self:
        """
        Create a client with automatic credential type detection.

        Automatically detects whether the credential is an API key (starts with "pwt_")
        or a JWT token and configures the appropriate authentication method.

        Args:
            base_url: The Parallel Works platform URL (e.g., "https://activate.parallel.works")
            credential: Your API key or JWT token
            timeout: Request timeout in seconds (default: 30)

        Returns:
            An authenticated Client instance

        Example:
            credential = os.environ.get("PW_API_KEY") or os.environ.get("PW_TOKEN")
            client = Client.with_credential(
                "https://activate.parallel.works",
                credential
            )
        """
        if is_api_key(credential):
            return cls.with_api_key(base_url, credential, timeout=timeout)
        return cls.with_token(base_url, credential, timeout=timeout)

    @classmethod
    def from_credential(cls, credential: str, *, timeout: float = 30.0) -> Self:
        """
        Create a client using only a credential.

        The platform host is automatically extracted from the credential:
        - For API keys: host is decoded from the first part after pwt_
        - For JWT tokens: host is read from the platform_host claim

        Args:
            credential: Your API key or JWT token
            timeout: Request timeout in seconds (default: 30)

        Returns:
            An authenticated Client instance

        Raises:
            CredentialError: If the credential format is invalid or host cannot be extracted

        Example:
            # Just pass your credential - no URL needed!
            client = Client.from_credential(os.environ["PW_API_KEY"])
        """
        host = extract_platform_host(credential)

        # Ensure https:// prefix
        if not host.startswith(("http://", "https://")):
            host = f"https://{host}"

        return cls.with_credential(host, credential, timeout=timeout)

    def _get_headers(self) -> dict[str, str]:
        """Get the default headers for requests."""
        return {
            "Authorization": self._config.auth_header,
            "Content-Type": "application/json",
        }

    # Async client methods

    async def __aenter__(self) -> Self:
        """Enter async context manager."""
        self._async_client = httpx.AsyncClient(
            base_url=self._config.base_url,
            headers=self._get_headers(),
            timeout=self._config.timeout,
        )
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb) -> None:
        """Exit async context manager."""
        if self._async_client:
            await self._async_client.aclose()
            self._async_client = None

    async def get(self, path: str, **kwargs) -> httpx.Response:
        """Make an async GET request."""
        if not self._async_client:
            raise RuntimeError("Client not initialized. Use 'async with' context manager.")
        return await self._async_client.get(path, **kwargs)

    async def post(self, path: str, **kwargs) -> httpx.Response:
        """Make an async POST request."""
        if not self._async_client:
            raise RuntimeError("Client not initialized. Use 'async with' context manager.")
        return await self._async_client.post(path, **kwargs)

    async def put(self, path: str, **kwargs) -> httpx.Response:
        """Make an async PUT request."""
        if not self._async_client:
            raise RuntimeError("Client not initialized. Use 'async with' context manager.")
        return await self._async_client.put(path, **kwargs)

    async def patch(self, path: str, **kwargs) -> httpx.Response:
        """Make an async PATCH request."""
        if not self._async_client:
            raise RuntimeError("Client not initialized. Use 'async with' context manager.")
        return await self._async_client.patch(path, **kwargs)

    async def delete(self, path: str, **kwargs) -> httpx.Response:
        """Make an async DELETE request."""
        if not self._async_client:
            raise RuntimeError("Client not initialized. Use 'async with' context manager.")
        return await self._async_client.delete(path, **kwargs)

    # Sync client wrapper

    def sync(self) -> SyncClient:
        """
        Get a synchronous client wrapper.

        Example:
            with Client.with_api_key(base_url, api_key).sync() as client:
                response = client.get("/api/organizations")
        """
        return SyncClient(self._config)


class SyncClient:
    """Synchronous wrapper for the Parallel Works client."""

    def __init__(self, config: ClientConfig) -> None:
        self._config = config
        self._client: httpx.Client | None = None

    def _get_headers(self) -> dict[str, str]:
        """Get the default headers for requests."""
        return {
            "Authorization": self._config.auth_header,
            "Content-Type": "application/json",
        }

    def __enter__(self) -> SyncClient:
        """Enter sync context manager."""
        self._client = httpx.Client(
            base_url=self._config.base_url,
            headers=self._get_headers(),
            timeout=self._config.timeout,
        )
        return self

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        """Exit sync context manager."""
        if self._client:
            self._client.close()
            self._client = None

    def get(self, path: str, **kwargs) -> httpx.Response:
        """Make a GET request."""
        if not self._client:
            raise RuntimeError("Client not initialized. Use 'with' context manager.")
        return self._client.get(path, **kwargs)

    def post(self, path: str, **kwargs) -> httpx.Response:
        """Make a POST request."""
        if not self._client:
            raise RuntimeError("Client not initialized. Use 'with' context manager.")
        return self._client.post(path, **kwargs)

    def put(self, path: str, **kwargs) -> httpx.Response:
        """Make a PUT request."""
        if not self._client:
            raise RuntimeError("Client not initialized. Use 'with' context manager.")
        return self._client.put(path, **kwargs)

    def patch(self, path: str, **kwargs) -> httpx.Response:
        """Make a PATCH request."""
        if not self._client:
            raise RuntimeError("Client not initialized. Use 'with' context manager.")
        return self._client.patch(path, **kwargs)

    def delete(self, path: str, **kwargs) -> httpx.Response:
        """Make a DELETE request."""
        if not self._client:
            raise RuntimeError("Client not initialized. Use 'with' context manager.")
        return self._client.delete(path, **kwargs)
