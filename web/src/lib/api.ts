export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

type Method = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'

export async function api<T>(method: Method, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (method !== 'GET') headers['X-Requested-With'] = 'kafka-ui'
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  const res = await fetch(`/api/v1${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: 'same-origin',
  })
  if (res.status === 204) return undefined as T

  const text = await res.text()
  const data = text ? JSON.parse(text) : undefined
  if (!res.ok) {
    const err = data?.error
    throw new ApiError(res.status, err?.code ?? 'UNKNOWN', err?.message ?? res.statusText)
  }
  return data as T
}

export const get = <T>(path: string) => api<T>('GET', path)

export type List<T> = { items: T[] }

export const enc = encodeURIComponent

// ---- wire types (mirror internal/kafka + internal/api) ----

export type Role = 'admin' | 'viewer'
export type Principal = { username: string; role: Role }

export type ClusterStatus = 'online' | 'offline' | 'unknown'
export type ClusterInfo = {
  id: string
  name: string
  readOnly: boolean
  hasSchemaRegistry: boolean
  status: ClusterStatus
  lastError?: string
  lastCheck: string
}

export type Broker = {
  id: number
  host: string
  port: number
  rack?: string
  isController: boolean
  leaderCount: number
  replicaCount: number
  sizeBytes: number
  diskTotalBytes: number
  diskUsableBytes: number
}

export type Overview = {
  clusterId: string
  controllerId: number
  version: string
  brokers: Broker[]
  topicCount: number
  internalTopicCount: number
  partitionCount: number
  underReplicatedPartitions: number
  offlinePartitions: number
  totalSizeBytes: number
}

export type ConfigEntry = {
  name: string
  value: string | null
  source: string
  isDefault: boolean
  sensitive: boolean
}

export type TopicSummary = {
  name: string
  internal: boolean
  partitions: number
  replicationFactor: number
  underReplicatedPartitions: number
  messageCount: number
  sizeBytes: number
}

export type Partition = {
  id: number
  leader: number
  replicas: number[]
  isr: number[]
  offlineReplicas: number[]
  startOffset: number
  endOffset: number
  sizeBytes: number
}

export type TopicDetail = TopicSummary & {
  id: string
  partitionDetails: Partition[]
  configs: ConfigEntry[]
}

export type GroupSummary = {
  id: string
  state: string
  protocolType: string
  protocol: string
  coordinator: number
  members: number
  topics: string[]
  totalLag: number
  error?: string
}

export type GroupMember = {
  memberId: string
  instanceId?: string
  clientId: string
  clientHost: string
  assigned: { topic: string; partitions: number[] }[]
}

export type PartitionLag = {
  topic: string
  partition: number
  committed: number
  startOffset: number
  endOffset: number
  lag: number
  memberId?: string
  clientId?: string
  clientHost?: string
  error?: string
}

export type GroupDetail = GroupSummary & {
  memberList: GroupMember[]
  partitions: PartitionLag[]
}
