import { Link, useNavigate, useParams } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { ChevronLeftIcon } from 'lucide-react'
import { useState } from 'react'
import { ConfigTable } from '@/components/config-table'
import { DataTable } from '@/components/data-table'
import { PageHeader, StatCard } from '@/components/page'
import { EmptyState, ErrorState, LoadingRows } from '@/components/states'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { TopicThroughput } from '@/features/analytics/lazy'
import { useCanMutate } from '@/features/auth/api'
import { useCluster } from '@/features/clusters/api'
import { StateBadge } from '@/features/groups/state-badge'
import { MessagesTab } from '@/features/messages/messages-tab'
import { ProduceDialog } from '@/features/messages/produce-dialog'
import { TopicSchemas } from '@/features/schemas/topic-schemas'
import type { ConfigEntry, GroupSummary, Partition } from '@/lib/api'
import { formatBytes, formatNumber, formatOffset } from '@/lib/format'
import { useTopic, useTopicConsumers } from './api'
import { ConfigEditDialog } from './config-edit-dialog'
import { TopicActions } from './topic-actions'

function ReplicaList({ p }: { p: Partition }) {
  return (
    <span className="inline-flex flex-wrap gap-1">
      {p.replicas.map((r) => {
        const inSync = p.isr.includes(r)
        return (
          <Badge
            key={r}
            variant={r === p.leader ? 'default' : 'outline'}
            className={inSync ? 'tabular-nums' : 'border-red-500/50 text-red-500 tabular-nums'}
            title={r === p.leader ? 'leader' : inSync ? 'in sync' : 'out of sync'}
          >
            {r}
          </Badge>
        )
      })}
    </span>
  )
}

const partitionColumns: ColumnDef<Partition>[] = [
  { accessorKey: 'id', header: 'Partition' },
  { accessorKey: 'leader', header: 'Leader' },
  {
    id: 'replicas',
    header: 'Replicas (ISR)',
    cell: ({ row }) => <ReplicaList p={row.original} />,
    enableSorting: false,
  },
  {
    accessorKey: 'startOffset',
    header: 'Start offset',
    meta: { align: 'right' },
    cell: ({ getValue }) => formatOffset(getValue<number>()),
  },
  {
    accessorKey: 'endOffset',
    header: 'End offset',
    meta: { align: 'right' },
    cell: ({ getValue }) => formatOffset(getValue<number>()),
  },
  {
    id: 'messages',
    header: 'Messages',
    meta: { align: 'right' },
    accessorFn: (p) => (p.startOffset >= 0 && p.endOffset >= 0 ? p.endOffset - p.startOffset : -1),
    cell: ({ getValue }) => formatOffset(getValue<number>()),
  },
  {
    accessorKey: 'sizeBytes',
    header: 'Size',
    meta: { align: 'right' },
    cell: ({ getValue }) => formatBytes(getValue<number>()),
  },
]

const consumerColumns: ColumnDef<GroupSummary>[] = [
  {
    accessorKey: 'id',
    header: 'Group',
    cell: ({ getValue }) => <span className="font-medium">{getValue<string>()}</span>,
  },
  { accessorKey: 'state', header: 'State', cell: ({ getValue }) => <StateBadge state={getValue<string>()} /> },
  { accessorKey: 'members', header: 'Members', meta: { align: 'right' } },
  {
    accessorKey: 'totalLag',
    header: 'Lag on topic',
    meta: { align: 'right' },
    cell: ({ getValue }) => formatNumber(getValue<number>()),
  },
]

