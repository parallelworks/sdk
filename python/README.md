# parallelworks-client

Official Python client for the Parallel Works ACTIVATE platform API.

## Installation

```bash
pip install parallelworks-client
```

## Usage

### Synchronous

```python
import os
from parallelworks_client import Client

# Create a client with API Key authentication
with Client.with_api_key(
    "https://cloud.parallel.works",
    os.environ["PW_API_KEY"]
).sync() as client:
    response = client.get("/api/organizations")
    response.raise_for_status()

    for org in response.json():
        print(f"Organization: {org['name']}")
```

### Asynchronous

```python
import asyncio
import os
from parallelworks_client import Client

async def main():
    async with Client.with_api_key(
        "https://cloud.parallel.works",
        os.environ["PW_API_KEY"]
    ) as client:
        response = await client.get("/api/organizations")
        response.raise_for_status()

        for org in response.json():
            print(f"Organization: {org['name']}")

asyncio.run(main())
```

## Authentication

The client supports two authentication methods:

### API Key (Basic Auth)

Best for long-running integrations with configurable expiration. API keys can be generated from your ACTIVATE account settings.

```python
client = Client.with_api_key(
    "https://cloud.parallel.works",
    "your-api-key"
)
```

### Bearer Token (JWT)

Best for scripts and CLI tools. Tokens expire after 24 hours and can be generated from your ACTIVATE account settings.

```python
client = Client.with_token(
    "https://cloud.parallel.works",
    "your-jwt-token"
)
```

## Configuration

You can customize the request timeout:

```python
client = Client.with_api_key(
    "https://cloud.parallel.works",
    "your-api-key",
    timeout=60.0  # 60 second timeout
)
```

## Documentation

For full API documentation, visit [https://parallelworks.com/docs](https://parallelworks.com/docs).

## License

MIT
