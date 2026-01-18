# Parallel Works Go Client

Official Go client for the Parallel Works ACTIVATE platform API.

## Installation

```bash
go get github.com/parallelworks/client-go
```

## Quick Start

The simplest way to create a client - just pass your credential:

```go
import parallelworks "github.com/parallelworks/client-go"

// The platform host is automatically extracted from your credential
client, err := parallelworks.NewClientFromCredential(os.Getenv("PW_API_KEY"))
if err != nil {
    log.Fatal(err)
}

resp, err := client.GetBucketsWithResponse(context.Background())
```

See the [examples](./examples) directory for complete runnable examples.

## Authentication

### Automatic Host Detection

API keys (`pwt_...`) and JWT tokens contain the platform host encoded within them. Use `NewClientFromCredential` to automatically extract it:

```go
// API key - host decoded from first segment after pwt_
client, _ := parallelworks.NewClientFromCredential("pwt_Y2xvdWQucGFyYWxsZWwud29ya3M.xxxxx")
// Connects to: https://activate.parallel.works

// JWT token - host read from platform_host claim
client, _ := parallelworks.NewClientFromCredential("eyJhbGci...")
// Connects to the host in the token's platform_host claim
```

### Explicit Host

If you prefer to specify the host explicitly:

```go
// API Key (Basic Auth) - best for long-running integrations
client, _ := parallelworks.NewClientWithResponses(
    "https://activate.parallel.works",
    parallelworks.WithAPIKey("pwt_..."),
)

// JWT Token (Bearer) - best for scripts, expires in 24h
client, _ := parallelworks.NewClientWithResponses(
    "https://activate.parallel.works",
    parallelworks.WithToken("eyJhbGci..."),
)

// Auto-detect credential type
client, _ := parallelworks.NewClientWithResponses(
    "https://activate.parallel.works",
    parallelworks.WithCredential(os.Getenv("PW_CREDENTIAL")),
)
```

### Credential Helpers

```go
parallelworks.IsAPIKey("pwt_abc.xyz")           // true
parallelworks.IsToken("eyJ.abc.def")            // true
parallelworks.ExtractPlatformHost("pwt_...")    // "activate.parallel.works"
```

## Documentation

For full API documentation, visit [https://parallelworks.com/docs](https://parallelworks.com/docs).

## License

MIT
