import { useQuery } from '@tanstack/react-query'
import { enc, type GroupSummary, get, type List, type TopicDetail, type TopicSummary } from '@/lib/api'

const base = (c: string) => `/clusters/${enc(c)}/topics`

export function useTopics(clusterId: string) {
  return useQuery({
    queryKey: ['clusters', clusterId, 'topics'],
    queryFn: () => get<List<TopicSummary>>(base(clusterId)),
    refetchInterval: 10_000,
    select: (d) => d.items,
  })
}

export function useTopic(clusterId: string, topic: string) {
  return useQuery({
    queryKey: ['clusters', clusterId, 'topics', topic],
    queryFn: () => get<TopicDetail>(`${base(clusterId)}/${enc(topic)}`),
    refetchInterval: 10_000,
  })
}

export function useTopicConsumers(clusterId: string, topic: string) {
  return useQuery({
    queryKey: ['clusters', clusterId, 'topics', topic, 'consumers'],
    queryFn: () => get<List<GroupSummary>>(`${base(clusterId)}/${enc(topic)}/consumers`),
    refetchInterval: 10_000,
    select: (d) => d.items,
  })
}
