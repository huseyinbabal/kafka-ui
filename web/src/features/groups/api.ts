import { useQuery } from '@tanstack/react-query'
import { enc, type GroupDetail, type GroupSummary, get, type List } from '@/lib/api'

const base = (c: string) => `/clusters/${enc(c)}/groups`

export function useGroups(clusterId: string) {
  return useQuery({
    queryKey: ['clusters', clusterId, 'groups'],
    queryFn: () => get<List<GroupSummary>>(base(clusterId)),
    refetchInterval: 5_000,
    select: (d) => d.items,
  })
}

export function useGroup(clusterId: string, group: string) {
  return useQuery({
    queryKey: ['clusters', clusterId, 'groups', group],
    queryFn: () => get<GroupDetail>(`${base(clusterId)}/${enc(group)}`),
    refetchInterval: 5_000,
  })
}
