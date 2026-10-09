"""Authentication utilities for the Parallel Works client."""

from __future__ import annotations

import asyncio
import base64
import json
import os
import subprocess
import sys
import threading
import time
from collections.abc import AsyncGenerator, Generator
from dataclasses import dataclass, field
from pathlib import Path
from typing import TYPE_CHECKING, Any

import httpx

from parallelworks_client.problem import PROBLEM_MEDIA_TYPE, accept_language_from_env

if TYPE_CHECKING:
    from typing import Self

# Prefix for Parallel Works API keys
API_KEY_PREFIX = "pwt_"

# Prefix for opaque access tokens, such as the one `pw auth` saves. Sent as a
# Bearer token; it names no platform host.
OPAQUE_ACCESS_TOKEN_PREFIX = "pwoa_"

USER_AGENT = "parallelworks-python-sdk/0"


class CredentialError(Exception):
    """Raised when a credential cannot be parsed."""

    pass


class NoPlatformHostError(CredentialError):
    """Raised when a credential names no platform host and nothing else supplies one."""

    def __init__(self, message: str = "could not extract platform host from credential") -> None:
        super().__init__(message)


class SignInExpiredError(CredentialError):
    """Raised when the pw CLI cannot provide a token for a `pw auth` sign-in and no earlier one is still valid."""

    def __init__(self, cause: str) -> None:
        super().__init__(
            f"the pw CLI could not provide a token for the pw auth sign-in; run pw auth again, or install pw: {cause}"
        )


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
    Check if a credential is sent as a Bearer token: a JWT, whose three
    base64-encoded parts are separated by dots, or an opaque access token.

    Args:
        credential: The credential string to check

    Returns:
        True if the credential appears to be a token
    """
    credential = credential.strip()
    if credential.startswith(OPAQUE_ACCESS_TOKEN_PREFIX):
        return True
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
        NoPlatformHostError: For an opaque access token, which names no host
        CredentialError: If the credential format is invalid or host cannot be extracted
    """
    credential = credential.strip()
    if is_api_key(credential):
        return _extract_host_from_api_key(credential)
    if credential.startswith(OPAQUE_ACCESS_TOKEN_PREFIX):
        raise NoPlatformHostError()
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


def _with_scheme(host: str) -> str:
    return host if host.startswith(("http://", "https://")) else f"https://{host}"


@dataclass
class ClientConfig:
    """Configuration for the Parallel Works API client."""

    base_url: str
    auth_header: str
    timeout: float = 30.0
    # Authenticates each request in place of auth_header, such as a CLIAuth.
    auth: httpx.Auth | None = None


def _default_headers(config: ClientConfig) -> dict[str, str]:
    headers = {
        "Accept": f"application/json, {PROBLEM_MEDIA_TYPE}",
        "Content-Type": "application/json",
        "User-Agent": USER_AGENT,
    }
    if config.auth_header:
        headers["Authorization"] = config.auth_header
    language = accept_language_from_env()
    if language:
        headers["Accept-Language"] = language
    return headers


