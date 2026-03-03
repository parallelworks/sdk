# Parallel Works SDKs

[![npm](https://img.shields.io/npm/v/@parallelworks/client)](https://www.npmjs.com/package/@parallelworks/client)
[![PyPI](https://img.shields.io/pypi/v/parallelworks-client)](https://pypi.org/project/parallelworks-client/)
[![Go Reference](https://pkg.go.dev/badge/github.com/parallelworks/sdk/go/v7.svg)](https://pkg.go.dev/github.com/parallelworks/sdk/go/v7)
[![API Docs](https://img.shields.io/badge/API_Docs-parallelworks.com-1099E3?style=flat-square&logo=data:image/svg%2bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCA2NC4yOSA2NC4yOSI+PGRlZnM+PHN0eWxlPi5jbHMtMXtmaWxsOm5vbmU7c3Ryb2tlOiMxMDk5RTM7c3Ryb2tlLW1pdGVybGltaXQ6MTA7c3Ryb2tlLXdpZHRoOjMuMnB4O308L3N0eWxlPjwvZGVmcz48ZyBpZD0iTGF5ZXJfMiIgZGF0YS1uYW1lPSJMYXllciAyIj48ZyBpZD0iTGF5ZXJfMS0yIiBkYXRhLW5hbWU9IkxheWVyIDEiPjxjaXJjbGUgY2xhc3M9ImNscy0xIiBjeD0iMzIuMTQiIGN5PSIzMi4xNCIgcj0iMzAuNTQiLz48cGF0aCBjbGFzcz0iY2xzLTEiIGQ9Ik0xNy4yNyw1OC4xMnYtMjZBMTQuODgsMTQuODgsMCwxLDEsMzIuMTQsNDdIMjQuODkiLz48cGF0aCBjbGFzcz0iY2xzLTEiIGQ9Ik05LjI1LDUyLjM1VjMyLjE0QTIyLjksMjIuOSwwLDEsMSwzMi4xNCw1NUgyNC44OVYzMi4xNGE3LjI1LDcuMjUsMCwxLDEsNy4yNSw3LjI1aC02Ii8+PC9nPjwvZz48L3N2Zz4=)](https://parallelworks.com/api)
[![Support](https://img.shields.io/badge/Support-support%40parallelworks.com-1099E3?style=flat-square&logo=maildotru&logoColor=cdd6f4)](mailto:support@parallelworks.com)

Official API clients for the [Parallel Works ACTIVATE](https://parallelworks.com) platform. All clients are auto-generated from the [OpenAPI specification](./openapi.json) and provide typed access to the full platform API. The [Parallel Works CLI](https://parallelworks.com/docs/cli) is built with the same Go SDK shipped here.

## Installation

```bash
npm install @parallelworks/client    # TypeScript
pip install parallelworks-client     # Python
go get github.com/parallelworks/sdk/go/v7  # Go
```

## Quick Start

Every client supports automatic host detection from your API key or token:

### TypeScript

```typescript
import { Client } from '@parallelworks/client'

const client = Client.fromCredential(process.env.PW_API_KEY!)
const { data } = await client.GET('/api/buckets')
```

### Python

```python
from parallelworks_client import Client

with Client.from_credential(os.environ["PW_API_KEY"]).sync() as client:
    buckets = client.get("/api/buckets").json()
```

### Go

```go
import parallelworks "github.com/parallelworks/sdk/go/v7"

client, _ := parallelworks.NewClientFromCredential(os.Getenv("PW_API_KEY"))
buckets, _ := client.GetBuckets(context.Background())
```

See each client's README for full documentation on authentication, error handling, retries, and more:
- [TypeScript](./typescript/README.md)
- [Python](./python/README.md)
- [Go](./go/README.md)

## Authentication

All clients accept either an **API key** (`pwt_...`) or an **auth token** and can automatically extract the platform host from the credential. You can also configure the host explicitly. See the [authentication guide](https://parallelworks.com/api/authenticate) and the individual client docs for details.

## Version Compatibility

SDK versions match the Parallel Works platform version. Minor and patch releases may add new API surface but will not introduce breaking changes. In rare cases, breaking changes may occur in minor or patch releases to address security vulnerabilities or critical bugs — these will be clearly documented in the release notes. Breaking changes are otherwise reserved for major version bumps. Only stable releases are published to package registries.

## Documentation

- [API Reference](https://parallelworks.com/api)
- [User Guide](https://parallelworks.com/docs)

## License

MIT License - see [LICENSE](./LICENSE) for details.
