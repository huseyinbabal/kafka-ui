import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCluster } from '@/features/clusters/api'
import { ApiError, api, get, type Principal } from '@/lib/api'

export const meKey = ['auth', 'me'] as const

export function useMe() {
  return useQuery({
    queryKey: meKey,
    queryFn: async () => {
      try {
        return await get<Principal>('/auth/me')
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) return null
        throw e
      }
    },
    staleTime: 60_000,
  })
}

export function useLogin() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { username: string; password: string }) => api<Principal>('POST', '/auth/login', body),
    onSuccess: (p) => qc.setQueryData(meKey, p),
  })
}

export function useLogout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api<void>('POST', '/auth/logout'),
    onSuccess: () => {
      qc.clear()
      qc.setQueryData(meKey, null)
    },
  })
}

export function useCanWrite() {
  const { data } = useMe()
  return data?.role === 'admin'
}

/** True when the user may change things on this cluster (admin role and not read-only). */
export function useCanMutate(clusterId: string) {
  const canWrite = useCanWrite()
  const { data } = useCluster(clusterId)
  return canWrite && data?.readOnly === false
}
