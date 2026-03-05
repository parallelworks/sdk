'use client'
import {
  createImmutableHook,
  createInfiniteHook,
  createQueryHook,
} from 'swr-openapi'
import type { Client } from '../index'

export interface SwrHooksOptions {
  /** Key prefix for SWR cache (default: 'api') */
  prefix?: string
}

/**
 * HTTP status code injected by the client middleware into error responses.
 * Use this to check the status code of API errors from SWR hooks.
 *
 * @example
 * ```tsx
 * const { error } = useQuery('/api/resource')
 * if (error && hasStatus(error, 404)) {
 *   return <NotFound />
 * }
 * ```
 */
export function hasStatus(
  error: Record<string, unknown>,
  status: number
): boolean {
  return (error as { status?: number }).status === status
}

type AuthenticatedClient = ReturnType<Client['withApiKey']>

/**
 * Create SWR hooks from an authenticated Parallel Works client
 *
 * @example
 * ```tsx
 * import { Client } from '@parallelworks/client'
 * import { createSwrHooks } from '@parallelworks/client/swr'
 *
 * // Server-side only - never expose API keys to the client
 * const client = new Client('https://activate.parallel.works')
 *   .withApiKey(process.env.PW_API_KEY!)
 *
 * export const { useQuery, useImmutable, useInfinite } = createSwrHooks(client)
 *
 * // In your component
 * function BucketList() {
 *   const { data, error, isLoading } = useQuery('/api/buckets')
 *
 *   if (isLoading) return <div>Loading...</div>
 *   if (error) return <div>Error: {error.message}</div>
 *
 *   return (
 *     <ul>
 *       {data?.map(org => <li key={org.id}>{org.name}</li>)}
 *     </ul>
 *   )
 * }
 * ```
 */
export function createSwrHooks(
  client: AuthenticatedClient,
  options: SwrHooksOptions = {}
) {
  const { prefix = 'api' } = options

  return {
    /** Hook for standard queries with SWR caching and revalidation */
    useQuery: createQueryHook(client, prefix),
    /** Hook for immutable data that never revalidates */
    useImmutable: createImmutableHook(client, prefix),
    /** Hook for paginated/infinite loading */
    useInfinite: createInfiniteHook(client, prefix),
  }
}

export default createSwrHooks
