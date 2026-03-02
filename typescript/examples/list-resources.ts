/// <reference types="node" />
/**
 * Example: List buckets and clusters using the Parallel Works TypeScript client
 *
 * Usage:
 *   export PW_API_KEY="your-api-key-or-token"
 *   pnpm tsx list-resources.ts
 */

import { Client } from '@parallelworks/client'

async function main() {
  const apiKey = process.env.PW_API_KEY
  if (!apiKey) {
    console.error('Error: PW_API_KEY environment variable is required')
    process.exit(1)
  }

  // Create an authenticated client - host is auto-detected from credential
  const client = Client.fromCredential(apiKey)

  console.log('Fetching resources...\n')

  // Fetch buckets and clusters in parallel
  const [bucketsResponse, clustersResponse] = await Promise.all([
    client.GET('/api/buckets'),
    client.GET('/api/clusters'),
  ])

  // Display buckets
  if (bucketsResponse.error) {
    console.error('Failed to get buckets:', bucketsResponse.error)
  } else {
    const buckets = bucketsResponse.data ?? []
    console.log(`Buckets (${buckets.length}):`)
    if (buckets.length === 0) {
      console.log('  No buckets found')
    } else {
      for (const bucket of buckets) {
        console.log(`  - ${bucket.name} (${bucket.csp})`)
      }
    }
  }

  console.log()

  // Display clusters
  if (clustersResponse.error) {
    console.error('Failed to get clusters:', clustersResponse.error)
  } else {
    const clusters = clustersResponse.data ?? []
    console.log(`Clusters (${clusters.length}):`)
    if (clusters.length === 0) {
      console.log('  No clusters found')
    } else {
      for (const cluster of clusters) {
        console.log(`  - ${cluster.name} (${cluster.status})`)
      }
    }
  }
}

main()
