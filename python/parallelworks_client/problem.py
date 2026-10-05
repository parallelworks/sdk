"""RFC 9457 problem details: the errors the Parallel Works API returns."""

from __future__ import annotations

import json
import os
import re
from collections.abc import Mapping
from dataclasses import dataclass, field
from typing import Any
from urllib.parse import unquote, urljoin

import httpx

PROBLEM_MEDIA_TYPE = "application/problem+json"
BLANK_PROBLEM_TYPE = "about:blank"

_LANGUAGE_TAG = re.compile(r"^[A-Za-z0-9_-]+$")


def accept_language_from_env(env: Mapping[str, str] | None = None) -> str | None:
    """
    The language of the POSIX locale environment (LC_ALL, then LC_MESSAGES,
    then LANG) as a language tag: "ja_JP.UTF-8" is "ja-JP". None when none is
    set or the locale is C or POSIX.
    """
    env = os.environ if env is None else env
    locale = env.get("LC_ALL") or env.get("LC_MESSAGES") or env.get("LANG")
    if not locale:
        return None
    tag = locale.split(".", 1)[0].split("@", 1)[0]
    if tag in ("", "C", "POSIX") or not _LANGUAGE_TAG.match(tag):
        return None
    return tag.replace("_", "-")


def code_for_status(status: int) -> str:
    """The code of an about:blank problem, which the server leaves out."""
    codes = {
        401: "unauthenticated",
        403: "forbidden",
        404: "not_found",
        409: "conflict",
        429: "rate_limited",
        503: "unavailable",
    }
    if status in codes:
        return codes[status]
    return "invalid_request" if 400 <= status < 500 else "internal"


@dataclass(frozen=True)
class FieldError:
    """One invalid field or parameter of a validation problem."""

    code: str
    type: str = ""
    detail: str = ""
    pointer: str | None = None
    parameter: str | None = None
    location: str | None = None
    params: dict[str, Any] = field(default_factory=dict)

    @property
    def path(self) -> str:
        """The field as a path such as items[0].name, or the parameter's name."""
        if self.pointer:
            return _pointer_path(self.pointer)
        return self.parameter or ""

    def __str__(self) -> str:
        message = self.detail or self.code
        return f"{self.path}: {message}" if self.path else message


class ProblemError(Exception):
    """
    A failed request, read as RFC 9457 problem details. The older
    {error, message, code, errors} envelope and bodies from outside the API,
    such as a proxy's, are read into the same shape with type about:blank.
    """

    def __init__(
        self,
        status: int,
        *,
        type: str = BLANK_PROBLEM_TYPE,
        code: str | None = None,
        title: str | None = None,
        detail: str | None = None,
        params: dict[str, Any] | None = None,
        errors: list[FieldError] | None = None,
        language: str | None = None,
        response: httpx.Response | None = None,
    ) -> None:
        self.status = status
        self.type = type or BLANK_PROBLEM_TYPE
        self.code = code or code_for_status(status)
        self.title = title
        self.detail = detail or title or httpx.codes.get_reason_phrase(status) or f"HTTP {status}"
        self.params = params or {}
        self.errors = errors or []
        self.language = language
        self.response = response
        super().__init__(self.detail)

    def __str__(self) -> str:
        return "; ".join([self.detail, *(str(e) for e in self.errors)])

    def type_url(self, base_url: str) -> str | None:
        """The URL documenting the problem's type, or None for about:blank."""
        if self.type == BLANK_PROBLEM_TYPE:
            return None
        return urljoin(base_url, self.type)

    @classmethod
    def from_response(cls, response: httpx.Response) -> ProblemError:
        """Reads a failed response's body, whatever shape it has."""
        try:
            body = json.loads(response.content)
        except ValueError:
            body = None
        if not isinstance(body, dict):
            body = {}
        status = body.get("status") if isinstance(body.get("status"), int) else response.status_code
        common = {
            "language": response.headers.get("Content-Language"),
            "response": response,
            "params": _dict(body.get("params")),
        }
        if isinstance(body.get("type"), str):
            return cls(
                status,
                type=body["type"],
                code=_str(body.get("code")),
                title=_str(body.get("title")),
                detail=_str(body.get("detail")),
                errors=[_field_error(e) for e in body.get("errors") or [] if isinstance(e, dict)],
                **common,
            )
        errors = [FieldError(code="", detail=e) for e in body.get("errors") or [] if isinstance(e, str)]
        return cls(status, code=_str(body.get("code")), detail=_str(body.get("message")), errors=errors, **common)


def raise_for_problem(response: httpx.Response) -> httpx.Response:
    """Raises a ProblemError for a response outside 2xx, and returns any other."""
    if not response.is_success:
        raise ProblemError.from_response(response)
    return response


def _field_error(e: dict[str, Any]) -> FieldError:
    return FieldError(
        code=_str(e.get("code")) or "",
        type=_str(e.get("type")) or "",
        detail=_str(e.get("detail")) or "",
        pointer=_str(e.get("pointer")),
        parameter=_str(e.get("parameter")),
        location=_str(e.get("in")),
        params=_dict(e.get("params")),
    )


def _str(v: Any) -> str | None:
    return v if isinstance(v, str) and v else None


def _dict(v: Any) -> dict[str, Any]:
    return dict(v) if isinstance(v, dict) else {}


def _pointer_path(pointer: str) -> str:
    raw = pointer.removeprefix("#")
    if not raw:
        return ""
    path = ""
    for segment in raw.removeprefix("/").split("/"):
        s = unquote(segment).replace("~1", "/").replace("~0", "~")
        if s.isdigit() and path:
            path += f"[{s}]"
        else:
            path += f".{s}" if path else s
    return path