class Client:
    """
    Parallel Works API client with authentication support.

    Use the class methods to create an authenticated client:

        # Signed in with `pw auth`: the client asks the pw CLI to renew the token
        client = Client.from_credential_config()

        # Using API Key (for unattended jobs such as CI)
        client = Client.with_api_key(
            "https://activate.parallel.works",
            "your-api-key"
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
        Create a client authenticated with a Bearer token, sent as is and never renewed.

        For a script, sign in once with `pw auth` and use from_credential_config,
        which renews the token; for an unattended job such as CI, use an API key.

        Args:
            base_url: The Parallel Works platform URL (e.g., "https://activate.parallel.works")
            token: A token, such as one `pw auth token --print` printed
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
        or a token and configures the appropriate authentication method.

        Args:
            base_url: The Parallel Works platform URL (e.g., "https://activate.parallel.works")
            credential: Your API key or token
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
        - For access tokens (pwoa_), which name no host: PW_PLATFORM_HOST, else
          the server of the credentials file's selected context

        Args:
            credential: Your API key or token
            timeout: Request timeout in seconds (default: 30)

        Returns:
            An authenticated Client instance

        Raises:
            CredentialError: If the credential format is invalid or host cannot be extracted

        Example:
            # Just pass your credential - no URL needed!
            client = Client.from_credential(os.environ["PW_API_KEY"])
        """
        try:
            host = extract_platform_host(credential)
        except NoPlatformHostError:
            fallback = host_for_unnamed_credential()
            if not fallback:
                raise
            host = fallback

        return cls.with_credential(_with_scheme(host), credential, timeout=timeout)

    @classmethod
    def from_credential_config(
        cls,
        *,
        context: str | None = None,
        platform_host: str | None = None,
        cli_command: str | None = None,
        cli_timeout: float | None = None,
        timeout: float = 30.0,
    ) -> Self:
        """
        Create a client from the pw credentials file.

        The credential is picked as the CLI and the Go SDK pick it: PW_API_KEY,
        then the context argument, PW_CONTEXT, and the file's current context.

        A context signed in with `pw auth` stays signed in: when its access
        token nears expiry the client runs `pw auth token --print` for a renewed
        one (see CLIAuth), so sign in once and let scripts run. Unattended jobs
        such as CI should use an API key in PW_API_KEY instead.

        Args:
            context: The context to use, over PW_CONTEXT and the current context
            platform_host: The platform host to use, over the context's server
            cli_command: The pw executable that renews a sign-in (default "pw", on PATH)
            cli_timeout: Bounds each run of the pw CLI, in seconds (default 60)
            timeout: Request timeout in seconds (default: 30)

        Example:
            async with Client.from_credential_config() as client:
                response = await client.get("/api/buckets")
        """
        config = load_credential_config()
        name, identity = resolve_identity(config, context=context, platform_host=platform_host)
        base_url = _with_scheme(identity.server)
        if identity.apikey:
            if is_token(identity.apikey):
                return cls.with_token(base_url, identity.apikey, timeout=timeout)
            return cls.with_api_key(base_url, identity.apikey, timeout=timeout)
        if not identity.token:
            raise CredentialError('you must first authenticate using "pw auth"')
        if identity.oauth:
            auth = CLIAuth(command=cli_command, context=name, timeout=cli_timeout)
            return cls(ClientConfig(base_url=base_url.rstrip("/"), auth_header="", timeout=timeout, auth=auth))
        return cls.with_token(base_url, identity.token, timeout=timeout)

    # Async client methods

    async def __aenter__(self) -> Self:
        """Enter async context manager."""
        self._async_client = httpx.AsyncClient(
            base_url=self._config.base_url,
            headers=_default_headers(self._config),
            timeout=self._config.timeout,
            auth=self._config.auth,
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

    def __enter__(self) -> SyncClient:
        """Enter sync context manager."""
        self._client = httpx.Client(
            base_url=self._config.base_url,
            headers=_default_headers(self._config),
            timeout=self._config.timeout,
            auth=self._config.auth,
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


DEFAULT_CLI_COMMAND = "pw"
"""The pw executable CLIAuth runs, looked up on PATH."""

DEFAULT_CLI_TIMEOUT = 60.0
"""One minute, as the AWS SDK's credential_process provider bounds a run by default."""

# Inside the CLI's two-minute renewal window, so the CLI always renews a token the SDK asks it for.
_CLI_RENEW_MARGIN = 60.0


@dataclass
class Identity:
    """A saved context in the pw credentials file, as the CLI writes it."""

    server: str
    name: str = ""
    organization: str = ""
    apikey: str = ""
    token: str = ""
    canonical_name: str = ""
    oauth: dict[str, Any] | None = None

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> Identity:
        return cls(
            server=data.get("server", ""),
            name=data.get("name", ""),
            organization=data.get("organization", ""),
            apikey=data.get("apikey", ""),
            token=data.get("token", ""),
            canonical_name=data.get("canonicalName", ""),
            oauth=data.get("oauth"),
        )


@dataclass
class CredentialConfig:
    """The pw credentials file."""

    identities: dict[str, Identity] = field(default_factory=dict)
    current_identity: str = ""
    path: str = ""


def default_credential_config_path() -> str:
    """
    The credentials file path, ~/.config/pw/credentials (XDG_CONFIG_HOME moves
    it, PW_CREDENTIALS_DIR replaces its directory), as the CLI and Go SDK resolve it.
    """
    override = os.environ.get("PW_CREDENTIALS_DIR")
    if override:
        return os.path.join(override, ".credentials")
    xdg = os.environ.get("XDG_CONFIG_HOME", "")
    base = xdg if os.path.isabs(xdg) else os.path.join(Path.home(), ".config")
    return os.path.join(base, "pw", "credentials")


def load_credential_config(path: str | None = None) -> CredentialConfig:
    """
    Read the credentials file, falling back to the legacy ~/pw/.credentials.
    A missing, empty or unparseable file reads as no contexts.
    """
    if not path:
        path = default_credential_config_path()
        if not os.path.exists(path) and not os.environ.get("PW_CREDENTIALS_DIR"):
            legacy = os.path.join(Path.home(), "pw", ".credentials")
            if os.path.exists(legacy):
                path = legacy
    try:
        with open(path, encoding="utf-8") as f:
            data = f.read()
    except FileNotFoundError:
        return CredentialConfig(path=path)
    if not data.strip():
        return CredentialConfig(path=path)
    try:
        parsed = json.loads(data)
    except ValueError:
        print(
            f"warning: credentials file ({path}) is unparseable, treating as missing; "
            "re-authenticate with 'pw auth' to recover",
            file=sys.stderr,
        )
        return CredentialConfig(path=path)
    return CredentialConfig(
        identities={name: Identity.from_json(i) for name, i in (parsed.get("identities") or {}).items()},
        current_identity=parsed.get("currentIdentity") or "",
        path=path,
    )


def _selected_context(config: CredentialConfig, override: str | None = None) -> str:
    """The context the CLI uses too: the override, then PW_CONTEXT, then the current context."""
    return override or os.environ.get("PW_CONTEXT") or config.current_identity


def resolve_identity(
    config: CredentialConfig, *, context: str | None = None, platform_host: str | None = None
) -> tuple[str, Identity]:
    """
    The identity a client authenticates as, and the context it came from (empty
    for PW_API_KEY): PW_API_KEY, then the context argument, PW_CONTEXT, and the
    file's current context, as the CLI and Go SDK pick it.
    """
    credential = os.environ.get("PW_API_KEY")
    if credential:
        return "", _identity_from_credential(config, credential, context, platform_host)
    name = _selected_context(config, context)
    if not name:
        raise CredentialError("no context configured; use 'pw auth' to authenticate")
    identity = config.identities.get(name)
    if identity is None:
        raise CredentialError(f'context "{name}" not found')
    identity = Identity(**identity.__dict__)
    if platform_host:
        identity.server = platform_host
    return name, identity


def _identity_from_credential(
    config: CredentialConfig, credential: str, context: str | None, platform_host: str | None
) -> Identity:
    # A token that names no platform goes to PW_PLATFORM_HOST, else the selected context's server.
    host = platform_host
    if not host:
        try:
            host = extract_platform_host(credential)
        except NoPlatformHostError:
            selected = config.identities.get(_selected_context(config, context))
            host = os.environ.get("PW_PLATFORM_HOST") or (selected.server if selected else "")
            if not host:
                raise
    if is_token(credential):
        return Identity(server=host, token=credential)
    return Identity(server=host, apikey=credential)


def host_for_unnamed_credential() -> str | None:
    """The platform host for a credential that names none: PW_PLATFORM_HOST, else the selected context's server."""
    host = os.environ.get("PW_PLATFORM_HOST")
    if host:
        return host
    config = load_credential_config()
    selected = config.identities.get(_selected_context(config))
    return selected.server if selected and selected.server else None


class CLIAuth(httpx.Auth):
    """
    Authenticates as a `pw auth` sign-in.

    It runs `pw auth token --print -o json` for a current access token instead
    of rotating the refresh token itself: the platform signs a device out when
    two clients rotate the same refresh token, and the CLI serializes its
    rotations across processes. Like a client-go exec credential plugin or an
    AWS credential_process, it reads the token and its lifetime from the
    command's stdout, an RFC 6749 section 5.1 token response, and caches the
    token in memory until it nears expiry.
    """

    def __init__(
        self,
        *,
        command: str | None = None,
        context: str | None = None,
        timeout: float | None = None,
    ) -> None:
        self.command = command or DEFAULT_CLI_COMMAND
        self.context = context or ""
        self.timeout = timeout or DEFAULT_CLI_TIMEOUT
        self._lock = threading.Lock()
        self._token = ""
        # None when the CLI did not say, so the token is used for the life of the client.
        self._expires_at: float | None = None
        # A token the CLI could not renew is used until it expires rather than asking on every request.
        self._settled = False

    def sync_auth_flow(self, request: httpx.Request) -> Generator[httpx.Request, httpx.Response, None]:
        request.headers["Authorization"] = f"Bearer {self.token()}"
        yield request

    async def async_auth_flow(self, request: httpx.Request) -> AsyncGenerator[httpx.Request, httpx.Response]:
        token = self._fresh_token() or await asyncio.get_running_loop().run_in_executor(None, self.token)
        request.headers["Authorization"] = f"Bearer {token}"
        yield request

    def _fresh_token(self) -> str:
        # Never blocks, so the event loop does not wait on a CLI run another thread holds the lock for.
        if not self._lock.acquire(blocking=False):
            return ""
        try:
            return self._token if self._usable() else ""
        finally:
            self._lock.release()

    def token(self) -> str:
        """A current access token for the sign-in, running the CLI first when it is due."""
        with self._lock:
            if self._usable():
                return self._token
            try:
                token, expires_at = self._run()
            except CredentialError as e:
                if self._token and self._expires_at is not None and time.time() < self._expires_at:
                    self._settled = True
                    return self._token
                raise SignInExpiredError(str(e)) from e
            self._token, self._expires_at = token, expires_at
            self._settled = expires_at is not None and expires_at - time.time() <= _CLI_RENEW_MARGIN
            return self._token

    def _usable(self) -> bool:
        if not self._token:
            return False
        if self._expires_at is None:
            return True
        if self._settled:
            return time.time() < self._expires_at
        return self._expires_at - time.time() > _CLI_RENEW_MARGIN

    def _run(self) -> tuple[str, float | None]:
        args = [self.command, "auth", "token", "--print", "-o", "json"]
        if self.context:
            args += ["--context", self.context]
        describe = " ".join(args)
        executable = resolve_command(self.command)
        # Without PW_API_KEY, which would make the CLI print it rather than the sign-in's token.
        env = {k: v for k, v in os.environ.items() if k != "PW_API_KEY"}
        try:
            result = subprocess.run(
                [executable, *args[1:]],
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                env=env,
                timeout=self.timeout,
                check=False,
            )
        except FileNotFoundError as e:
            raise CredentialError(f"{describe}: not found") from e
        except subprocess.TimeoutExpired as e:
            raise CredentialError(f"{describe} did not finish within {self.timeout:g}s") from e
        except OSError as e:
            raise CredentialError(f"{describe}: {e}") from e
        received = time.time()
        if result.returncode != 0:
            detail = result.stderr.strip()
            raise CredentialError(f"{describe}: exit status {result.returncode}" + (f": {detail}" if detail else ""))
        return _parse_token_response(describe, result.stdout, received)


def _parse_token_response(describe: str, stdout: str, received: float) -> tuple[str, float | None]:
    try:
        response = json.loads(stdout)
    except ValueError as e:
        raise CredentialError(f"{describe} printed no token response: {e}") from e
    if not isinstance(response, dict):
        raise CredentialError(f"{describe} printed no token response")
    token = response.get("access_token")
    if not isinstance(token, str) or not token or any(c.isspace() for c in token):
        raise CredentialError(f"{describe} printed no single access_token")
    token_type = response.get("token_type")
    if not isinstance(token_type, str) or token_type.lower() != "bearer":
        raise CredentialError(f"{describe} printed token_type {token_type!r}, want Bearer")
    expires_in = response.get("expires_in")
    if isinstance(expires_in, (int, float)) and not isinstance(expires_in, bool):
        return token, received + expires_in
    return token, None


def resolve_command(command: str) -> str:
    """
    Find command on PATH as Go's exec.LookPath does: a command that names a
    path is used as is, and a match through a relative PATH entry, such as the
    current directory, is refused. Windows' own lookup would search the
    current directory first.
    """
    windows = sys.platform == "win32"
    if "/" in command or (windows and ("\\" in command or ":" in command)):
        return command
    extensions = [""]
    if windows:
        extensions = [ext for ext in os.environ.get("PATHEXT", ".COM;.EXE;.BAT;.CMD").split(";") if ext]
        if any(command.lower().endswith(ext.lower()) for ext in extensions):
            extensions.insert(0, "")
    for directory in os.environ.get("PATH", "").split(os.pathsep):
        for ext in extensions:
            candidate = os.path.join(directory or ".", command + ext)
            if not os.path.isfile(candidate) or not (windows or os.access(candidate, os.X_OK)):
                continue
            if not os.path.isabs(candidate):
                raise CredentialError(
                    f"{command} resolves to {candidate}, relative to the current directory; "
                    "give the pw command as an absolute path to run it"
                )
            return candidate
    raise CredentialError(f"{command}: not found on PATH")
