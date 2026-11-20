import { Link, useParams } from '@tanstack/react-router'
import { PauseIcon, PlayIcon, RadioIcon } from 'lucide-react'
import { useTheme } from 'next-themes'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import ForceGraph2D, { type ForceGraphMethods as Methods2D } from 'react-force-graph-2d'
import ForceGraph3D, { type ForceGraphMethods as Methods3D } from 'react-force-graph-3d'
import { ErrorState } from '@/components/states'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { formatRate } from '@/features/analytics/api'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import {
  filterGraph,
  type GraphLink,
  type GraphNode,
  type GroupStatus,
  groupStatus,
  type NodeKind,
  statusLabel,
} from './graph'
import { useLive } from './use-live'

// dataviz reference palette: categorical slots for kinds, fixed status steps for group lag.
const palettes = {
  light: {
    bg: '#ffffff',
    broker: '#2a78d6',
    member: '#6250d6',
    topic: '#8a8a85',
    text: '#0b0b0b',
    link: 'rgba(82,81,78,0.35)',
  },
  dark: {
    bg: '#0a0a0a',
    broker: '#3987e5',
    member: '#9085e9',
    topic: '#9a9a94',
    text: '#f0efec',
    link: 'rgba(195,194,183,0.3)',
  },
}
const statusColor: Record<GroupStatus, string> = {
  good: '#0ca30c',
  warning: '#fab219',
  serious: '#ec835a',
  critical: '#d03b3b',
}
/** New nodes are drawn larger for this long. */
const highlightMs = 4000

const kindLabel: Record<NodeKind, string> = {
  broker: 'Broker',
  topic: 'Topic',
  group: 'Consumer group',
  member: 'Consumer',
}

const escapeHtml = (s: string) =>
  s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] as string)

function nodeSize(n: GraphNode, now: number) {
  const base =
    n.kind === 'broker' ? 14 : n.kind === 'group' ? 5 : n.kind === 'member' ? 1.5 : 1.5 + Math.log10(1 + n.rate) * 4 // topic: grows with ingress
  return now - n.bornAt < highlightMs ? base * 3 : base
}

function useSize(ref: React.RefObject<HTMLDivElement | null>) {
  const [size, setSize] = useState({ width: 800, height: 600 })
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const ro = new ResizeObserver(([e]) =>
      setSize({ width: Math.floor(e.contentRect.width), height: Math.floor(e.contentRect.height) }),
    )
    ro.observe(el)
    return () => ro.disconnect()
  }, [ref])
  return size
}

function Legend({ colors }: { colors: (typeof palettes)['light'] }) {
  const dot = (c: string) => <span className="inline-block size-2.5 rounded-full" style={{ background: c }} />
  return (
    <div className="flex flex-col gap-1.5 text-xs">
      <span className="flex items-center gap-2">{dot(colors.broker)} Broker (large; controller marked in details)</span>
      <span className="flex items-center gap-2">{dot(colors.topic)} Topic (size = ingress)</span>
      <span className="flex items-center gap-2">{dot(colors.member)} Consumer</span>
      <span className="mt-1 text-muted-foreground">Consumer groups by lag:</span>
      {(Object.keys(statusLabel) as GroupStatus[]).map((s) => (
        <span key={s} className="flex items-center gap-2">
          {dot(statusColor[s])} {statusLabel[s]}
        </span>
      ))}
      <span className="mt-1 text-muted-foreground">Moving particles = messages flowing (speed ∝ rate)</span>
    </div>
  )
}

