import { Link, useParams } from '@tanstack/react-router'
import { CrownIcon, PauseIcon, PlayIcon, RadioIcon, ServerIcon, UsersIcon } from 'lucide-react'
import { useCallback, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { ErrorState } from '@/components/states'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { formatRate } from '@/features/analytics/api'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import {
  buildFlow,
  type Flow,
  type FlowGroup,
  type FlowTopic,
  filterGraph,
  flowSeconds,
  flowWidth,
  type GraphNode,
  type GroupStatus,
  groupStatus,
  type NodeKind,
  statusLabel,
} from './graph'
import { useLive } from './use-live'

// dataviz status palette (fixed); always paired with a text label.
const statusColor: Record<GroupStatus, string> = {
  good: '#0ca30c',
  warning: '#fab219',
  serious: '#ec835a',
  critical: '#d03b3b',
}
/** New consumers are highlighted for this long. */
const highlightMs = 6000

const kindLabel: Record<NodeKind, string> = {
  broker: 'Broker',
  topic: 'Topic',
  group: 'Consumer group',
  member: 'Consumer',
}

function StatusPill({ status }: { status: GroupStatus }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs">
      <span className="size-2 rounded-full" style={{ background: statusColor[status] }} />
      {statusLabel[status]}
    </span>
  )
}

function RateBar({ rate, max, label }: { rate: number; max: number; label: string }) {
  return (
    <div className="flex items-center gap-2 text-xs">
      <span className="w-7 text-muted-foreground">{label}</span>
      <div className="h-1.5 flex-1 rounded-full bg-muted">
        <div
          className="h-1.5 rounded-full"
          style={{
            width: `${rate > 0 ? Math.max(4, Math.sqrt(rate / max) * 100) : 0}%`,
            background: 'var(--series-1)',
          }}
        />
      </div>
      <span className="w-16 text-right font-mono tabular-nums">{formatRate(rate)}/s</span>
    </div>
  )
}

function BrokerCard({ node, onSelect }: { node: GraphNode; onSelect: () => void }) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className="flex min-w-44 flex-col gap-1 rounded-lg border-2 bg-card px-3 py-2 text-left hover:bg-accent"
      style={{ borderColor: 'color-mix(in oklab, var(--series-1) 60%, transparent)' }}
    >
      <span className="flex items-center gap-2 font-medium text-sm">
        <ServerIcon className="size-4" style={{ color: 'var(--series-1)' }} /> {node.label}
        {node.controller && (
          <Badge variant="secondary" className="gap-1">
            <CrownIcon className="size-3" /> controller
          </Badge>
        )}
      </span>
      <span className="text-muted-foreground text-xs tabular-nums">
        {node.leaders ?? 0} leaders · in {formatRate(node.rate)} msg/s
      </span>
    </button>
  )
}

function TopicCard({
  topic,
  max,
  onSelect,
  refCb,
}: {
  topic: FlowTopic
  max: number
  onSelect: () => void
  refCb: (el: HTMLElement | null) => void
}) {
  const n = topic.node
  return (
    <button
      type="button"
      ref={refCb}
      onClick={onSelect}
      className="flex w-full flex-col gap-1.5 rounded-md border bg-card px-3 py-2 text-left hover:bg-accent"
    >
      <span className="flex items-center gap-2">
        <span className="truncate font-medium font-mono text-sm" title={n.label}>
          {n.label}
        </span>
        {n.internal && <Badge variant="outline">internal</Badge>}
        <span className="ml-auto shrink-0 text-muted-foreground text-xs">{n.partitions} part.</span>
      </span>
      <RateBar rate={n.rate} max={max} label="in" />
      <RateBar rate={topic.outRate} max={max} label="out" />
      {topic.leaders.length > 0 && (
        <span className="flex flex-wrap gap-1">
          {topic.leaders.map((l) => (
            <span key={l.broker} className="rounded bg-muted px-1.5 text-[10px] text-muted-foreground">
              {l.broker.replace('broker ', 'b')} ×{l.partitions}
            </span>
          ))}
        </span>
      )}
    </button>
  )
}

