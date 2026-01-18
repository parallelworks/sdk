# Parallel Works Go Client

Official Go client for the Parallel Works ACTIVATE platform API.

## Installation

```bash
go get github.com/parallelworks/client-go
```

## Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    parallelworks "github.com/parallelworks/client-go"
)

func main() {
    // Create a client with API Key authentication
    client, err := parallelworks.NewClientWithResponses(
        "https://cloud.parallel.works",
        parallelworks.WithAPIKey("your-api-key"),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Make API calls
    resp, err := client.GetOrganizationsWithResponse(context.Background())
    if err != nil {
        log.Fatal(err)
    }

    if resp.JSON200 != nil {
        for _, org := range *resp.JSON200 {
            fmt.Printf("Organization: %s\n", org.Name)
        }
    }
}
```

## Authentication

The client supports two authentication methods via functional options:

### API Key (Basic Auth)

Best for long-running integrations with configurable expiration. API keys can be generated from your ACTIVATE account settings.

```go
client, err := parallelworks.NewClientWithResponses(
    "https://cloud.parallel.works",
    parallelworks.WithAPIKey("your-api-key"),
)
```

### Bearer Token (JWT)

Best for scripts and CLI tools. Tokens expire after 24 hours and can be generated from your ACTIVATE account settings.

```go
client, err := parallelworks.NewClientWithResponses(
    "https://cloud.parallel.works",
    parallelworks.WithToken("your-jwt-token"),
)
```

## Advanced Configuration

You can combine multiple options:

```go
client, err := parallelworks.NewClientWithResponses(
    "https://cloud.parallel.works",
    parallelworks.WithAPIKey("your-api-key"),
    parallelworks.WithHTTPClient(&http.Client{
        Timeout: 30 * time.Second,
    }),
)
```

## Documentation

For full API documentation, visit [https://parallelworks.com/docs](https://parallelworks.com/docs).

## License

MIT
