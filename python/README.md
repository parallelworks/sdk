# parallelworks-client

Official Python client for the Parallel Works ACTIVATE platform API.

## Installation

```bash
pip install parallelworks-client
```

## Quick Start

The simplest way to create a client - just pass your credential:

```python
import os
from parallelworks_client import Client

# The platform host is automatically extracted from your credential
with Client.from_credential(os.environ["PW_API_KEY"]).sync() as client:
    response = client.get("/api/buckets")
    for bucket in response.json():
        print(f"Bucket: {bucket['name']}")
```

See the [examples](./examples) directory for complete runnable examples.

## Authentication

### Automatic Host Detection

API keys (`pwt_...`) and JWT tokens contain the platform host encoded within them. Use `from_credential` to automatically extract it:

```python
# API key - host decoded from first segment after pwt_
client = Client.from_credential("pwt_Y2xvdWQucGFyYWxsZWwud29ya3M.xxxxx")
# Connects to: https://cloud.parallel.works

# JWT token - host read from platform_host claim
client = Client.from_credential("eyJhbGci...")
# Connects to the host in the token's platform_host claim
```

### Explicit Host

If you prefer to specify the host explicitly:

```python
# API Key (Basic Auth) - best for long-running integrations
client = Client.with_api_key(
    "https://cloud.parallel.works",
    "pwt_..."
)

# JWT Token (Bearer) - best for scripts, expires in 24h
client = Client.with_token(
    "https://cloud.parallel.works",
    "eyJhbGci..."
)

# Auto-detect credential type
client = Client.with_credential(
    "https://cloud.parallel.works",
    os.environ["PW_CREDENTIAL"]
)
```

### Credential Helpers

```python
from parallelworks_client import is_api_key, is_token, extract_platform_host

is_api_key("pwt_abc.xyz")           # True
is_token("eyJ.abc.def")             # True
extract_platform_host("pwt_...")    # "cloud.parallel.works"
```

## Async Support

```python
import asyncio
from parallelworks_client import Client

async def main():
    async with Client.from_credential(os.environ["PW_API_KEY"]) as client:
        response = await client.get("/api/buckets")
        print(response.json())

asyncio.run(main())
```

## Errors

The client asks for errors as [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) problem details, in the language of the locale environment (`LC_ALL`, `LC_MESSAGES`, `LANG`). `raise_for_problem` raises any failed response, including an older server's error envelope, as a `ProblemError` with a stable `code`, `params`, the server's `detail`, and the invalid fields in `errors`:

```python
from parallelworks_client import Client, ProblemError, raise_for_problem

with Client.from_credential(os.environ["PW_API_KEY"]).sync() as client:
    try:
        raise_for_problem(client.get("/api/buckets"))
    except ProblemError as problem:
        print(problem.code, problem.detail)
        for field in problem.errors:
            print(f"  {field}")
```

## Documentation

For full API documentation, visit [https://parallelworks.com/docs](https://parallelworks.com/docs).

## License

Apache-2.0
