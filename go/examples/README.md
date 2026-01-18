# Go Client Examples

## Prerequisites

- Go 1.23 or later
- A Parallel Works API key (generate from your ACTIVATE account settings)

## Running the Example

```bash
# Set your API key
export PW_API_KEY="your-api-key"

# Optionally set a custom host (defaults to cloud.parallel.works)
export PW_HOST="https://your-instance.parallel.works"

# Run the example
go run main.go
```

## Expected Output

```
Fetching organizations...

Found 2 organization(s):

  - My Organization (ID: 507f1f77bcf86cd799439011)
  - Another Org (ID: 507f1f77bcf86cd799439012)
```
