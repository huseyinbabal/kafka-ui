import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, enc, get, type List } from '@/lib/api'

export type ACL = {
  principal: string
  host: string
  resourceType: string
  resourceName: string
  patternType: string
  operation: string
  permission: string
}

const base = (c: string) => `/clusters/${enc(c)}/acls`
const key = (c: string) => ['clusters', c, 'acls']

export function useACLs(clusterId: string) {
  return useQuery({
    queryKey: key(clusterId),
    queryFn: () => get<List<ACL>>(base(clusterId)),
    select: (d) => d.items,
  })
}

export function useCreateACL(clusterId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (a: ACL) => api<void>('POST', base(clusterId), a),
    onSuccess: () => void qc.invalidateQueries({ queryKey: key(clusterId) }),
  })
}

export function useDeleteACL(clusterId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (a: ACL) => api<{ deleted: number }>('POST', `${base(clusterId)}/delete`, a),
    onSuccess: () => void qc.invalidateQueries({ queryKey: key(clusterId) }),
  })
}

export const resourceTypes = ['TOPIC', 'GROUP', 'CLUSTER', 'TRANSACTIONAL_ID'] as const
export const operations = [
  'ALL',
  'READ',
  'WRITE',
  'CREATE',
  'DELETE',
  'ALTER',
  'DESCRIBE',
  'CLUSTER_ACTION',
  'DESCRIBE_CONFIGS',
  'ALTER_CONFIGS',
  'IDEMPOTENT_WRITE',
] as const
