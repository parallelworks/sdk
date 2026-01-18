# TypeScript Client Examples

## Prerequisites

- Node.js 18 or later
- pnpm (recommended) or npm
- A Parallel Works API key or token (from your ACTIVATE account settings)

## Setup

```bash
# Install dependencies
pnpm install
```

## Running the Examples

```bash
# Set your API key or token
export PW_API_KEY="your-api-key-or-token"

# Run the example
pnpm list-resources

# Or directly with tsx
pnpm tsx list-resources.ts
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
