# Go Client Examples

## Prerequisites

- Go 1.18 or later
- A Parallel Works API key or token (from your ACTIVATE account settings)

## Running the Example

```bash
# Set your API key or token
export PW_API_KEY="your-api-key-or-token"

# Run the example
go run main.go
```

The platform host is automatically extracted from your credential.

## Expected Output

```
Fetching resources...

Buckets (2):
  - my-data-bucket (AWS)
  - archive-bucket (GCP)

Clusters (1):
  - dev-cluster (on)
```
