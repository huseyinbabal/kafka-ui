import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, enc, get, type List } from '@/lib/api'
import { prettyJson } from '@/lib/json'

export type SchemaType = 'AVRO' | 'PROTOBUF' | 'JSON'
export type SchemaReference = { name: string; subject: string; version: number }
export type SubjectSummary = {
  subject: string
  latestVersion: number
  schemaId: number
  schemaType: SchemaType
  versions: number
}
export type SchemaVersion = {
  subject: string
  version: number
  id: number
  schemaType: SchemaType
  schema: string
  references: SchemaReference[]
}
export type SubjectDetail = {
  subject: string
  versions: SchemaVersion[]
  compatibility: string
  globalCompatibility: string
}
export type RegisterInput = { schema: string; schemaType: SchemaType; references?: SchemaReference[] }
export type CompatibilityCheck = { compatible: boolean; messages: string[] }

export const compatibilityLevels = [
  'BACKWARD',
  'BACKWARD_TRANSITIVE',
  'FORWARD',
  'FORWARD_TRANSITIVE',
  'FULL',
  'FULL_TRANSITIVE',
  'NONE',
] as const

const base = (c: string) => `/clusters/${enc(c)}/schemas/subjects`
const key = (c: string) => ['clusters', c, 'schemas']

export function useSubjects(clusterId: string, enabled = true) {
  return useQuery({
    queryKey: key(clusterId),
    queryFn: () => get<List<SubjectSummary>>(base(clusterId)),
    select: (d) => d.items,
    enabled,
  })
}

export function useSubject(clusterId: string, subject: string) {
  return useQuery({
    queryKey: [...key(clusterId), subject],
    queryFn: () => get<SubjectDetail>(`${base(clusterId)}/${enc(subject)}`),
  })
}

export function useCheckCompatibility(clusterId: string, subject: string) {
  return useMutation({
    mutationFn: (input: RegisterInput) =>
      api<CompatibilityCheck>('POST', `${base(clusterId)}/${enc(subject)}/compatibility-check`, input),
  })
}

function useInvalidate(clusterId: string) {
  const qc = useQueryClient()
  return () => qc.invalidateQueries({ queryKey: key(clusterId) })
}

export function useRegisterSchema(clusterId: string, subject: string) {
  const invalidate = useInvalidate(clusterId)
  return useMutation({
    mutationFn: (input: RegisterInput) =>
      api<SchemaVersion>('POST', `${base(clusterId)}/${enc(subject)}/versions`, input),
    onSuccess: () => void invalidate(),
  })
}

export function useSetCompatibility(clusterId: string, subject: string) {
  const invalidate = useInvalidate(clusterId)
  return useMutation({
    mutationFn: (level: string) => api<void>('PUT', `${base(clusterId)}/${enc(subject)}/compatibility`, { level }),
    onSuccess: () => void invalidate(),
  })
}

export function useDeleteSubject(clusterId: string, subject: string) {
  const invalidate = useInvalidate(clusterId)
  return useMutation({
    mutationFn: (permanent: boolean) =>
      api<{ versions: number[] }>('DELETE', `${base(clusterId)}/${enc(subject)}${permanent ? '?permanent=true' : ''}`),
    onSuccess: () => void invalidate(),
  })
}

/** Avro and JSON Schema are JSON documents; pretty-print them for reading and diffing. */
export function formatSchema(s: Pick<SchemaVersion, 'schema' | 'schemaType'>): string {
  return s.schemaType === 'PROTOBUF' ? s.schema : prettyJson(s.schema)
}
