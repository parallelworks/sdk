# @parallelworks/client

Official TypeScript client for the Parallel Works ACTIVATE platform API.

## Installation

```bash
npm install @parallelworks/client
```

For SWR hooks support (React):

```bash
npm install @parallelworks/client swr swr-openapi
```

## Quick Start

The simplest way to create a client - just pass your credential:

```typescript
import { Client } from '@parallelworks/client'

// The platform host is automatically extracted from your credential
const client = Client.fromCredential(process.env.PW_API_KEY!)

const { data, error } = await client.GET('/api/buckets')
```

See the [examples](./examples) directory for complete runnable examples.

## Authentication

### Automatic Host Detection

API keys (`pwt_...`) and JWT tokens contain the platform host encoded within them. Use `fromCredential` to automatically extract it:

```typescript
// API key - host decoded from first segment after pwt_
const client = Client.fromCredential('pwt_Y2xvdWQucGFyYWxsZWwud29ya3M.xxxxx')
// Connects to: https://cloud.parallel.works

// JWT token - host read from platform_host claim
const client = Client.fromCredential('eyJhbGci...')
// Connects to the host in the token's platform_host claim
```

### Explicit Host

If you prefer to specify the host explicitly:

```typescript
// API Key (Basic Auth) - best for long-running integrations
const client = new Client('https://cloud.parallel.works')
  .withApiKey('pwt_...')

// JWT Token (Bearer) - best for scripts, expires in 24h
const client = new Client('https://cloud.parallel.works')
  .withToken('eyJhbGci...')

// Auto-detect credential type
const client = new Client('https://cloud.parallel.works')
  .withCredential(process.env.PW_CREDENTIAL!)
```

### Credential Helpers

```typescript
import { isApiKey, isToken, extractPlatformHost } from '@parallelworks/client'

isApiKey('pwt_abc.xyz')           // true
isToken('eyJ.abc.def')            // true
extractPlatformHost('pwt_...')    // "cloud.parallel.works"
```

## SWR Hooks (React)

```tsx
// lib/api.ts
import { Client } from '@parallelworks/client'
import { createSwrHooks } from '@parallelworks/client/swr'

// Credential should be securely provided by your server (e.g., via session)
const client = Client.fromCredential(credential)

export const { useQuery, useImmutable, useInfinite } = createSwrHooks(client)
```

```tsx
// components/BucketList.tsx
import { useQuery } from '@/lib/api'

export function BucketList() {
  const { data, error, isLoading } = useQuery('/api/buckets')

  if (isLoading) return <div>Loading...</div>
  if (error) return <div>Error: {error.message}</div>

  return (
    <ul>
      {data?.map(bucket => (
        <li key={bucket.id}>{bucket.name}</li>
      ))}
    </ul>
  )
}
```

## Documentation

For full API documentation, visit [https://parallelworks.com/docs](https://parallelworks.com/docs).

## License

MIT
