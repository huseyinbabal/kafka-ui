import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api, enc, type TopicSummary } from '@/lib/api'

const base = (c: string) => `/clusters/${enc(c)}/topics`

export type CreateTopicInput = {
  name: string
  partitions: number
  replicationFactor: number
  configs: Record<string, string>
  validateOnly?: boolean
}

function useInvalidateTopics(clusterId: string) {
  const qc = useQueryClient()
  return () => qc.invalidateQueries({ queryKey: ['clusters', clusterId, 'topics'] })
}

export function useCreateTopic(clusterId: string) {
  const invalidate = useInvalidateTopics(clusterId)
  return useMutation({
    mutationFn: (input: CreateTopicInput) => api<TopicSummary>('POST', base(clusterId), input),
    onSuccess: (_, input) => {
      if (!input.validateOnly) void invalidate()
    },
  })
}

export function useDeleteTopic(clusterId: string, topic: string) {
  const invalidate = useInvalidateTopics(clusterId)
  return useMutation({
    mutationFn: () => api<void>('DELETE', `${base(clusterId)}/${enc(topic)}`),
    onSuccess: () => void invalidate(),
  })
}

export type ConfigChange = { name: string; value: string | null }

export function useAlterTopicConfigs(clusterId: string, topic: string) {
  const invalidate = useInvalidateTopics(clusterId)
  return useMutation({
    mutationFn: (changes: ConfigChange[]) =>
      api<void>('PATCH', `${base(clusterId)}/${enc(topic)}/configs`, { changes }),
    onSuccess: () => void invalidate(),
  })
}

export function useSetPartitions(clusterId: string, topic: string) {
  const invalidate = useInvalidateTopics(clusterId)
  return useMutation({
    mutationFn: (count: number) => api<void>('PUT', `${base(clusterId)}/${enc(topic)}/partitions`, { count }),
    onSuccess: () => void invalidate(),
  })
}

export type PurgeResult = { partition: number; lowWatermark: number; error?: string }

export function usePurgeTopic(clusterId: string, topic: string) {
  const invalidate = useInvalidateTopics(clusterId)
  return useMutation({
    mutationFn: () => api<{ items: PurgeResult[] }>('POST', `${base(clusterId)}/${enc(topic)}/purge`, {}),
    onSuccess: () => void invalidate(),
  })
}
