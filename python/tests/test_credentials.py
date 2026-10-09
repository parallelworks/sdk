from __future__ import annotations

import asyncio
import functools
import json
import os
import stat
import sys
import time
from datetime import datetime, timedelta, timezone
from pathlib import Path

import httpx
import pytest

from parallelworks_client import (
    CLIAuth,
    Client,
    CredentialError,
    NoPlatformHostError,
    SignInExpiredError,
    is_token,
    load_credential_config,
    resolve_command,
    resolve_identity,
)

pytestmark = pytest.mark.skipif(sys.platform == "win32", reason="the fake pw is a shell script")


class FakeCLI:
    def __init__(self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
        self.bin = tmp_path / "bin"
        self.bin.mkdir()
        self.cred_dir = tmp_path / "cred"
        self.cred_dir.mkdir()
        self.log = self.bin / "calls"
        monkeypatch.setenv("PATH", f"{self.bin}{os.pathsep}{os.environ['PATH']}")
        monkeypatch.setenv("PW_CREDENTIALS_DIR", str(self.cred_dir))
        for name in ("PW_API_KEY", "PW_CONTEXT", "PW_PLATFORM_HOST"):
            monkeypatch.delenv(name, raising=False)

    @property
    def credentials(self) -> Path:
        return self.cred_dir / ".credentials"

    def script(self, body: str) -> None:
        pw = self.bin / "pw"
        pw.write_text(f'#!/bin/sh\necho "$* key=${{PW_API_KEY:-unset}}" >> {self.log}\n{body}\n')
        pw.chmod(pw.stat().st_mode | stat.S_IEXEC)

    def calls(self) -> list[str]:
        return self.log.read_text().strip().split("\n") if self.log.exists() else []

    def sign_in(self, token: str, expires_in: timedelta, path: Path | None = None) -> None:
        # Nanoseconds, as Go writes them.
        expires_at = (datetime.now(timezone.utc) + expires_in).strftime("%Y-%m-%dT%H:%M:%S.%f123Z")
        doc = {
            "currentIdentity": "work",
            "identities": {
                "work": {
                    "token": token,
                    "server": "work.example.com",
                    "name": "work",
                    "organization": "org",
                    "oauth": {"clientId": "pw-cli", "expiresAt": expires_at, "keychainAccount": "work"},
                },
                "other": {
                    "token": "pwoa_other",
                    "server": "other.example.com",
                    "name": "other",
                    "organization": "org",
                    "oauth": {"expiresAt": "2000-01-01T00:00:00Z"},
                },
                "api": {
                    "apikey": "pwt_aG9zdA==.key",
                    "server": "api.example.com",
                    "name": "api",
                    "organization": "org",
                },
            },
        }
        (path or self.credentials).write_text(json.dumps(doc))

    def print_token(self, token: str, expires_in: int | None = None) -> None:
        """Prints a token response for token; no expires_in leaves it out."""
        response: dict[str, object] = {"access_token": token, "token_type": "Bearer"}
        if expires_in is not None:
            response["expires_in"] = expires_in
        self.script(f"echo '{json.dumps(response)}'")


@pytest.fixture
def cli(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> FakeCLI:
    return FakeCLI(tmp_path, monkeypatch)


@pytest.fixture
def seen(monkeypatch: pytest.MonkeyPatch) -> list[httpx.Request]:
    """Answers every request the client sends, and records it."""
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json=[])

    transport = httpx.MockTransport(handler)
    monkeypatch.setattr(httpx, "Client", functools.partial(httpx.Client, transport=transport))
    monkeypatch.setattr(httpx, "AsyncClient", functools.partial(httpx.AsyncClient, transport=transport))
    return requests


def authorizations(seen: list[httpx.Request], client: Client, times: int = 1) -> list[str]:
    with client.sync() as http:
        for _ in range(times):
            http.get("/api/workflows")
    return [r.headers["Authorization"] for r in seen]


def test_caches_the_token_until_it_nears_expiry(
    cli: FakeCLI, seen: list[httpx.Request], monkeypatch: pytest.MonkeyPatch
):
    cli.sign_in("pwoa_stored", timedelta(hours=1))
    cli.print_token("pwoa_new", 3600)
    client = Client.from_credential_config()
    assert client._config.base_url == "https://work.example.com"
    monkeypatch.setenv("PW_API_KEY", "pwt_leaked")

    assert authorizations(seen, client, 3) == ["Bearer pwoa_new"] * 3
    assert cli.calls() == ["auth token --print -o json --context work key=unset"]


def test_renews_the_token_before_it_expires(cli: FakeCLI, seen: list[httpx.Request], monkeypatch: pytest.MonkeyPatch):
    cli.sign_in("pwoa_stored", timedelta(hours=1))
    cli.print_token("pwoa_new", 3600)
    client = Client.from_credential_config()
    assert authorizations(seen, client) == ["Bearer pwoa_new"]

    now = time.time()
    monkeypatch.setattr(time, "time", lambda: now + 3570)
    cli.print_token("pwoa_newer", 3600)
    assert authorizations(seen, client, 2) == ["Bearer pwoa_new", "Bearer pwoa_newer", "Bearer pwoa_newer"]
    assert len(cli.calls()) == 2


def test_uses_a_token_with_no_expiry_for_the_life_of_the_client(cli: FakeCLI, seen: list[httpx.Request]):
    cli.sign_in("pwoa_stored", timedelta(hours=1))
    cli.print_token("pwoa_new")

    assert authorizations(seen, Client.from_credential_config(), 3) == ["Bearer pwoa_new"] * 3
    assert len(cli.calls()) == 1


def test_asks_for_the_context_the_sdk_selected(
    cli: FakeCLI, seen: list[httpx.Request], monkeypatch: pytest.MonkeyPatch
):
    cli.sign_in("pwoa_old", timedelta(hours=1))
    cli.print_token("pwoa_printed", 3600)
    monkeypatch.setenv("PW_CONTEXT", "work")

    assert authorizations(seen, Client.from_credential_config(context="other")) == ["Bearer pwoa_printed"]
    assert cli.calls() == ["auth token --print -o json --context other key=unset"]


def test_keeps_a_token_the_cli_could_not_renew_until_it_expires(cli: FakeCLI, seen: list[httpx.Request]):
    cli.sign_in("pwoa_stored", timedelta(seconds=30))
    cli.print_token("pwoa_stored", 30)

    assert authorizations(seen, Client.from_credential_config(), 3) == ["Bearer pwoa_stored"] * 3
    assert len(cli.calls()) == 1


def test_uses_the_last_token_until_it_expires_when_the_cli_fails(cli: FakeCLI, monkeypatch: pytest.MonkeyPatch):
    cli.print_token("pwoa_new", 3600)
    auth = CLIAuth(context="work")
    assert auth.token() == "pwoa_new"

    cli.script("exit 1")
    now = time.time()
    monkeypatch.setattr(time, "time", lambda: now + 3570)
    assert auth.token() == "pwoa_new"
    monkeypatch.setattr(time, "time", lambda: now + 3630)
    with pytest.raises(SignInExpiredError, match="exit status 1"):
        auth.token()


def test_fails_without_the_cli(cli: FakeCLI, seen: list[httpx.Request]):
    cli.sign_in("pwoa_stored", timedelta(hours=1))
    with pytest.raises(SignInExpiredError, match="not found"):
        authorizations(seen, Client.from_credential_config(cli_command=str(cli.bin / "missing-pw")))


def test_fails_when_the_cli_cannot_run(cli: FakeCLI):
    not_executable = cli.bin / "not-executable"
    not_executable.write_text("#!/bin/sh\necho pwoa_unexpected\n")
    with pytest.raises(SignInExpiredError):
        CLIAuth(command=str(not_executable)).token()


def test_surfaces_the_cli_error(cli: FakeCLI):
    cli.script("echo 'sign-in revoked' >&2\nexit 1")

    with pytest.raises(SignInExpiredError, match="sign-in revoked"):
        CLIAuth(context="work").token()


@pytest.mark.parametrize(
    "body",
    [
        "echo pwoa_bare",
        """echo '{"token_type":"Bearer"}'""",
        """echo '{"access_token":"pwoa_x","token_type":"mac"}'""",
        """echo '{"access_token":"pwoa x","token_type":"Bearer"}'""",
        "echo '[]'",
    ],
)
def test_rejects_output_that_is_not_a_bearer_token_response(cli: FakeCLI, body: str):
    cli.script(body)
    with pytest.raises(SignInExpiredError):
        CLIAuth().token()


def test_bounds_a_run_of_the_cli(cli: FakeCLI):
    cli.script("exec sleep 5")

    with pytest.raises(SignInExpiredError, match="did not finish within 0.1s"):
        CLIAuth(context="work", timeout=0.1).token()


def test_refuses_a_pw_found_through_a_relative_path_entry(cli: FakeCLI, monkeypatch: pytest.MonkeyPatch):
    cli.print_token("pwoa_new", 3600)
    monkeypatch.setenv("PATH", os.path.relpath(cli.bin))
    with pytest.raises(CredentialError, match="relative to the current directory"):
        resolve_command("pw")
    with pytest.raises(SignInExpiredError):
        CLIAuth().token()
    assert cli.calls() == []

    monkeypatch.setenv("PATH", str(cli.bin))
    assert resolve_command("pw") == str(cli.bin / "pw")
    assert resolve_command("./pw") == "./pw"
    with pytest.raises(CredentialError, match="not found on PATH"):
        resolve_command("missing-pw")


def test_async_requests_renew_too(cli: FakeCLI, seen: list[httpx.Request]):
    cli.sign_in("pwoa_old", timedelta(hours=1))
    cli.print_token("pwoa_new", 3600)

    async def request() -> None:
        async with Client.from_credential_config() as client:
            await client.get("/api/workflows")
            await client.get("/api/workflows")

    asyncio.run(request())
    assert [r.headers["Authorization"] for r in seen] == ["Bearer pwoa_new"] * 2
    assert len(cli.calls()) == 1


def test_api_key_context_and_pw_api_key_are_used_as_is(
    cli: FakeCLI, seen: list[httpx.Request], monkeypatch: pytest.MonkeyPatch
):
    cli.script("echo pwoa_unexpected")
    cli.sign_in("pwoa_stored", timedelta(hours=1))

    assert authorizations(seen, Client.from_credential_config(context="api"))[0].startswith("Basic ")
    monkeypatch.setenv("PW_API_KEY", "pwoa_explicit")
    client = Client.from_credential_config()
    assert client._config.base_url == "https://work.example.com"
    assert authorizations(seen, client)[1] == "Bearer pwoa_explicit"
    assert cli.calls() == []


def test_resolves_like_the_cli(cli: FakeCLI, monkeypatch: pytest.MonkeyPatch):
    cli.sign_in("pwoa_stored", timedelta(hours=1))
    config = load_credential_config()

    assert resolve_identity(config)[0] == "work"
    monkeypatch.setenv("PW_CONTEXT", "other")
    assert resolve_identity(config)[0] == "other"
    assert resolve_identity(config, context="api")[0] == "api"
    with pytest.raises(CredentialError):
        resolve_identity(config, context="nope")
    monkeypatch.setenv("PW_API_KEY", "pwt_aG9zdA==.key")
    name, identity = resolve_identity(config)
    assert (name, identity.apikey, identity.server) == ("", "pwt_aG9zdA==.key", "host")


def test_missing_or_unparseable_file_has_no_contexts(cli: FakeCLI):
    assert load_credential_config().identities == {}
    cli.credentials.write_text("{not json")
    assert load_credential_config().identities == {}


def test_xdg_path(cli: FakeCLI, tmp_path: Path, monkeypatch: pytest.MonkeyPatch):
    monkeypatch.delenv("PW_CREDENTIALS_DIR")
    monkeypatch.setenv("XDG_CONFIG_HOME", str(tmp_path / "xdg"))
    assert load_credential_config().path == str(tmp_path / "xdg" / "pw" / "credentials")


def test_access_token_names_no_host(cli: FakeCLI, monkeypatch: pytest.MonkeyPatch):
    assert is_token("pwoa_abc")
    with pytest.raises(NoPlatformHostError):
        Client.from_credential("pwoa_abc")

    cli.sign_in("pwoa_stored", timedelta(hours=1))
    client = Client.from_credential("pwoa_abc")
    assert client._config.base_url == "https://work.example.com"
    assert client._config.auth_header == "Bearer pwoa_abc"
    monkeypatch.setenv("PW_PLATFORM_HOST", "env.example.com")
    assert Client.from_credential("pwoa_abc")._config.base_url == "https://env.example.com"
