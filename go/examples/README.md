# Go Client Examples

## Prerequisites

- Go 1.23 or later
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
Fetching organizations...

Found 2 organization(s):

  - My Organization (ID: 507f1f77bcf86cd799439011)
  - Another Org (ID: 507f1f77bcf86cd799439012)
```
