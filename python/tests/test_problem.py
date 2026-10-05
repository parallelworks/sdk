import json

import httpx
import pytest

from parallelworks_client import Client, ProblemError, accept_language_from_env, raise_for_problem


def response(status: int, body: str, content_type: str = "application/problem+json", **headers: str) -> httpx.Response:
    return httpx.Response(
        status,
        content=body.encode(),
        headers={"Content-Type": content_type, **headers},
        request=httpx.Request("GET", "https://activate.parallel.works/api/workflows"),
    )


def test_reads_a_problem():
    body = {
        "type": "/problems/activate/workflow_not_found",
        "status": 404,
        "detail": "ワークフローが見つかりません",
        "code": "workflow_not_found",
        "params": {"name": "x"},
    }
    with pytest.raises(ProblemError) as caught:
        raise_for_problem(response(404, json.dumps(body), **{"Content-Language": "ja"}))
    p = caught.value
    assert p.status == 404
    assert p.code == "workflow_not_found"
    assert p.detail == "ワークフローが見つかりません"
    assert p.params == {"name": "x"}
    assert p.language == "ja"
    assert (
        p.type_url("https://activate.parallel.works")
        == "https://activate.parallel.works/problems/activate/workflow_not_found"
    )


def test_derives_the_code_of_about_blank():
    p = ProblemError.from_response(response(403, '{"type":"about:blank","status":403,"title":"Forbidden"}'))
    assert p.code == "forbidden"
    assert p.detail == "Forbidden"
    assert p.type_url("https://activate.parallel.works") is None


def test_reads_field_errors():
    body = {
        "type": "/problems/activate/validation",
        "status": 422,
        "detail": "validation failed",
        "code": "validation",
        "errors": [
            {
                "type": "/problems/activate/too_long",
                "code": "too_long",
                "detail": "too long",
                "pointer": "#/items/0/name",
                "params": {"max": 3},
            },
            {"type": "/problems/activate/required", "code": "required", "parameter": "org", "in": "query"},
        ],
    }
    p = ProblemError.from_response(response(422, json.dumps(body)))
    assert [(e.path, e.code, e.params) for e in p.errors] == [
        ("items[0].name", "too_long", {"max": 3}),
        ("org", "required", {}),
    ]
    assert p.errors[1].location == "query"
    assert str(p) == "validation failed; items[0].name: too long; org: required"


def test_reads_the_older_envelope():
    body = '{"error":true,"message":"Workflow not found","code":"workflow_not_found","errors":["no such workflow"]}'
    p = ProblemError.from_response(response(404, body, "application/json"))
    assert p.code == "workflow_not_found"
    assert p.detail == "Workflow not found"
    assert p.type == "about:blank"
    assert [str(e) for e in p.errors] == ["no such workflow"]


def test_reads_a_proxy_page():
    p = ProblemError.from_response(response(502, "<html>Bad Gateway</html>", "text/html"))
    assert p.status == 502
    assert p.code == "internal"
    assert p.detail == "Bad Gateway"


def test_passes_a_success_through():
    ok = response(200, "[]", "application/json")
    assert raise_for_problem(ok) is ok


@pytest.mark.parametrize(
    ("env", "want"),
    [
        ({"LANG": "ja_JP.UTF-8"}, "ja-JP"),
        ({"LANG": "de_DE.UTF-8@euro"}, "de-DE"),
        ({"LANG": "es"}, "es"),
        ({"LANG": "C"}, None),
        ({"LANG": "C.UTF-8"}, None),
        ({"LANG": "POSIX"}, None),
        ({}, None),
        ({"LC_MESSAGES": "ko_KR.UTF-8", "LANG": "en_US.UTF-8"}, "ko-KR"),
        ({"LC_ALL": "fr_FR.UTF-8", "LC_MESSAGES": "ko_KR.UTF-8"}, "fr-FR"),
    ],
)
def test_accept_language_from_env(env, want):
    assert accept_language_from_env(env) == want


def test_client_asks_for_problems_in_the_users_language(monkeypatch):
    monkeypatch.setenv("LC_ALL", "ja_JP.UTF-8")
    with Client.with_token("https://activate.parallel.works", "a.b.c").sync() as client:
        headers = client._client.headers
    assert "application/problem+json" in headers["Accept"]
    assert headers["Accept-Language"] == "ja-JP"


def test_client_omits_the_language_for_the_c_locale(monkeypatch):
    monkeypatch.setenv("LC_ALL", "C")
    with Client.with_token("https://activate.parallel.works", "a.b.c").sync() as client:
        assert "Accept-Language" not in client._client.headers
