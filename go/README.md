# Parallel Works Go SDK

Official Go SDK for the Parallel Works ACTIVATE platform API.

## Installation

```bash
go get github.com/parallelworks/sdk/go
```

## Usage

```go
package main

import (
    "context"
    "fmt"
    "net/http"

    parallelworks "github.com/parallelworks/sdk/go"
)

func main() {
    platformHost := "https://your-instance.parallel.works"
    apiKey := "YOUR_API_KEY" // Generate token from ACTIVATE UI
    // Create a client with authentication
    client, err := parallelworks.NewClientWithResponses(
        platformHost,
        parallelworks.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
            req.Header.Set("Authorization", "Bearer " + apiKey)
            return nil
        }),
    )
    if err != nil {
        panic(err)
    }

    // Make API calls
    resp, err := client.GetOrganizationsWithResponse(context.Background())
    if err != nil {
        panic(err)
    }

    if resp.JSON200 != nil {
        for _, org := range *resp.JSON200 {
            fmt.Printf("Organization: %s\n", org.Name)
        }
    }
}
```

## Authentication

The SDK supports three authentication methods:

1. **API Key (Basic Auth)**: Use your API key as the password
2. **Bearer Token (JWT)**: Use a JWT token in the Authorization header
3. **Session Cookie**: Use session-based authentication

## Documentation

For full API documentation, visit [https://parallelworks.com/docs](https://parallelworks.com/docs).

## License

MIT
