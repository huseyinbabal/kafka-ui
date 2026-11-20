import { useEffect, useRef, useState } from 'react'
import { ApiError, enc } from '@/lib/api'
import { openSSE } from '@/lib/sse'
import { type Change, emptyGraph, type Graph, mergeSnapshot, type Snapshot } from './graph'

const maxChanges = 30

export type LiveState = {
  /** Mutable graph; node/link objects keep identity across snapshots. */
  graph: Graph
  /** Bumps on every applied snapshot (re-evaluate colors and sizes). */
  version: number
  /** Bumps only when nodes or links were added or removed (layout needs new data). */
  structure: number
  changes: Change[]
  lastAt: number
  status: 'connecting' | 'live' | 'error'
  error?: Error
}

/** Streams topology snapshots; while paused, snapshots are dropped. */
export function useLive(clusterId: string, paused: boolean): LiveState {
  const graph = useRef<Graph>(emptyGraph)
  const pausedRef = useRef(paused)
  pausedRef.current = paused
  const [state, setState] = useState<LiveState>({
    graph: emptyGraph,
    version: 0,
    structure: 0,
    changes: [],
    lastAt: 0,
    status: 'connecting',
  })

  useEffect(() => {
    graph.current = emptyGraph
    setState({ graph: emptyGraph, version: 0, structure: 0, changes: [], lastAt: 0, status: 'connecting' })
    const ctrl = new AbortController()
    let retry: ReturnType<typeof setTimeout> | undefined

    const connect = () => {
      openSSE(
        `/clusters/${enc(clusterId)}/live`,
        (e) => {
          if (e.event === 'error') {
            const data = JSON.parse(e.data)
            setState((s) => ({ ...s, status: 'error', error: new ApiError(502, data.code, data.message) }))
            return
          }
          if (e.event !== 'snapshot' || pausedRef.current) return
          const snap = JSON.parse(e.data) as Snapshot
          const res = mergeSnapshot(graph.current, snap, Date.now())
          graph.current = res.graph
          setState((s) => ({
            graph: res.graph,
            version: s.version + 1,
            structure: res.structural ? s.structure + 1 : s.structure,
            changes: [...res.changes.reverse(), ...s.changes].slice(0, maxChanges),
            lastAt: snap.at,
            status: 'live',
          }))
        },
        ctrl.signal,
      )
        .then(() => {
          // Server closed the stream (e.g. redeploy): reconnect.
          if (!ctrl.signal.aborted) retry = setTimeout(connect, 2000)
        })
        .catch((err: unknown) => {
          if (ctrl.signal.aborted) return
          const error = err instanceof Error ? err : new Error(String(err))
          setState((s) => ({ ...s, status: 'error', error }))
          // Client errors (unknown cluster, offline) won't fix themselves quickly; back off.
          const status = err instanceof ApiError ? err.status : 0
          retry = setTimeout(connect, status >= 400 && status < 500 ? 15_000 : 3000)
        })
    }
    connect()
    return () => {
      ctrl.abort()
      clearTimeout(retry)
    }
  }, [clusterId])

  return state
}
