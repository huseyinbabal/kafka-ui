import { Link, useNavigate, useParams } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { AlertTriangleIcon, InfoIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { DataTable } from '@/components/data-table'
import { PageHeader } from '@/components/page'
import { EmptyState, ErrorState, LoadingRows } from '@/components/states'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { formatBytes, formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import {
  formatDuration,
  formatRate,
  type GroupStat,
  type PartitionRate,
  type Range,
  ranges,
  type TopicStat,
  useHygiene,
  useOverview,
  useSkew,
} from './api'
import { TimeChart } from './time-chart'

export function RangePicker({ value, onChange }: { value: Range; onChange: (r: Range) => void }) {
  return (
    <fieldset className="inline-flex rounded-md border p-0.5">
      <legend className="sr-only">Time range</legend>
      {ranges.map((r) => (
        <button
          key={r}
          type="button"
          aria-pressed={value === r}
          onClick={() => onChange(r)}
          className={cn(
            'rounded px-2.5 py-1 font-medium text-xs tabular-nums transition-colors',
            value === r ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground',
          )}
        >
          {r}
        </button>
      ))}
    </fieldset>
  )
}

function useTopicColumns(clusterId: string): ColumnDef<TopicStat>[] {
  return useMemo(
    () => [
      {
        accessorKey: 'topic',
        header: 'Topic',
        cell: ({ getValue }) => (
          <Link
            to="/clusters/$clusterId/topics/$topic"
            params={{ clusterId, topic: getValue<string>() }}
            className="font-medium hover:underline"
          >
            {getValue<string>()}
          </Link>
        ),
      },
      {
        accessorKey: 'msgRate',
        header: 'Msg/s',
        meta: { align: 'right' },
        cell: ({ getValue }) => formatRate(getValue<number>()),
      },
      {
        accessorKey: 'messages',
        header: 'Produced',
        meta: { align: 'right' },
        cell: ({ getValue }) => formatNumber(getValue<number>()),
      },
      {
        accessorKey: 'sizeBytes',
        header: 'Size',
        meta: { align: 'right' },
        cell: ({ getValue }) => formatBytes(getValue<number>()),
      },
      {
        accessorKey: 'growthBytes',
        header: 'Growth',
        meta: { align: 'right' },
        cell: ({ getValue }) => {
          const v = getValue<number>()
          return v === 0 ? (
            <span className="text-muted-foreground">0</span>
          ) : (
            `${v > 0 ? '+' : '−'}${formatBytes(Math.abs(v))}`
          )
        },
      },
    ],
    [clusterId],
  )
}

function useGroupColumns(clusterId: string): ColumnDef<GroupStat>[] {
  return useMemo(
    () => [
      {
        accessorKey: 'group',
        header: 'Group',
        cell: ({ getValue }) => (
          <Link
            to="/clusters/$clusterId/groups/$group"
            params={{ clusterId, group: getValue<string>() }}
            className="font-medium hover:underline"
          >
            {getValue<string>()}
          </Link>
        ),
      },
      {
        accessorKey: 'lag',
        header: 'Lag',
        meta: { align: 'right' },
        cell: ({ getValue }) => formatNumber(getValue<number>()),
      },
      {
        accessorKey: 'consumeRate',
        header: 'Consume/s',
        meta: { align: 'right' },
        cell: ({ getValue }) => formatRate(getValue<number>()),
      },
      {
        accessorKey: 'ingressRate',
        header: 'Ingress/s',
        meta: { align: 'right' },
        cell: ({ getValue }) => formatRate(getValue<number>()),
      },
      {
        id: 'catchUp',
        header: 'Catch up in',
        meta: { align: 'right' },
        accessorFn: (g) => g.catchUpSeconds ?? (g.lag > 0 ? Number.POSITIVE_INFINITY : 0),
        cell: ({ row }) => {
          const g = row.original
          if (g.lag === 0) return <span className="text-muted-foreground">caught up</span>
          if (g.consumeRate === 0) return <span className="text-muted-foreground">not consuming</span>
          if (g.catchUpSeconds === null)
            return (
              <span className="inline-flex items-center gap-1 text-amber-600 dark:text-amber-400">
                <AlertTriangleIcon className="size-3.5" /> falling behind
              </span>
            )
          return formatDuration(g.catchUpSeconds)
        },
      },
    ],
    [clusterId],
  )
}

/** Partitions taking more than 1.5× their fair share of a topic's ingress. */
export function hotPartitions(parts: PartitionRate[]) {
  const byTopic = new Map<string, PartitionRate[]>()
  for (const p of parts) byTopic.set(p.topic, [...(byTopic.get(p.topic) ?? []), p])
  const out: (PartitionRate & { share: number; partitions: number })[] = []
  for (const [, ps] of byTopic) {
    const total = ps.reduce((s, p) => s + p.rate, 0)
    if (ps.length < 2 || total <= 0) continue
    for (const p of ps) {
      const share = p.rate / total
      if (share > 1.5 / ps.length) out.push({ ...p, share, partitions: ps.length })
    }
  }
  return out.sort((a, b) => b.share - a.share)
}

function SkewCard({ clusterId }: { clusterId: string }) {
  const { data, error, isPending } = useSkew(clusterId)
  const hot = useMemo(() => hotPartitions(data?.partitions ?? []), [data])
  const maxLeaders = Math.max(1, ...(data?.brokers ?? []).map((b) => b.leaders))
  return (
    <Card>
      <CardHeader>
        <CardTitle>Partition skew</CardTitle>
        <CardDescription>
          Leader spread across brokers and partitions taking an outsized share of ingress (last minute).
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-5">
        {error ? (
          <ErrorState error={error} />
        ) : isPending ? (
          <LoadingRows rows={3} />
        ) : (
          <>
            <div className="flex flex-col gap-2">
              {data.brokers.map((b) => (
                <div key={b.broker} className="grid grid-cols-[5rem_1fr_auto] items-center gap-3 text-sm">
                  <span className="text-muted-foreground">Broker {b.broker}</span>
                  <div className="h-2 rounded-full bg-muted" title={`${b.leaders} leaders`}>
                    <div
                      className="h-2 rounded-full"
                      style={{ width: `${(b.leaders / maxLeaders) * 100}%`, background: 'var(--series-1)' }}
                    />
                  </div>
                  <span className="whitespace-nowrap text-xs tabular-nums">
                    {b.leaders} leaders · {b.replicas} replicas · {formatRate(b.ingress)} msg/s
                  </span>
                </div>
              ))}
            </div>
            {hot.length === 0 ? (
              <p className="text-muted-foreground text-sm">No hot partitions: ingress is spread evenly.</p>
            ) : (
              <ul className="flex flex-col gap-1.5 text-sm">
                {hot.slice(0, 8).map((p) => (
                  <li key={`${p.topic}-${p.partition}`} className="flex flex-wrap items-center gap-2">
                    <AlertTriangleIcon className="size-3.5 text-amber-600 dark:text-amber-400" />
                    <Link
                      to="/clusters/$clusterId/topics/$topic"
                      params={{ clusterId, topic: p.topic }}
                      className="font-medium hover:underline"
                    >
                      {p.topic}
                    </Link>
                    <span>
                      partition {p.partition} takes {Math.round(p.share * 100)}% of ingress (fair share{' '}
                      {Math.round(100 / p.partitions)}%), leader broker {p.leader}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </>
        )}
      </CardContent>
    </Card>
  )
}

const kindLabel: Record<string, string> = {
  'low-rf': 'Low replication',
  'minisr-too-high': 'min.insync.replicas',
  'minisr-weak': 'Weak durability',
  'under-replicated': 'Under-replicated',
  'empty-group': 'Empty group',
}

function HygieneCard({ clusterId }: { clusterId: string }) {
  const { data, error, isPending } = useHygiene(clusterId)
  return (
    <Card>
      <CardHeader>
        <CardTitle>Hygiene</CardTitle>
        <CardDescription>Replication settings, idle topics and empty groups.</CardDescription>
      </CardHeader>
      <CardContent>
        {error ? (
          <ErrorState error={error} />
        ) : isPending ? (
          <LoadingRows rows={3} />
        ) : data.issues.length === 0 && data.idleTopics.length === 0 ? (
          <p className="text-muted-foreground text-sm">Nothing to report.</p>
        ) : (
          <ul className="flex flex-col gap-2 text-sm">
            {data.issues.map((i) => (
              <li key={`${i.kind}-${i.target}`} className="flex items-start gap-2">
                {i.severity === 'warning' ? (
                  <AlertTriangleIcon className="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400" />
                ) : (
                  <InfoIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                )}
                <div className="min-w-0">
                  <span className="font-medium font-mono">{i.target}</span>{' '}
                  <Badge variant={i.severity === 'warning' ? 'outline' : 'secondary'} className="ml-1">
                    {i.severity === 'warning' ? 'Warning' : 'Info'} · {kindLabel[i.kind] ?? i.kind}
                  </Badge>
                  <p className="text-muted-foreground">{i.message}</p>
                </div>
              </li>
            ))}
            {data.idleTopics.map((t) => (
              <li key={`idle-${t}`} className="flex items-start gap-2">
                <InfoIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                <div>
                  <Link
                    to="/clusters/$clusterId/topics/$topic"
                    params={{ clusterId, topic: t }}
                    className="font-medium font-mono hover:underline"
                  >
                    {t}
                  </Link>{' '}
                  <Badge variant="secondary" className="ml-1">
                    Info · Idle topic
                  </Badge>
                  <p className="text-muted-foreground">
                    No new messages since {new Date(data.idleSince).toLocaleString()} (checks up to 24h of collected
                    data).
                  </p>
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

export function AnalyticsPage() {
  const { clusterId } = useParams({ from: '/clusters/$clusterId/analytics' })
  const navigate = useNavigate()
  const [range, setRange] = useState<Range>('1h')
  const { data, error, isPending } = useOverview(clusterId, range)
  const topicColumns = useTopicColumns(clusterId)
  const groupColumns = useGroupColumns(clusterId)

  const throughput = useMemo(() => data?.throughput.map((p) => ({ ts: p.ts, messages: p.messages })) ?? [], [data])
  const bytes = useMemo(() => data?.bytes.map((p) => ({ ts: p.ts, bytes: p.bytes })) ?? [], [data])
  const lag = useMemo(() => data?.lag.map((p) => ({ ts: p.ts, lag: p.lag })) ?? [], [data])
  const partial = data && data.collectedSince > data.from

  return (
    <>
      <PageHeader
        title="Analytics"
        description={
          data?.collectedSince
            ? partial
              ? `Collected since ${new Date(data.collectedSince).toLocaleString()}; earlier parts of the range are empty.`
              : `Last ${range} · ${Math.round(data.step / 1000)}s buckets`
            : undefined
        }
        actions={<RangePicker value={range} onChange={setRange} />}
      />
      {error ? (
        <ErrorState error={error} />
      ) : isPending ? (
        <LoadingRows rows={8} />
      ) : data.collectedSince === 0 ? (
        <EmptyState
          title="Collecting the first samples"
          description="The collector samples offsets and lag every few seconds. Charts fill in within a minute."
        />
      ) : (
        <>
          <div className="grid gap-4 xl:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>Messages in / s</CardTitle>
                <CardDescription>All non-internal topics</CardDescription>
              </CardHeader>
              <CardContent>
                <TimeChart
                  data={throughput}
                  series={[{ key: 'messages', label: 'msg/s' }]}
                  from={data.from}
                  to={data.to}
                  format={(n) => `${formatRate(n)} msg/s`}
                />
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>Bytes in / s</CardTitle>
                <CardDescription>
                  Estimated from log size growth, in {Math.round((data.bytesStep ?? 60000) / 60000)}m buckets
                </CardDescription>
              </CardHeader>
              <CardContent>
                <TimeChart
                  data={bytes}
                  series={[{ key: 'bytes', label: 'bytes/s' }]}
                  from={data.from}
                  to={data.to}
                  format={(n) => `${formatBytes(n)}/s`}
                />
              </CardContent>
            </Card>
          </div>
          <div className="grid gap-4">
            <Card>
              <CardHeader>
                <CardTitle>Consumer lag</CardTitle>
                <CardDescription>Sum over all groups (partitions with a committed offset)</CardDescription>
              </CardHeader>
              <CardContent>
                <TimeChart
                  data={lag}
                  series={[{ key: 'lag', label: 'lag' }]}
                  from={data.from}
                  to={data.to}
                  format={(n) => `${formatNumber(n)} messages`}
                />
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>Groups by lag</CardTitle>
                <CardDescription>Rates over the last 5 minutes; catch-up = lag ÷ (consume − ingress)</CardDescription>
              </CardHeader>
              <CardContent>
                <DataTable
                  columns={groupColumns}
                  data={data.groups}
                  getRowId={(g) => g.group}
                  onRowClick={(g) =>
                    navigate({ to: '/clusters/$clusterId/groups/$group', params: { clusterId, group: g.group } })
                  }
                  emptyState={<EmptyState title="No groups with commits" />}
                />
              </CardContent>
            </Card>
          </div>
          <Card>
            <CardHeader>
              <CardTitle>Topics</CardTitle>
              <CardDescription>Ingress, size and growth over the selected range</CardDescription>
            </CardHeader>
            <CardContent>
              <DataTable
                columns={topicColumns}
                data={data.topTopics}
                getRowId={(t) => t.topic}
                searchPlaceholder="Search topics…"
                emptyState={<EmptyState title="No topics" />}
              />
            </CardContent>
          </Card>
          <div className="grid gap-4 xl:grid-cols-2">
            <SkewCard clusterId={clusterId} />
            <HygieneCard clusterId={clusterId} />
          </div>
        </>
      )}
    </>
  )
}
