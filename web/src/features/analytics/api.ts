import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { enc, get } from '@/lib/api'

export type Range = '15m' | '1h' | '6h' | '24h' | '7d'
export const ranges: Range[] = ['15m', '1h', '6h', '24h', '7d']

export type ThroughputPoint = { ts: number; messages: number }
export type BytesPoint = { ts: number; bytes: number }
export type LagPoint = { ts: number; lag: number }
export type TopicStat = { topic: string; msgRate: number; messages: number; sizeBytes: number; growthBytes: number }
export type GroupStat = {
  group: string
  lag: number
  consumeRate: number
  ingressRate: number
  catchUpSeconds: number | null
}
export type Overview = {
  step: number
  from: number
  to: number
  collectedSince: number
  throughput: ThroughputPoint[]
  bytes: BytesPoint[]
  bytesStep: number
  lag: LagPoint[]
  topTopics: TopicStat[]
  groups: GroupStat[]
}
export type TopicPoint = { ts: number; msgRate: number; bytesRate: number; sizeBytes: number; messages: number }
export type GroupPoint = { ts: number; lag: number; consumeRate: number; byTopic: Record<string, number> }
export type PartitionRate = { topic: string; partition: number; leader: number; rate: number }
export type BrokerLoad = { broker: number; leaders: number; replicas: number; ingress: number }
export type Skew = { at: string; partitions: PartitionRate[]; brokers: BrokerLoad[] }
export type HygieneIssue = { kind: string; severity: 'warning' | 'info'; target: string; message: string }
export type Hygiene = { issues: HygieneIssue[]; idleTopics: string[]; idleSince: number }

const base = (c: string) => `/clusters/${enc(c)}/analytics`
/** Refresh roughly once per bucket, but not faster than the collector. */
const refresh = (r: Range) => (r === '15m' || r === '1h' ? 10_000 : 60_000)

export function useOverview(clusterId: string, range: Range) {
  return useQuery({
    queryKey: ['clusters', clusterId, 'analytics', 'overview', range],
    queryFn: () => get<Overview>(`${base(clusterId)}/overview?range=${range}`),
    refetchInterval: refresh(range),
    placeholderData: keepPreviousData,
  })
}

export function useTopicSeries(clusterId: string, topic: string, range: Range) {
  return useQuery({
    queryKey: ['clusters', clusterId, 'analytics', 'topic', topic, range],
    queryFn: () =>
      get<{ step: number; points: TopicPoint[] }>(`${base(clusterId)}/topics/${enc(topic)}?range=${range}`),
    refetchInterval: refresh(range),
    placeholderData: keepPreviousData,
  })
}

export function useGroupSeries(clusterId: string, group: string, range: Range) {
  return useQuery({
    queryKey: ['clusters', clusterId, 'analytics', 'group', group, range],
    queryFn: () =>
      get<{ step: number; points: GroupPoint[] }>(`${base(clusterId)}/groups/${enc(group)}?range=${range}`),
    refetchInterval: refresh(range),
    placeholderData: keepPreviousData,
  })
}

export function useSkew(clusterId: string) {
  return useQuery({
    queryKey: ['clusters', clusterId, 'analytics', 'skew'],
    queryFn: () => get<Skew>(`${base(clusterId)}/skew`),
    refetchInterval: 10_000,
  })
}

export function useHygiene(clusterId: string) {
  return useQuery({
    queryKey: ['clusters', clusterId, 'analytics', 'hygiene'],
    queryFn: () => get<Hygiene>(`${base(clusterId)}/hygiene?range=24h`),
    refetchInterval: 60_000,
  })
}

/** "2m 30s", "1h 5m", "3d 4h". */
export function formatDuration(seconds: number): string {
  const s = Math.round(seconds)
  if (s < 60) return `${s}s`
  const units: [number, string][] = [
    [86400, 'd'],
    [3600, 'h'],
    [60, 'm'],
    [1, 's'],
  ]
  const parts: string[] = []
  let rest = s
  for (const [size, label] of units) {
    if (rest >= size) {
      parts.push(`${Math.floor(rest / size)}${label}`)
      rest %= size
    }
    if (parts.length === 2) break
  }
  return parts.join(' ')
}

export function formatRate(n: number): string {
  if (n === 0) return '0'
  if (n < 10) return n.toFixed(1)
  return Math.round(n).toLocaleString('en-US')
}