export function TopicPage() {
  const { clusterId, topic } = useParams({ from: '/clusters/$clusterId/topics/$topic' })
  const navigate = useNavigate()
  const { data, isPending, error } = useTopic(clusterId, topic)
  const consumers = useTopicConsumers(clusterId, topic)
  const canMutate = useCanMutate(clusterId) && !!data && !data.internal
  const hasSchemaRegistry = useCluster(clusterId).data?.hasSchemaRegistry ?? false
  const [editing, setEditing] = useState<ConfigEntry | null>(null)

  return (
    <>
      <div>
        <Link
          to="/clusters/$clusterId/topics"
          params={{ clusterId }}
          className="inline-flex items-center gap-1 text-muted-foreground text-sm hover:text-foreground"
        >
          <ChevronLeftIcon className="size-4" /> Topics
        </Link>
      </div>
      <PageHeader
        title={topic}
        description={data && <span className="font-mono text-xs">topic id {data.id}</span>}
        actions={
          <>
            {data?.internal && <Badge variant="outline">internal</Badge>}
            {canMutate && (
              <ProduceDialog
                clusterId={clusterId}
                topic={topic}
                partitions={data.partitions}
                hasSchemaRegistry={hasSchemaRegistry}
              />
            )}
            {canMutate && <TopicActions clusterId={clusterId} topic={data} />}
          </>
        }
      />
      {error ? (
        <ErrorState error={error} />
      ) : isPending ? (
        <LoadingRows rows={6} />
      ) : (
        <>
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-5">
            <StatCard label="Partitions" value={data.partitions} />
            <StatCard label="Replication" value={data.replicationFactor} />
            <StatCard
              label="Under-replicated"
              value={data.underReplicatedPartitions}
              tone={data.underReplicatedPartitions > 0 ? 'warn' : 'ok'}
            />
            <StatCard label="Messages" value={formatNumber(data.messageCount)} />
            <StatCard label="Size" value={formatBytes(data.sizeBytes)} hint="leader replicas" />
          </div>
          <Tabs defaultValue="partitions">
            <TabsList>
              <TabsTrigger value="partitions">Partitions</TabsTrigger>
              <TabsTrigger value="consumers">
                Consumers {consumers.data && <Badge variant="secondary">{consumers.data.length}</Badge>}
              </TabsTrigger>
              <TabsTrigger value="throughput">Throughput</TabsTrigger>
              <TabsTrigger value="configs">Configs</TabsTrigger>
              <TabsTrigger value="messages">Messages</TabsTrigger>
              {hasSchemaRegistry && <TabsTrigger value="schema">Schema</TabsTrigger>}
            </TabsList>
            <TabsContent value="partitions" className="mt-4">
              <DataTable columns={partitionColumns} data={data.partitionDetails} getRowId={(p) => String(p.id)} />
            </TabsContent>
            <TabsContent value="consumers" className="mt-4">
              {consumers.error ? (
                <ErrorState error={consumers.error} />
              ) : (
                <DataTable
                  columns={consumerColumns}
                  data={consumers.data ?? []}
                  loading={consumers.isPending}
                  getRowId={(g) => g.id}
                  onRowClick={(g) =>
                    navigate({ to: '/clusters/$clusterId/groups/$group', params: { clusterId, group: g.id } })
                  }
                  emptyState={
                    <EmptyState title="No consumer groups" description="No group has offsets on this topic." />
                  }
                />
              )}
            </TabsContent>
            <TabsContent value="throughput" className="mt-4">
              <TopicThroughput clusterId={clusterId} topic={topic} />
            </TabsContent>
            <TabsContent value="configs" className="mt-4">
              <ConfigTable configs={data.configs ?? []} onEdit={canMutate ? setEditing : undefined} />
              <ConfigEditDialog clusterId={clusterId} topic={topic} entry={editing} onClose={() => setEditing(null)} />
            </TabsContent>
            {hasSchemaRegistry && (
              <TabsContent value="schema" className="mt-4">
                <TopicSchemas clusterId={clusterId} topic={topic} />
              </TabsContent>
            )}
            <TabsContent value="messages" className="mt-4">
              <MessagesTab
                clusterId={clusterId}
                topic={topic}
                partitions={data.partitions}
                hasSchemaRegistry={hasSchemaRegistry}
              />
            </TabsContent>
          </Tabs>
        </>
      )}
    </>
  )
}
