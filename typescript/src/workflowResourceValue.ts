import type { components } from '@parallelworks/client'

export function computeResourceValue(
  cluster: components['schemas']['GeneralCluster']
) {
  const uri =
    cluster.type === 'managed-cluster'
      ? `pw://${cluster.name}`
      : `pw://${cluster.user}/${cluster.name}`
  return {
    id: cluster.id,
    type: cluster.type,
    provider: cluster.type,
    ip: cluster.ipAddress,
    user: cluster.user,
    namespace: cluster.user,
    name: cluster.name,
    tags: cluster.tags,
    schedulerType: cluster.schedulerType,
    uri,
  }
}
