# Parallel Works Go Client

Official Go client for the Parallel Works ACTIVATE platform API.

## Requirements

- Go 1.18 or later

## Installation

```bash
go get github.com/parallelworks/sdk/go/v7
```

## Quick Start

The simplest way to create a client — just pass your credential:

```go
import parallelworks "github.com/parallelworks/sdk/go/v7"

// The platform host is automatically extracted from your credential
client, err := parallelworks.NewClientFromCredential(os.Getenv("PW_API_KEY"))
if err != nil {
    log.Fatal(err)
}

workflows, err := client.ListWorkflows(context.Background())
```

## Authentication

### Automatic Host Detection

API keys (`pwt_...`) and JWT tokens contain the platform host encoded within them. Use `NewClientFromCredential` to automatically extract it:

```go
// API key — host decoded from first segment after pwt_
client, _ := parallelworks.NewClientFromCredential("pwt_Y2xvdWQucGFyYWxsZWwud29ya3M.xxxxx")
// Connects to: https://cloud.parallel.works

// JWT token — host read from platform_host claim
client, _ := parallelworks.NewClientFromCredential("eyJhbGci...")
// Connects to the host in the token's platform_host claim
```

### Explicit Host

If you prefer to specify the host explicitly:

```go
// API Key (Basic Auth)
client := parallelworks.NewClient(
    "https://cloud.parallel.works",
    parallelworks.WithAuth(&parallelworks.BasicAuth{
        Username: "pwt_...",
    }),
)

// JWT Token (Bearer)
client := parallelworks.NewClient(
    "https://cloud.parallel.works",
    parallelworks.WithAuth(&parallelworks.BearerAuth{
        Token: "eyJhbGci...",
    }),
)
```

### Credential Helpers

```go
parallelworks.IsAPIKey("pwt_abc.xyz")        // true
parallelworks.IsToken("eyJ.abc.def")         // true
parallelworks.ExtractPlatformHost("pwt_...")  // "cloud.parallel.works", nil
```

## Client Options

```go
client := parallelworks.NewClient(
    "https://cloud.parallel.works",
    parallelworks.WithAuth(&parallelworks.BasicAuth{Username: "pwt_..."}),
    parallelworks.WithHTTPClient(customHTTPClient),
    parallelworks.WithUserAgent("my-app/1.0"),
    parallelworks.WithDefaultRetry(),
    parallelworks.WithMiddleware(loggingMiddleware),
)
```

### Retry

Enable automatic retries with exponential backoff:

```go
// Use default retry config (3 retries, retries on 429/5xx)
client := parallelworks.NewClient(url, parallelworks.WithDefaultRetry())

// Or customize
client := parallelworks.NewClient(url, parallelworks.WithRetry(parallelworks.RetryConfig{
    MaxRetries:           5,
    BaseDelay:            2 * time.Second,
    MaxDelay:             60 * time.Second,
    Multiplier:           2.0,
    RetryableStatusCodes: []int{429, 500, 502, 503, 504},
}))
```

### Middleware

Add custom middleware to intercept requests:

```go
logging := func(req *http.Request, next parallelworks.RoundTripFunc) (*http.Response, error) {
    log.Printf("%s %s", req.Method, req.URL)
    return next(req)
}

client := parallelworks.NewClient(url, parallelworks.WithMiddleware(logging))
```

## Error Handling

API errors can be matched with sentinel errors using `errors.Is`:

```go
_, err := client.ListWorkflows(ctx)
if errors.Is(err, parallelworks.ErrNotFound) {
    // handle 404
} else if errors.Is(err, parallelworks.ErrUnauthorized) {
    // handle 401
}
```

## Documentation

For full API documentation, visit [https://parallelworks.com/docs](https://parallelworks.com/docs).

## License

MIT
