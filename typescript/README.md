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

### Signed In With `pw auth` (Node.js)

For scripts you run yourself, sign in once with `pw auth`, then create the client from the pw credentials file:

```typescript
import { Client } from '@parallelworks/client'

const client = Client.fromCredentialConfig()
const { data } = await client.GET('/api/buckets')
```

The credential is picked as the `pw` CLI picks it: `PW_API_KEY`, then the `context` option, `PW_CONTEXT`, and the current context. A signed-in context stays signed in: the client runs `pw auth token --print -o json` for an access token, reads its lifetime from the response (an RFC 6749 token response), and runs it again a minute before the token expires, the way a Kubernetes exec credential plugin or an AWS `credential_process` works, so only the `pw` CLI ever rotates the sign-in's refresh token. `cliCommand` and `cliTimeoutMs` set the `pw` executable (default `pw` on `PATH`) and how long one run may take (default one minute). A `pw` found through a relative `PATH` entry, such as the current directory, is refused, as Go refuses it. Without `pw`, requests reject with `SignInExpiredError`. This needs Node.js 20.16 or later.

For unattended jobs such as CI, use an API key in `PW_API_KEY` instead.

### Automatic Host Detection

API keys (`pwt_...`) and JWT tokens contain the platform host encoded within them. Use `fromCredential` to automatically extract it:

```typescript
// API key - host decoded from first segment after pwt_
const client = Client.fromCredential('pwt_Y2xvdWQucGFyYWxsZWwud29ya3M.xxxxx')
// Connects to: https://cloud.parallel.works

// JWT token - host read from platform_host claim
const client = Client.fromCredential('eyJhbGci...')
// Connects to the host in the token's platform_host claim

// Access token (pwoa_...), which names no host: PW_PLATFORM_HOST, else the
// server of the credentials file's selected context (Node.js)
const client = Client.fromCredential(token)
```

### Explicit Host

If you prefer to specify the host explicitly:

```typescript
// API Key (Basic Auth) - best for unattended jobs such as CI
const client = new Client('https://cloud.parallel.works')
  .withApiKey('pwt_...')

// Bearer token, sent as is and never renewed
const client = new Client('https://cloud.parallel.works')
  .withToken(token)

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

## Errors

The client asks for errors as [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) problem details. Outside a browser it sends `Accept-Language` from the locale environment (`LC_ALL`, `LC_MESSAGES`, `LANG`), or from the `acceptLanguage` option. `toApiError` reads any error, including an older server's error envelope, as an `ApiError` with a stable `code`, `params`, the invalid `fields`, and the server's detail as `message`:

```ts
import { Client, problemTypeUrl, toApiError } from '@parallelworks/client'

const { error } = await client.GET('/api/buckets')
if (error) {
  const problem = toApiError(error)
  console.error(problem.code, problem.message)
  for (const field of problem.fields) console.error(`  ${field.path}: ${field.detail}`)
  console.error(problemTypeUrl(problem, 'https://activate.parallel.works'))
}
```

## Documentation

For full API documentation, visit [https://parallelworks.com/docs](https://parallelworks.com/docs).

## License

Apache-2.0