function NodeDetails({ clusterId, node }: { clusterId: string; node: GraphNode }) {
  const row = (k: string, v: React.ReactNode) => (
    <div className="flex justify-between gap-4 border-b py-1.5 text-sm">
      <span className="text-muted-foreground">{k}</span>
      <span className="text-right font-mono">{v}</span>
    </div>
  )
  return (
    <div className="flex flex-col px-4">
      {node.kind === 'broker' && (
        <>
          {row('Role', node.controller ? 'active controller' : 'broker')}
          {row('Address', node.host)}
          {row('Partition leaders', node.leaders ?? 0)}
          {row('Ingress (as leader)', `${formatRate(node.rate)} msg/s`)}
          <Link to="/clusters/$clusterId/brokers" params={{ clusterId }} className="mt-3 text-sm hover:underline">
            Open brokers →
          </Link>
        </>
      )}
      {node.kind === 'topic' && (
        <>
          {row('Partitions', node.partitions)}
          {row('Ingress', `${formatRate(node.rate)} msg/s`)}
          {row('Internal', node.internal ? 'yes' : 'no')}
          <Link
            to="/clusters/$clusterId/topics/$topic"
            params={{ clusterId, topic: node.label }}
            className="mt-3 text-sm hover:underline"
          >
            Open topic →
          </Link>
        </>
      )}
      {node.kind === 'group' && (
        <>
          {row('State', node.state)}
          {row('Members', node.members ?? 0)}
          {row('Lag', formatNumber(node.lag ?? 0))}
          {row('Status', statusLabel[groupStatus(node)])}
          {row('Consumed', `${formatRate(node.rate)} msg/s`)}
          <Link
            to="/clusters/$clusterId/groups/$group"
            params={{ clusterId, group: node.label }}
            className="mt-3 text-sm hover:underline"
          >
            Open group →
          </Link>
        </>
      )}
      {node.kind === 'member' && (
        <>
          {row('Group', node.group)}
          {row('Client ID', node.clientId)}
          {row('Host', node.host)}
          {row('Assigned partitions', node.partitions ?? 0)}
          {row('Lag', formatNumber(node.lag ?? 0))}
          {row('Consumed', `${formatRate(node.rate)} msg/s`)}
          <Link
            to="/clusters/$clusterId/groups/$group"
            params={{ clusterId, group: node.group ?? '' }}
            className="mt-3 text-sm hover:underline"
          >
            Open group →
          </Link>
        </>
      )}
    </div>
  )
}

