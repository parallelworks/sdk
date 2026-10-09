import { type ApiError, problemMiddleware } from '@parallelworks/problem'
import createClient, {
  type ClientOptions as OpenAPIClientOptions,
  type Middleware,
} from 'openapi-fetch'
import type { paths } from './types/api'

export type { paths }
export type { components, operations } from './types/api'
export {
  ApiError,
  codeForStatus,
  FieldError,
  fromResponseBody,
  problemMediaType,
  toApiError,
} from '@parallelworks/problem'

type OpenAPIFetchClient = ReturnType<typeof createClient<paths>>

export interface ClientOptions extends Omit<OpenAPIClientOptions, 'baseUrl'> {
  /**
   * The Accept-Language for error details. Outside a browser it defaults to
   * the locale environment (see `acceptLanguageFromEnv`).
   */
  acceptLanguage?: string
}

/**
 * The language of the POSIX locale environment (LC_ALL, then LC_MESSAGES,
 * then LANG) as a language tag: `ja_JP.UTF-8` is `ja-JP`. Undefined when none
 * is set or the locale is C or POSIX.
 */
export function acceptLanguageFromEnv(
  env: Record<string, string | undefined> = (
    globalThis as { process?: { env?: Record<string, string | undefined> } }
  ).process?.env ?? {}
): string | undefined {
  const locale = env['LC_ALL'] || env['LC_MESSAGES'] || env['LANG']
  const tag = locale?.split('.')[0]?.split('@')[0]
  if (!tag || tag === 'C' || tag === 'POSIX' || !/^[\w-]+$/.test(tag)) {
    return undefined
  }
  return tag.replace(/_/g, '-')
}

/**
 * The absolute URL documenting an error's problem type, resolved against the
 * API's base URL. Undefined for `about:blank`, which documents nothing.
 */
export function problemTypeUrl(
  error: ApiError,
  baseUrl: string
): string | undefined {
  if (error.type === 'about:blank') {
    return undefined
  }
  try {
    return new URL(error.type, baseUrl).toString()
  } catch {
    return undefined
  }
}

/** Prefix for Parallel Works API keys */
export const API_KEY_PREFIX = 'pwt_'

/**
 * Prefix for opaque user tokens. Like an API key, a user token names its
 * platform host; like a JWT, it is sent as a Bearer token.
 */
export const USER_TOKEN_PREFIX = 'pwut_'

/** Error thrown when credential parsing fails */
export class CredentialError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'CredentialError'
  }
}

/**
 * Check if a credential is an API key.
 *
 * API keys start with the prefix "pwt_".
 *
 * @param credential - The credential string to check
 * @returns True if the credential appears to be an API key
 */
export function isApiKey(credential: string): boolean {
  return credential.trim().startsWith(API_KEY_PREFIX)
}

/**
 * Check if a credential is a token, sent as a Bearer token.
 *
 * Tokens are opaque user tokens (starting with "pwut_") or JWTs, which have
 * three base64-encoded parts separated by dots.
 *
 * @param credential - The credential string to check
 * @returns True if the credential appears to be a token
 */
export function isToken(credential: string): boolean {
  const trimmed = credential.trim()
  if (trimmed.startsWith(USER_TOKEN_PREFIX)) {
    return true
  }
  const parts = trimmed.split('.')
  return parts.length === 3 && !trimmed.startsWith(API_KEY_PREFIX)
}

/**
 * Extract the platform host from an API key or token.
 *
 * For API keys (pwt_xxxx.yyyy) and opaque user tokens (pwut_xxxx.yyyy): decodes
 * the first part after the prefix to get the host
 * For JWT tokens: decodes the payload (second segment) and reads platform_host field
 *
 * @param credential - The API key or token
 * @returns The platform host (e.g., "activate.parallel.works")
 * @throws CredentialError if the credential format is invalid
 */
export function extractPlatformHost(credential: string): string {
  credential = credential.trim()
  if (isApiKey(credential)) {
    return extractEncodedHost(credential, API_KEY_PREFIX)
  }
  if (credential.startsWith(USER_TOKEN_PREFIX)) {
    return extractEncodedHost(credential, USER_TOKEN_PREFIX)
  }
  if (isToken(credential)) {
    return extractHostFromToken(credential)
  }
  throw new CredentialError('Invalid credential format')
}

function extractEncodedHost(credential: string, prefix: string): string {
  const withoutPrefix = credential.slice(prefix.length)

  // Split by dot
  const dotIndex = withoutPrefix.indexOf('.')
  if (dotIndex === -1) {
    throw new CredentialError('Invalid credential format')
  }

  const encodedHost = withoutPrefix.slice(0, dotIndex)

  // Decode base64 (handle both browser and Node.js)
  let host: string
  try {
    if (typeof atob !== 'undefined') {
      // Browser - handle URL-safe base64
      const normalized = encodedHost.replace(/-/g, '+').replace(/_/g, '/')
      host = atob(normalized)
    } else {
      // Node.js
      host = Buffer.from(encodedHost, 'base64url').toString()
    }
  } catch {
    try {
      // Fallback to standard base64
      if (typeof atob !== 'undefined') {
        host = atob(encodedHost)
      } else {
        host = Buffer.from(encodedHost, 'base64').toString()
      }
    } catch (e) {
      throw new CredentialError(`Could not decode credential host: ${e}`)
    }
  }

  if (!host) {
    throw new CredentialError('No platform host in credential')
  }

  return host
}

