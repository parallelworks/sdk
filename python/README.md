# parallelworks-sdk

Official Python SDK for the Parallel Works ACTIVATE platform API.

## Installation

```bash
pip install parallelworks-sdk
```

## Usage

```python
from parallelworks_client import Client

# Create a client
import os

base_url = "https://your-instance.parallel.works"
api_key = os.getenv("PW_API_KEY") # Generate token from ACTIVATE UI
client = Client(
    base_url=base_url,
    headers={"Authorization": f"Bearer {api_key}"}
)

# Make API calls
organizations = client.get_organizations()
for org in organizations:
    print(f"Organization: {org.name}")
```

## Authentication

The SDK supports three authentication methods:

1. **API Key (Basic Auth)**: Use your API key as the password
2. **Bearer Token (JWT)**: Use a JWT token in the Authorization header
3. **Session Cookie**: Use session-based authentication

## Async Support

The SDK supports async operations:

```python
import asyncio
from parallelworks_client import AsyncClient

async def main():
    async with AsyncClient(
        base_url="https://your-instance.parallel.works",
        headers={"Authorization": "Bearer YOUR_API_KEY"}
    ) as client:
        organizations = await client.get_organizations()
        for org in organizations:
            print(f"Organization: {org.name}")

asyncio.run(main())
```

## Documentation

For full API documentation, visit [https://parallelworks.com/docs](https://parallelworks.com/docs).

## License

MIT
