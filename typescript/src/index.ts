import createClient, { type ClientOptions as OpenAPIClientOptions } from 'openapi-fetch'
import type { paths } from './types/api'

export type { paths }
export type { components } from './types/api'

type OpenAPIFetchClient = ReturnType<typeof createClient<paths>>

export interface ClientOptions extends Omit<OpenAPIClientOptions, 'baseUrl'> {}

/**
 * Parallel Works API Client
 *
 * @example
 * ```ts
 * import { Client } from '@parallelworks/client'
 *
 * // Using API Key (Basic Auth) - recommended for integrations
 * const client = new Client('https://cloud.parallel.works')
 *   .withApiKey('your-api-key')
 *
 * // Using Bearer Token (JWT) - for scripts
 * const client = new Client('https://cloud.parallel.works')
 *   .withToken('your-jwt-token')
 *
 * // Make requests
 * const { data, error } = await client.GET('/api/organizations')
 * ```
 */
export class Client {
  private baseUrl: string
  private options: ClientOptions
  private authHeader?: string

  constructor(baseUrl: string, options: ClientOptions = {}) {
    this.baseUrl = baseUrl
    this.options = options
  }

  /**
   * Authenticate with an API Key using Basic Auth
   *
   * Best for long-running integrations with configurable expiration.
   * API keys can be generated from your ACTIVATE account settings.
   *
   * @param apiKey - Your API key from account settings
   * @returns Configured API client ready to make requests
   */
  withApiKey(apiKey: string): OpenAPIFetchClient {
    // API Keys use Basic Auth with base64(apiKey:)
    const encoded = typeof btoa !== 'undefined'
      ? btoa(`${apiKey}:`)
      : Buffer.from(`${apiKey}:`).toString('base64')
    this.authHeader = `Basic ${encoded}`
    return this.build()
  }

  /**
   * Authenticate with a Bearer Token (JWT)
   *
   * Best for scripts and CLI tools. Tokens expire after 24 hours.
   * Tokens can be generated from your ACTIVATE account settings.
   *
   * @param token - Your JWT token from account settings
   * @returns Configured API client ready to make requests
   */
  withToken(token: string): OpenAPIFetchClient {
    this.authHeader = `Bearer ${token}`
    return this.build()
  }

  private build(): OpenAPIFetchClient {
    return createClient<paths>({
      baseUrl: this.baseUrl,
      ...this.options,
      headers: {
        ...(this.authHeader && { Authorization: this.authHeader }),
        ...this.options.headers,
      },
    })
  }
}

export default Client