function extractHostFromToken(token: string): string {
  const parts = token.split('.')
  if (parts.length !== 3) {
    throw new CredentialError('Invalid JWT format')
  }

  const payload = parts[1]

  // Decode base64url payload
  let payloadJson: string
  try {
    if (typeof atob !== 'undefined') {
      // Browser - handle URL-safe base64
      const normalized = payload!.replace(/-/g, '+').replace(/_/g, '/')
      // Add padding if needed
      const padded = normalized + '='.repeat((4 - (normalized.length % 4)) % 4)
      payloadJson = atob(padded)
    } else {
      // Node.js
      payloadJson = Buffer.from(payload!, 'base64url').toString()
    }
  } catch (e) {
    throw new CredentialError(`Could not decode JWT payload: ${e}`)
  }

  let claims: { platform_host?: string }
  try {
    claims = JSON.parse(payloadJson)
  } catch (e) {
    throw new CredentialError(`Could not parse JWT claims: ${e}`)
  }

  if (!claims.platform_host) {
    throw new CredentialError('No platform_host in JWT claims')
  }

  return claims.platform_host
}

/**
 * Parallel Works API Client
 *
 * @example
 * ```ts
 * import { Client } from '@parallelworks/client'
 *
 * // Using API Key (Basic Auth) - recommended for integrations
 * const client = new Client('https://activate.parallel.works')
 *   .withApiKey('pwt_...')
 *
 * // Using Bearer Token (JWT) - for scripts
 * const client = new Client('https://activate.parallel.works')
 *   .withToken('eyJ...')
 *
 * // Or let the client extract the host from your credential
 * const client = Client.fromCredential(process.env.PW_API_KEY!)
 *
 * // Make requests
 * const { data, error } = await client.GET('/api/buckets')
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
   * Create a client using only a credential.
   *
   * The platform host is automatically extracted from the credential:
   * - For API keys and opaque user tokens: host is decoded from the first part after the prefix
   * - For JWT tokens: host is read from the platform_host claim
   *
   * @param credential - Your API key or JWT token
   * @param options - Additional client options
   * @returns Configured API client ready to make requests
   * @throws CredentialError if the credential format is invalid
   *
   * @example
   * ```ts
   * // Just pass your credential - no URL needed!
   * const client = Client.fromCredential(process.env.PW_API_KEY!)
   * ```
   */
  static fromCredential(
    credential: string,
    options: ClientOptions = {}
  ): OpenAPIFetchClient {
    let host = extractPlatformHost(credential)

    // Ensure https:// prefix
    if (!host.startsWith('http://') && !host.startsWith('https://')) {
      host = `https://${host}`
    }

    return new Client(host, options).withCredential(credential)
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
    // Trim whitespace to handle env vars with trailing newlines
    apiKey = apiKey.trim()
    // API Keys use Basic Auth with base64(apiKey:)
    const encoded =
      typeof btoa !== 'undefined'
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
    // Trim whitespace to handle env vars with trailing newlines
    this.authHeader = `Bearer ${token.trim()}`
    return this.build()
  }

  /**
   * Authenticate with automatic credential type detection
   *
   * Automatically detects whether the credential is an API key (starts with "pwt_")
   * or a token and configures the appropriate authentication method.
   *
   * @param credential - Your API key or JWT token
   * @returns Configured API client ready to make requests
   */
  withCredential(credential: string): OpenAPIFetchClient {
    if (isApiKey(credential)) {
      return this.withApiKey(credential)
    }
    return this.withToken(credential)
  }

  /**
   * Build the openapi-fetch client with configured options.
   *
   * @returns Configured openapi-fetch client instance
   */
  build(): OpenAPIFetchClient {
    const { acceptLanguage: language, ...options } = this.options
    const acceptLanguage =
      language ??
      (typeof document === 'undefined' ? acceptLanguageFromEnv() : undefined)
    const client = createClient<paths>({
      baseUrl: this.baseUrl,
      ...options,
      headers: {
        ...(acceptLanguage && { 'Accept-Language': acceptLanguage }),
        ...options.headers,
        ...(this.authHeader && { Authorization: this.authHeader }),
      },
    })
    client.use(problemMiddleware)

    // Attach HTTP status code to error response bodies so consumers
    // (e.g. SWR hooks) can distinguish 404s from other errors.
    // openapi-fetch parses the body after middleware, so we replace the
    // response with one whose body includes the status field.
    // Also guarantee `message` is populated, from a problem's detail when it
    // has one: upstream infrastructure (WAFs, proxies) can return non-2xx with
    // an empty body, which would otherwise short-circuit openapi-fetch (it
    // returns `error: undefined` when Content-Length is 0) and leave consumers
    // with no error to react to.
    const statusMiddleware: Middleware = {
      async onResponse({ response }) {
        if (!response.ok) {
          const body = await response.json().catch(() => ({}))
          const record =
            body && typeof body === 'object'
              ? (body as Record<string, unknown>)
              : {}
          record['status'] = response.status
          if (typeof record['message'] !== 'string' || !record['message']) {
            record['message'] =
              (typeof record['detail'] === 'string' && record['detail']) ||
              response.statusText ||
              `Request failed with status ${response.status}`
          }
          // Strip Content-Length from the original headers — we're replacing
          // the body, and a stale `Content-Length: 0` from an empty upstream
          // response causes openapi-fetch to skip body parsing entirely.
          const headers = new Headers(response.headers)
          headers.delete('content-length')
          return new Response(JSON.stringify(record), {
            status: response.status,
            statusText: response.statusText,
            headers,
          })
        }
        return response
      },
    }
    client.use(statusMiddleware)

    return client
  }
}

export default Client