function GroupCard({
  group,
  now,
  onSelect,
  refCb,
}: {
  group: FlowGroup
  now: number
  onSelect: (n: GraphNode) => void
  refCb: (el: HTMLElement | null) => void
}) {
  const n = group.node
  const shown = group.members.slice(0, 8)
  return (
    <div
      ref={refCb}
      className="flex w-full flex-col gap-2 rounded-md border border-l-4 bg-card px-3 py-2"
      style={{ borderLeftColor: statusColor[group.status] }}
    >
      <button type="button" onClick={() => onSelect(n)} className="flex items-center gap-2 text-left hover:underline">
        <UsersIcon className="size-4 shrink-0 text-muted-foreground" />
        <span className="truncate font-medium text-sm" title={n.label}>
          {n.label}
        </span>
        <Badge variant="outline" className="ml-auto shrink-0">
          {n.state}
        </Badge>
      </button>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
        <StatusPill status={group.status} />
        <span className="tabular-nums">lag {formatNumber(n.lag ?? 0)}</span>
        <span className="tabular-nums">consumes {formatRate(n.rate)}/s</span>
      </div>
      {group.members.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {shown.map((m) => {
            const isNew = now - m.bornAt < highlightMs
            return (
              <button
                type="button"
                key={m.id}
                onClick={() => onSelect(m)}
                title={`${m.label} · ${m.partitions ?? 0} partitions · ${formatRate(m.rate)}/s`}
                className={cn(
                  'max-w-full truncate rounded-full border px-2 py-0.5 font-mono text-[10px] hover:bg-accent',
                  isNew && 'animate-pulse',
                )}
                style={
                  isNew
                    ? {
                        borderColor: 'var(--series-1)',
                        background: 'color-mix(in oklab, var(--series-1) 15%, transparent)',
                      }
                    : undefined
                }
              >
                {isNew && <span className="mr-1 font-sans font-semibold">new</span>}
                {m.clientId}@{m.host}
              </button>
            )
          })}
          {group.members.length > shown.length && (
            <span className="text-muted-foreground text-xs">+{group.members.length - shown.length} more</span>
          )}
        </div>
      ) : (
        <span className="text-muted-foreground text-xs">no active consumers</span>
      )}
    </div>
  )
}

type Path = { key: string; d: string; width: number; seconds: number; label?: { x: number; y: number; text: string } }
type Paths = { ingress: Path[]; flows: Path[]; width: number; height: number }

/** Measures card positions and returns SVG paths for ingress arrows and topic → group flows. */
function useFlowPaths(
  container: React.RefObject<HTMLDivElement | null>,
  els: React.RefObject<Map<string, HTMLElement>>,
  flow: Flow,
): Paths {
  const [paths, setPaths] = useState<Paths>({ ingress: [], flows: [], width: 0, height: 0 })
  const measure = useCallback(() => {
    const root = container.current
    if (!root) return
    const box = root.getBoundingClientRect()
    const rect = (id: string) => {
      const el = els.current.get(id)
      if (!el) return undefined
      const r = el.getBoundingClientRect()
      return { left: r.left - box.left, right: r.right - box.left, mid: r.top - box.top + r.height / 2 }
    }
    const ingress: Path[] = []
    for (const t of flow.topics) {
      const r = rect(t.node.id)
      if (!r) continue
      const w = flowWidth(t.node.rate, flow.maxRate)
      ingress.push({
        key: `in:${t.node.id}`,
        d: `M 16 ${r.mid} L ${r.left - 6} ${r.mid}`,
        width: w,
        seconds: flowSeconds(t.node.rate),
        label: t.node.rate > 0 ? { x: 16, y: r.mid - w / 2 - 5, text: `${formatRate(t.node.rate)}/s` } : undefined,
      })
    }
    const flows: Path[] = []
    for (const e of flow.edges) {
      const a = rect(e.topicId)
      const b = rect(e.groupId)
      if (!a || !b) continue
      const x1 = a.right + 2
      const x2 = b.left - 6
      const cx = (x1 + x2) / 2
      flows.push({
        key: `${e.topicId}->${e.groupId}`,
        d: `M ${x1} ${a.mid} C ${cx} ${a.mid}, ${cx} ${b.mid}, ${x2} ${b.mid}`,
        width: flowWidth(e.rate, flow.maxRate),
        seconds: flowSeconds(e.rate),
        label: e.rate > 0 ? { x: cx, y: (a.mid + b.mid) / 2 - 8, text: `${formatRate(e.rate)}/s` } : undefined,
      })
    }
    setPaths({ ingress, flows, width: root.scrollWidth, height: root.scrollHeight })
  }, [container, els, flow])

  useLayoutEffect(() => {
    measure()
    const ro = new ResizeObserver(measure)
    if (container.current) ro.observe(container.current)
    return () => ro.disconnect()
  }, [measure, container])
  return paths
}

function FlowPath({ p, active }: { p: Path; active: boolean }) {
  return (
    <>
      <path d={p.d} fill="none" stroke="var(--border)" strokeWidth={Math.max(p.width, 1.5)} strokeLinecap="round" />
      {p.seconds > 0 && (
        <path
          d={p.d}
          fill="none"
          stroke="var(--series-1)"
          strokeWidth={p.width}
          strokeLinecap="round"
          className={cn('flow-dash', !active && 'flow-dash-paused')}
          style={{ animationDuration: `${p.seconds}s` }}
        />
      )}
    </>
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
        </>
      )}
      {(node.kind === 'group' || node.kind === 'member') && (
        <Link
          to="/clusters/$clusterId/groups/$group"
          params={{ clusterId, group: node.kind === 'group' ? node.label : (node.group ?? '') }}
          className="mt-3 text-sm hover:underline"
        >
          Open group →
        </Link>
      )}
    </div>
  )
}