export function LivePage() {
  const { clusterId } = useParams({ from: '/clusters/$clusterId/live' })
  const { resolvedTheme } = useTheme()
  const colors = palettes[resolvedTheme === 'light' ? 'light' : 'dark']
  const [mode, setMode] = useState<'3d' | '2d'>('3d')
  const [paused, setPaused] = useState(false)
  const [hideInternal, setHideInternal] = useState(true)
  const [query, setQuery] = useState('')
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const live = useLive(clusterId, paused)
  const box = useRef<HTMLDivElement>(null)
  const { width, height } = useSize(box)
  const fg3 = useRef<Methods3D<GraphNode, GraphLink> | undefined>(undefined)
  const fg2 = useRef<Methods2D<GraphNode, GraphLink> | undefined>(undefined)
  // Fit the view once after the first layout settles (and again after switching 2D/3D).
  const fitted = useRef(false)
  // biome-ignore lint/correctness/useExhaustiveDependencies: refit when the mode changes
  useEffect(() => {
    fitted.current = false
  }, [mode, clusterId, query, hideInternal])
  const onEngineStop = useCallback(() => {
    if (fitted.current) return
    fitted.current = true
    fg3.current?.zoomToFit(600, 40)
    fg2.current?.zoomToFit(600, 40)
  }, [])

  // New data for the layout only when the structure or the filter changes, so
  // ordinary updates don't reheat the simulation.
  // biome-ignore lint/correctness/useExhaustiveDependencies: live.structure is the change signal for the mutable graph
  const data = useMemo(() => filterGraph(live.graph, { hideInternal, query }), [live.structure, hideInternal, query])
  const selected = selectedId ? live.graph.nodes.find((n) => n.id === selectedId) : undefined

  // Accessors change identity on every snapshot so colors and sizes refresh.
  // biome-ignore lint/correctness/useExhaustiveDependencies: version re-evaluates mutated nodes
  const nodeColor = useCallback(
    (n: GraphNode) =>
      n.kind === 'group'
        ? statusColor[groupStatus(n)]
        : n.kind === 'broker'
          ? colors.broker
          : n.kind === 'member'
            ? colors.member
            : colors.topic,
    [live.version, colors],
  )
  // biome-ignore lint/correctness/useExhaustiveDependencies: version re-evaluates mutated nodes
  const nodeVal = useCallback((n: GraphNode) => nodeSize(n, Date.now()), [live.version])
  const nodeLabel = useCallback(
    (n: GraphNode) =>
      `<div style="font: 12px system-ui; padding: 2px 4px"><b>${escapeHtml(n.label)}</b><br/>${kindLabel[n.kind]}${
        n.kind === 'group' ? ` · lag ${formatNumber(n.lag ?? 0)} · ${statusLabel[groupStatus(n)]}` : ''
      }${n.rate > 0 ? ` · ${formatRate(n.rate)} msg/s` : ''}${n.controller ? ' · controller' : ''}</div>`,
    [],
  )
  // biome-ignore lint/correctness/useExhaustiveDependencies: version re-evaluates mutated links
  const particles = useCallback(
    (l: GraphLink) => (l.kind !== 'offsets' && l.rate > 0 ? Math.min(4, 1 + Math.floor(Math.log10(1 + l.rate))) : 0),
    [live.version],
  )
  // biome-ignore lint/correctness/useExhaustiveDependencies: version re-evaluates mutated links
  const particleSpeed = useCallback((l: GraphLink) => 0.002 + (Math.min(l.rate, 200) / 200) * 0.012, [live.version])
  const linkWidth = useCallback(
    (l: GraphLink) => (l.kind === 'consumes' ? 0.6 + Math.min(l.partitions, 10) * 0.15 : 0.3),
    [],
  )
  const onNodeClick = useCallback((n: GraphNode) => setSelectedId(n.id), [])

  const drawLabel = useCallback(
    (n: GraphNode, ctx: CanvasRenderingContext2D, scale: number) => {
      if (n.kind === 'member' || (n.kind === 'topic' && scale < 2)) return
      const size = Math.max(10 / scale, 2)
      ctx.font = `${size}px system-ui`
      ctx.fillStyle = colors.text
      ctx.textAlign = 'center'
      ctx.fillText(n.label, n.x ?? 0, (n.y ?? 0) + Math.sqrt(nodeSize(n, Date.now())) * 4 + size)
    },
    [colors],
  )

  const counts = useMemo(() => {
    const c: Record<NodeKind, number> = { broker: 0, topic: 0, group: 0, member: 0 }
    for (const n of data.nodes) c[n.kind]++
    return c
  }, [data])

  const common = {
    graphData: data,
    width,
    height,
    backgroundColor: colors.bg,
    nodeId: 'id',
    nodeLabel,
    nodeColor,
    nodeVal,
    nodeRelSize: 3,
    linkColor: () => colors.link,
    linkWidth,
    linkDirectionalParticles: particles,
    linkDirectionalParticleSpeed: particleSpeed,
    linkDirectionalParticleWidth: 1.6,
    linkDirectionalParticleColor: (l: GraphLink) => (l.kind === 'leader' ? colors.broker : colors.member),
    onNodeClick,
    cooldownTicks: 120,
    onEngineStop,
    warmupTicks: 20,
  }

  return (
    <div className="flex h-[calc(100svh-7rem)] flex-col gap-3">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="mr-auto font-semibold text-2xl tracking-tight">Live topology</h1>
        <span className="flex items-center gap-1.5 text-muted-foreground text-xs" aria-live="polite">
          <RadioIcon
            className={cn('size-3.5', live.status === 'live' && !paused && 'animate-pulse text-emerald-500')}
          />
          {paused
            ? 'paused'
            : live.status === 'live'
              ? `updated ${new Date(live.lastAt).toLocaleTimeString()}`
              : live.status}
        </span>
        <span className="text-muted-foreground text-xs tabular-nums">
          {counts.broker} brokers · {counts.topic} topics · {counts.group} groups · {counts.member} consumers
        </span>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <fieldset className="inline-flex rounded-md border p-0.5">
          <legend className="sr-only">View</legend>
          {(['3d', '2d'] as const).map((m) => (
            <button
              key={m}
              type="button"
              aria-pressed={mode === m}
              onClick={() => setMode(m)}
              className={cn(
                'rounded px-2.5 py-1 font-medium text-xs uppercase',
                mode === m ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground',
              )}
            >
              {m}
            </button>
          ))}
        </fieldset>
        <Input
          aria-label="Filter by topic, group or consumer"
          placeholder="Filter by topic, group or consumer…"
          className="w-72"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <div className="flex items-center gap-2">
          <Switch id="live-internal" checked={hideInternal} onCheckedChange={setHideInternal} />
          <Label htmlFor="live-internal" className="font-normal">
            Hide internal topics
          </Label>
        </div>
        <Button variant="outline" size="sm" onClick={() => setPaused((p) => !p)}>
          {paused ? <PlayIcon /> : <PauseIcon />} {paused ? 'Resume' : 'Pause'}
        </Button>
      </div>
      {live.status === 'error' && live.error && <ErrorState error={live.error} title="Live updates interrupted" />}
      <div className="relative min-h-0 flex-1 overflow-hidden rounded-lg border">
        <div ref={box} className="absolute inset-0">
          {mode === '3d' ? (
            <ForceGraph3D<GraphNode, GraphLink> ref={fg3} {...common} nodeResolution={10} nodeOpacity={0.95} />
          ) : (
            <ForceGraph2D<GraphNode, GraphLink>
              ref={fg2}
              {...common}
              nodeCanvasObjectMode={() => 'after'}
              nodeCanvasObject={drawLabel}
            />
          )}
        </div>
        <details className="absolute bottom-3 left-3 rounded-md border bg-background/85 p-3 backdrop-blur" open>
          <summary className="cursor-pointer font-medium text-sm">Legend</summary>
          <div className="mt-2">
            <Legend colors={colors} />
          </div>
        </details>
        <div className="absolute top-3 right-3 max-h-[45%] w-72 overflow-y-auto rounded-md border bg-background/85 p-3 text-xs backdrop-blur">
          <p className="mb-1.5 font-medium text-sm">Recent changes</p>
          {live.changes.length === 0 ? (
            <p className="text-muted-foreground">Consumers joining or leaving appear here.</p>
          ) : (
            <ul className="flex flex-col gap-1">
              {live.changes.map((c) => (
                <li key={`${c.at}-${c.kind}-${c.node.id}`} className="flex items-start gap-1.5">
                  <Badge variant={c.kind === 'joined' ? 'secondary' : 'outline'} className="shrink-0">
                    {c.kind}
                  </Badge>
                  <span className="min-w-0 break-all">
                    {c.node.kind === 'member' ? `${c.node.label} → ${c.node.group}` : `group ${c.node.label}`}
                    <span className="block text-muted-foreground">{new Date(c.at).toLocaleTimeString()}</span>
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
      <Sheet open={!!selected} onOpenChange={(o) => !o && setSelectedId(null)}>
        <SheetContent>
          {selected && (
            <>
              <SheetHeader>
                <SheetTitle className="break-all">{selected.label}</SheetTitle>
                <SheetDescription>{kindLabel[selected.kind]}</SheetDescription>
              </SheetHeader>
              <NodeDetails clusterId={clusterId} node={selected} />
            </>
          )}
        </SheetContent>
      </Sheet>
    </div>
  )
}
