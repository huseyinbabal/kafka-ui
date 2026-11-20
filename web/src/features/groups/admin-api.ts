import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api, enc } from '@/lib/api'

const base = (c: string, g: string) => `/clusters/${enc(c)}/groups/${enc(g)}`

export type ResetStrategy = 'earliest' | 'latest' | 'offset' | 'timestamp' | 'shift'
export type ResetInput = {
  topic: string
  partitions: number[]
  strategy: ResetStrategy
  value: number
  dryRun: boolean
}
export type ResetPlanItem = { topic: string; partition: number; current: number; target: number; error?: string }
export type ResetResult = { applied: boolean; items: ResetPlanItem[] }

export function useResetOffsets(clusterId: string, group: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: ResetInput) => api<ResetResult>('POST', `${base(clusterId, group)}/reset-offsets`, input),
    onSuccess: (r) => {
      if (r.applied) void qc.invalidateQueries({ queryKey: ['clusters', clusterId, 'groups'] })
    },
  })
}

export function useDeleteGroup(clusterId: string, group: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api<void>('DELETE', base(clusterId, group)),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['clusters', clusterId, 'groups'] }),
  })
}

/** Offsets can only change while no consumer is active. */
export const canReset = (state: string) => state === 'Empty' || state === 'Dead'
