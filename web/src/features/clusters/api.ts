import { useQuery } from '@tanstack/react-query'
import { type ClusterInfo, enc, get, type List, type Overview } from '@/lib/api'

export function useClusters() {
  return useQuery({
    queryKey: ['clusters'],
    queryFn: () => get<List<ClusterInfo>>('/clusters'),
    refetchInterval: 15_000,
    select: (d) => d.items,
  })
}

export function useCluster(clusterId: string) {
  const q = useClusters()
  return { ...q, data: q.data?.find((c) => c.id === clusterId) }
}

export function useOverview(clusterId: string) {
  return useQuery({
    queryKey: ['clusters', clusterId, 'overview'],
    queryFn: () => get<Overview>(`/clusters/${enc(clusterId)}/overview`),
    refetchInterval: 10_000,
  })
}
