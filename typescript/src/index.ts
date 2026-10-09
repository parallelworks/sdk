import { type ApiError, problemMiddleware } from '@parallelworks/problem'
import createClient, {
  type ClientOptions as OpenAPIClientOptions,
  type Middleware,
} from 'openapi-fetch'
import {
  CLIAuth,
  hostForUnnamedCredential,
  loadCredentialConfig,
  type ResolveOptions,
  resolveIdentity,
  type TokenProvider,
} from './credentials'
import type { paths } from './types/api'

export * from './credentials'

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

export interface CredentialConfigOptions extends ClientOptions, ResolveOptions {
  /** The pw executable that renews a `pw auth` sign-in, looked up on PATH. */
  cliCommand?: string | undefined
  /** Bounds each run of the pw CLI that renews a `pw auth` sign-in. */
  cliTimeoutMs?: number | undefined
}

function withScheme(host: string): string {
  return host.startsWith('http://') || host.startsWith('https://')
    ? host
    : `https://${host}`
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
 * Prefix for opaque access tokens, such as the one `pw auth` saves. Sent as a
 * Bearer token; it names no platform host.
 */
export const OPAQUE_ACCESS_TOKEN_PREFIX = 'pwoa_'

/** Error thrown when credential parsing fails */
export class CredentialError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'CredentialError'
  }
}

/** Raised when a credential names no platform host and nothing else supplies one. */
export class NoPlatformHostError extends CredentialError {
  constructor(message = 'could not extract platform host from credential') {
    super(message)
    this.name = 'NoPlatformHostError'
  }
}