export function LivePage() {
  const { clusterId } = useParams({ from: '/clusters/$clusterId/live' })
  const [paused, setPaused] = useState(false)
  const [hideInternal, setHideInternal] = useState(true)
  const [query, setQuery] = useState('')
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const live = useLive(clusterId, paused)

  // biome-ignore lint/correctness/useExhaustiveDependencies: version is the change signal for the mutable graph
  const flow = useMemo(
    () => buildFlow(filterGraph(live.graph, { hideInternal, query })),
    [live.version, hideInternal, query],
  )
  const selected = selectedId ? live.graph.nodes.find((n) => n.id === selectedId) : undefined
  const now = live.lastAt || Date.now()

  const container = useRef<HTMLDivElement>(null)
  const els = useRef(new Map<string, HTMLElement>())
  const refs = useRef(new Map<string, (el: HTMLElement | null) => void>())
  // Stable ref callbacks per id, so React doesn't detach/attach them on every render.
  const setEl = useCallback((id: string) => {
    let cb = refs.current.get(id)
    if (!cb) {
      cb = (el) => {
        if (el) els.current.set(id, el)
        else els.current.delete(id)
      }
      refs.current.set(id, cb)
    }
    return cb
  }, [])
  const paths = useFlowPaths(container, els, flow)
  const totalIn = flow.topics.reduce((s, t) => s + t.node.rate, 0)
  const totalOut = flow.groups.reduce((s, g) => s + g.node.rate, 0)

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="mr-auto font-semibold text-2xl tracking-tight">Live topology</h1>
        <span className="text-sm tabular-nums">
          <span className="text-muted-foreground">produced</span> {formatRate(totalIn)} msg/s ·{' '}
          <span className="text-muted-foreground">consumed</span> {formatRate(totalOut)} msg/s
        </span>
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
      </div>
      <div className="flex flex-wrap items-center gap-3">
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
        <span className="ml-auto flex flex-wrap items-center gap-3 text-muted-foreground text-xs">
          <span>Line width and speed = msg/s</span>
          {(Object.keys(statusLabel) as GroupStatus[]).map((s) => (
            <StatusPill key={s} status={s} />
          ))}
        </span>
      </div>
      {live.status === 'error' && live.error && <ErrorState error={live.error} title="Live updates interrupted" />}

      <section aria-label="Brokers" className="flex flex-wrap gap-3">
        {flow.brokers.map((b) => (
          <BrokerCard key={b.id} node={b} onSelect={() => setSelectedId(b.id)} />
        ))}
      </section>

      <div className="grid gap-4 2xl:grid-cols-[1fr_18rem]">
        <div className="overflow-x-auto rounded-lg border">
          <div ref={container} className="relative min-w-[46rem] p-4">
            <svg
              className="pointer-events-none absolute inset-0"
              width={paths.width}
              height={paths.height}
              aria-hidden="true"
            >
              {paths.ingress.map((p) => (
                <FlowPath key={p.key} p={p} active={!paused} />
              ))}
              {paths.flows.map((p) => (
                <FlowPath key={p.key} p={p} active={!paused} />
              ))}
              {[...paths.ingress, ...paths.flows].map(
                (p) =>
                  p.label && (
                    <text
                      key={`label:${p.key}`}
                      x={p.label.x}
                      y={p.label.y}
                      textAnchor={p.key.startsWith('in:') ? 'start' : 'middle'}
                      className="fill-foreground font-mono text-[11px]"
                      style={{ paintOrder: 'stroke', stroke: 'var(--background)', strokeWidth: 4 }}
                    >
                      {p.label.text}
                    </text>
                  ),
              )}
            </svg>
            <div className="relative grid grid-cols-[6rem_16rem_minmax(6rem,1fr)_minmax(18rem,22rem)] gap-y-3">
              <div className="col-start-1 font-medium text-muted-foreground text-xs uppercase tracking-wide">
                Produced
              </div>
              <div className="col-start-2 font-medium text-muted-foreground text-xs uppercase tracking-wide">
                Topics
              </div>
              <div className="col-start-4 font-medium text-muted-foreground text-xs uppercase tracking-wide">
                Consumer groups
              </div>
              <div className="col-start-2 row-start-2 flex flex-col gap-3">
                {flow.topics.length === 0 && <p className="text-muted-foreground text-sm">No topics match.</p>}
                {flow.topics.map((t) => (
                  <TopicCard
                    key={t.node.id}
                    topic={t}
                    max={flow.maxRate}
                    onSelect={() => setSelectedId(t.node.id)}
                    refCb={setEl(t.node.id)}
                  />
                ))}
              </div>
              <div className="col-start-4 row-start-2 flex flex-col gap-3">
                {flow.groups.length === 0 && <p className="text-muted-foreground text-sm">No consumer groups match.</p>}
                {flow.groups.map((g) => (
                  <GroupCard
                    key={g.node.id}
                    group={g}
                    now={now}
                    onSelect={(n) => setSelectedId(n.id)}
                    refCb={setEl(g.node.id)}
                  />
                ))}
              </div>
            </div>
          </div>
        </div>

        <Card className="h-fit">
          <CardHeader>
            <CardTitle>Recent changes</CardTitle>
          </CardHeader>
          <CardContent className="max-h-[60vh] overflow-y-auto text-xs">
            {live.changes.length === 0 ? (
              <p className="text-muted-foreground">Consumers joining or leaving appear here.</p>
            ) : (
              <ul className="flex flex-col gap-2">
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
          </CardContent>
        </Card>
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
