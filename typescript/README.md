# @parallelworks/sdk

Official TypeScript SDK for the Parallel Works ACTIVATE platform API.

## Installation

```bash
npm install @parallelworks/sdk @hey-api/client-fetch
```

## Usage

```typescript
import { client, getOrganizations } from '@parallelworks/sdk';

// Configure the client
client.setConfig({
  baseUrl: 'https://your-instance.parallel.works',
  // Add authentication headers as needed
  headers: {
    Authorization: 'Bearer YOUR_API_KEY'
  }
});

// Make API calls
const { data, error } = await getOrganizations();

if (error) {
  console.error('Error:', error);
} else {
  console.log('Organizations:', data);
}
```

## Authentication

The SDK supports three authentication methods:

1. **API Key (Basic Auth)**: Use your API key as the password with any username
2. **Bearer Token (JWT)**: Use a JWT token in the Authorization header
3. **Session Cookie**: Use session-based authentication for browser clients

## Documentation

For full API documentation, visit [https://docs.parallel.works](https://docs.parallel.works).

## License

MIT
