import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import { api, enc, get } from '@/lib/api'
import { openSSE } from '@/lib/sse'

export type Format = 'auto' | 'string' | 'json' | 'base64' | 'hex'
export type SeekMode = 'newest' | 'oldest' | 'offset' | 'timestamp' | 'live'
export type FilterField = 'any' | 'key' | 'value' | 'header'

export type Payload = { format: Format; text: string; size: number; truncated?: boolean; schemaId?: number }
export type Message = {
  partition: number
  offset: number
  timestamp: number
  key: Payload | null
  value: Payload | null
  headers: { key: string; value: Payload | null }[]
  size: number
}
export type Progress = { scanned: number; matched: number }
export type StopReason = 'end' | 'limit' | 'scanLimit' | 'timeout' | 'cancelled'
export type BrowseResult = Progress & { reason: StopReason }

export type BrowseParams = {
  mode: SeekMode
  limit: number
  partitions: number[]
  offset?: number
  timestamp?: number
  q: string
  qField: FilterField
  regex: boolean
  keyFormat: Format
  valueFormat: Format
}

const base = (c: string, t: string) => `/clusters/${enc(c)}/topics/${enc(t)}/messages`

export function browseQuery(p: BrowseParams): string {
  const qs = new URLSearchParams({
    mode: p.mode,
    limit: String(p.limit),
    keyFormat: p.keyFormat,
    valueFormat: p.valueFormat,
  })
  if (p.partitions.length > 0) qs.set('partitions', p.partitions.join(','))
  if (p.mode === 'offset' && p.offset !== undefined) qs.set('offset', String(p.offset))
  if (p.mode === 'timestamp' && p.timestamp !== undefined) qs.set('timestamp', String(p.timestamp))
  if (p.q) {
    qs.set('q', p.q)
    qs.set('qField', p.qField)
    if (p.regex) qs.set('regex', 'true')
  }
  return qs.toString()
}

/** Live tail keeps at most this many rows in memory. */
const liveCap = 1000

export type StreamState = {
  status: 'idle' | 'running' | 'done' | 'error'
  messages: Message[]
  progress: Progress
  result?: BrowseResult
  error?: Error
}

const idle: StreamState = { status: 'idle', messages: [], progress: { scanned: 0, matched: 0 } }

/** Streams messages over SSE, batching renders to one per animation frame. */
export function useMessageStream(clusterId: string, topic: string) {
  const [state, setState] = useState<StreamState>(idle)
  const abort = useRef<AbortController | null>(null)

  const stop = useCallback(() => abort.current?.abort(), [])
  useEffect(() => stop, [stop])

  const start = useCallback(
    (p: BrowseParams) => {
      abort.current?.abort()
      const ctrl = new AbortController()
      abort.current = ctrl
      const live = p.mode === 'live'
      let pending: Message[] = []
      let progress: Progress = { scanned: 0, matched: 0 }
      let frame = 0
      const flush = () => {
        frame = 0
        const batch = pending
        pending = []
        setState((s) => {
          if (abort.current !== ctrl) return s
          const messages = live ? [...batch.reverse(), ...s.messages].slice(0, liveCap) : [...s.messages, ...batch]
          return { ...s, messages, progress }
        })
      }
      const schedule = () => {
        if (!frame) frame = requestAnimationFrame(flush)
      }
      const finish = (patch: Partial<StreamState>) => {
        if (frame) cancelAnimationFrame(frame)
        flush()
        setState((s) => (abort.current === ctrl ? { ...s, ...patch } : s))
      }

      setState({ ...idle, status: 'running' })
      openSSE(
        `${base(clusterId, topic)}/stream?${browseQuery(p)}`,
        (e) => {
          const data = JSON.parse(e.data)
          if (e.event === 'message') {
            pending.push(data as Message)
            progress = { ...progress, matched: progress.matched + 1 }
            schedule()
          } else if (e.event === 'progress') {
            progress = data as Progress
            schedule()
          } else if (e.event === 'done') {
            progress = data as Progress
            finish({ status: 'done', result: data as BrowseResult, progress: data })
          } else if (e.event === 'error') {
            finish({ status: 'error', error: new Error(data.message) })
          }
        },
        ctrl.signal,
      )
        .then(() => setState((s) => (abort.current === ctrl && s.status === 'running' ? { ...s, status: 'done' } : s)))
        .catch((err: unknown) => {
          if (ctrl.signal.aborted) {
            finish({ status: 'done', result: { ...progress, reason: 'cancelled' } })
          } else {
            finish({ status: 'error', error: err instanceof Error ? err : new Error(String(err)) })
          }
        })
    },
    [clusterId, topic],
  )

  return { ...state, start, stop }
}

export function fetchMessage(clusterId: string, topic: string, m: Message, keyFormat: Format, valueFormat: Format) {
  const qs = new URLSearchParams({ keyFormat, valueFormat })
  return get<Message>(`${base(clusterId, topic)}/${m.partition}/${m.offset}?${qs}`)
}

export type ProduceInput = {
  partition: number | null
  key: string | null
  value: string | null
  keyFormat: Format
  valueFormat: Format
  headers: { key: string; value: string }[]
}
export type ProduceResult = { partition: number; offset: number; timestamp: number }

export function useProduce(clusterId: string, topic: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: ProduceInput) => api<ProduceResult>('POST', base(clusterId, topic), input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['clusters', clusterId, 'topics'] }),
  })
}
