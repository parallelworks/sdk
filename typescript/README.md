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

## Usage

### Basic Client

```typescript
import { Client } from '@parallelworks/client'

// Using API Key (Basic Auth) - recommended for integrations
const client = new Client('https://cloud.parallel.works')
  .withApiKey(process.env.PW_API_KEY!)

// Or using Bearer Token (JWT) - for scripts, expires in 24h
const client = new Client('https://cloud.parallel.works')
  .withToken(process.env.PW_TOKEN!)

// Make requests with full type safety
const { data, error } = await client.GET('/api/organizations')

if (error) {
  console.error('Error:', error)
} else {
  console.log('Organizations:', data)
}

// POST example
const { data: newOrg } = await client.POST('/api/organizations', {
  body: { name: 'My Organization' }
})
```

### SWR Hooks (React)

```tsx
// lib/api.ts
import { Client } from '@parallelworks/client'
import { createSwrHooks } from '@parallelworks/client/swr'

const client = new Client('https://cloud.parallel.works')
  .withApiKey(process.env.NEXT_PUBLIC_PW_API_KEY!)

export const { useQuery, useImmutable, useInfinite } = createSwrHooks(client)
```

```tsx
// components/OrganizationList.tsx
import { useQuery } from '@/lib/api'

export function OrganizationList() {
  const { data, error, isLoading } = useQuery('/api/organizations')

  if (isLoading) return <div>Loading...</div>
  if (error) return <div>Error: {error.message}</div>

  return (
    <ul>
      {data?.map(org => (
        <li key={org.id}>{org.name}</li>
      ))}
    </ul>
  )
}
```

## Authentication

The client supports two authentication methods:

### API Key (Basic Auth)

Best for long-running integrations with configurable expiration dates.

```typescript
const client = new Client('https://cloud.parallel.works')
  .withApiKey('your-api-key')
```

### Bearer Token (JWT)

Best for scripts and CLI tools. Tokens expire after 24 hours.

```typescript
const client = new Client('https://cloud.parallel.works')
  .withToken('your-jwt-token')
```

Both API keys and tokens can be generated from your ACTIVATE account settings.

## Documentation

For full API documentation, visit [https://parallelworks.com/docs](https://parallelworks.com/docs).

## License

MIT
