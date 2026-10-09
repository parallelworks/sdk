import base64

import pytest

from parallelworks_client import USER_TOKEN_PREFIX, Client, CredentialError, extract_platform_host, is_api_key, is_token


def user_token(host: str) -> str:
    return f"{USER_TOKEN_PREFIX}{base64.b64encode(host.encode()).decode()}.{base64.b64encode(b'raw').decode()}"


def test_user_token_is_a_token_not_an_api_key():
    token = user_token("activate.parallel.works")
    assert is_token(token)
    assert not is_api_key(token)


def test_user_token_names_its_platform_host():
    assert extract_platform_host(user_token("activate.parallel.works")) == "activate.parallel.works"
    with pytest.raises(CredentialError):
        extract_platform_host(f"{USER_TOKEN_PREFIX}no-dot")


def test_user_token_is_sent_as_a_bearer_token_to_the_host_it_names():
    token = user_token("activate.parallel.works")
    client = Client.from_credential(token)
    assert client._config.base_url == "https://activate.parallel.works"
    assert client._config.auth_header == f"Bearer {token}"