/** Raised when the pw CLI cannot provide a token for a `pw auth` sign-in and no earlier one is still valid. */
export class SignInExpiredError extends CredentialError {
  constructor(cause: unknown) {
    super(
      `the pw CLI could not provide a token for the pw auth sign-in; run pw auth again, or install pw: ${cause instanceof Error ? cause.message : String(cause)}`
    )
    this.name = 'SignInExpiredError'
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
 * Check if a credential is sent as a Bearer token: a JWT, whose three
 * base64-encoded parts are separated by dots, or an opaque access token.
 *
 * @param credential - The credential string to check
 * @returns True if the credential appears to be a token
 */
export function isToken(credential: string): boolean {
  const trimmed = credential.trim()
  if (trimmed.startsWith(OPAQUE_ACCESS_TOKEN_PREFIX)) {
    return true
  }
  const parts = trimmed.split('.')
  return parts.length === 3 && !trimmed.startsWith(API_KEY_PREFIX)
}

/**
 * Extract the platform host from an API key or JWT token.
 *
 * For API keys (pwt_xxxx.yyyy): decodes the first part after pwt_ to get the host
 * For JWT tokens: decodes the payload (second segment) and reads platform_host field
 *
 * @param credential - The API key or JWT token
 * @returns The platform host (e.g., "activate.parallel.works")
 * @throws NoPlatformHostError for an opaque access token, which names no host
 * @throws CredentialError if the credential format is invalid
 */
export function extractPlatformHost(credential: string): string {
  credential = credential.trim()
  if (isApiKey(credential)) {
    return extractHostFromApiKey(credential)
  }
  if (credential.startsWith(OPAQUE_ACCESS_TOKEN_PREFIX)) {
    throw new NoPlatformHostError()
  }
  if (isToken(credential)) {
    return extractHostFromToken(credential)
  }
  throw new CredentialError('Invalid credential format')
}

function extractHostFromApiKey(apiKey: string): string {
  // Remove pwt_ prefix
  const withoutPrefix = apiKey.slice(API_KEY_PREFIX.length)

  // Split by dot
  const dotIndex = withoutPrefix.indexOf('.')
  if (dotIndex === -1) {
    throw new CredentialError('Invalid API key format')
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
      throw new CredentialError(`Could not decode API key host: ${e}`)
    }
  }

  if (!host) {
    throw new CredentialError('No platform host in API key')
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
 * // Signed in with `pw auth`: the client asks the pw CLI to renew the token
 * const client = Client.fromCredentialConfig()
 *
 * // Using API Key (Basic Auth) - for unattended jobs such as CI
 * const client = new Client('https://activate.parallel.works')
 *   .withApiKey('pwt_...')
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
  private authHeader: string | undefined
  private tokenProvider: TokenProvider | undefined

  constructor(baseUrl: string, options: ClientOptions = {}) {
    this.baseUrl = baseUrl
    this.options = options
  }

  /**
   * Create a client using only a credential.
   *
   * The platform host is automatically extracted from the credential:
   * - For API keys: host is decoded from the first part after pwt_
   * - For JWT tokens: host is read from the platform_host claim
   * - For access tokens (pwoa_), which name no host: PW_PLATFORM_HOST, else
   *   the server of the credentials file's selected context (Node.js only)
   *
   * @param credential - Your API key or token
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
    let host: string
    try {
      host = extractPlatformHost(credential)
    } catch (e) {
      const fallback =
        e instanceof NoPlatformHostError
          ? hostForUnnamedCredential()
          : undefined
      if (!fallback) {
        throw e
      }
      host = fallback
    }
    return new Client(withScheme(host), options).withCredential(credential)
  }

  /**
   * Create a client from the pw credentials file (Node.js only), picking the
   * credential as the CLI and the Go SDK do: PW_API_KEY, then the `context`
   * option, PW_CONTEXT, and the file's current context.
   *
   * A context signed in with `pw auth` stays signed in: when its access token
   * nears expiry the client runs `pw auth token --print` for a renewed one
   * (see `CLIAuth`), so sign in once and let scripts run. Unattended jobs such
   * as CI should use an API key in PW_API_KEY instead.
   *
   * @example
   * ```ts
   * const client = Client.fromCredentialConfig()
   * const { data } = await client.GET('/api/buckets')
   * ```
   */
  static fromCredentialConfig(
    options: CredentialConfigOptions = {}
  ): OpenAPIFetchClient {
    const {
      context,
      platformHost,
      cliCommand,
      cliTimeoutMs,
      ...clientOptions
    } = options
    const config = loadCredentialConfig()
    const { context: name, identity } = resolveIdentity(config, {
      context,
      platformHost,
    })
    const client = new Client(withScheme(identity.server), clientOptions)
    if (identity.apikey) {
      return isToken(identity.apikey)
        ? client.withToken(identity.apikey)
        : client.withApiKey(identity.apikey)
    }
    if (!identity.token) {
      throw new CredentialError('you must first authenticate using "pw auth"')
    }
    if (identity.oauth) {
      return client.withTokenProvider(
        new CLIAuth({
          command: cliCommand,
          context: name,
          timeoutMs: cliTimeoutMs,
        })
      )
    }
    return client.withToken(identity.token)
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
    this.tokenProvider = undefined
    return this.build()
  }

  /**
   * Authenticate with a Bearer token, sent as is and never renewed.
   *
   * For a script, sign in once with `pw auth` and use `fromCredentialConfig`,
   * which renews the token; for an unattended job such as CI, use an API key.
   *
   * @param token - A token, such as one `pw auth token --print` printed
   * @returns Configured API client ready to make requests
   */
  withToken(token: string): OpenAPIFetchClient {
    // Trim whitespace to handle env vars with trailing newlines
    this.authHeader = `Bearer ${token.trim()}`
    this.tokenProvider = undefined
    return this.build()
  }

  /**
   * Authenticate with automatic credential type detection
   *
   * Automatically detects whether the credential is an API key (starts with "pwt_")
   * or a token and configures the appropriate authentication method.
   *
   * @param credential - Your API key or token
   * @returns Configured API client ready to make requests
   */
  withCredential(credential: string): OpenAPIFetchClient {
    if (isApiKey(credential)) {
      return this.withApiKey(credential)
    }
    return this.withToken(credential)
  }

  /**
   * Authenticate each request with a Bearer token the provider supplies, such
   * as a `CLIAuth` that renews a `pw auth` sign-in.
   *
   * @param provider - Supplies the current token
   * @returns Configured API client ready to make requests
   */
  withTokenProvider(provider: TokenProvider): OpenAPIFetchClient {
    this.authHeader = undefined
    this.tokenProvider = provider
    return this.build()
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
    const provider = this.tokenProvider
    if (provider) {
      client.use({
        async onRequest({ request }) {
          request.headers.set(
            'Authorization',
            `Bearer ${await provider.token()}`
          )
          return request
        },
      })
    }
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
