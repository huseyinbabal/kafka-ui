import { Link, useParams } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { ChevronLeftIcon } from 'lucide-react'
import { DataTable } from '@/components/data-table'
import { PageHeader, StatCard } from '@/components/page'
import { EmptyState, ErrorState, LoadingRows } from '@/components/states'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { GroupLagTrend } from '@/features/analytics/series-cards'
import { useCanMutate } from '@/features/auth/api'
import type { GroupMember, PartitionLag } from '@/lib/api'
import { formatNumber, formatOffset } from '@/lib/format'
import { useGroup } from './api'
import { GroupActions } from './group-actions'
import { StateBadge } from './state-badge'

const memberColumns: ColumnDef<GroupMember>[] = [
  {
    accessorKey: 'clientId',
    header: 'Client ID',
    cell: ({ getValue }) => <span className="font-medium">{getValue<string>()}</span>,
  },
  {
    accessorKey: 'clientHost',
    header: 'Host',
    cell: ({ getValue }) => <span className="font-mono text-xs">{getValue<string>()}</span>,
  },
  {
    id: 'assigned',
    header: 'Assigned partitions',
    enableSorting: false,
    accessorFn: (m) => m.assigned.map((a) => a.topic).join(' '),
    cell: ({ row }) => (
      <span className="flex flex-wrap gap-1">
        {row.original.assigned.map((a) => (
          <Badge key={a.topic} variant="secondary" className="font-normal">
            {a.topic}: {a.partitions.join(', ')}
          </Badge>
        ))}
      </span>
    ),
  },
  {
    accessorKey: 'memberId',
    header: 'Member ID',
    cell: ({ getValue }) => (
      <span className="block max-w-64 truncate font-mono text-muted-foreground text-xs">{getValue<string>()}</span>
    ),
  },
]

const lagColumns: ColumnDef<PartitionLag>[] = [
  {
    accessorKey: 'topic',
    header: 'Topic',
    cell: ({ getValue }) => <span className="font-medium">{getValue<string>()}</span>,
  },
  { accessorKey: 'partition', header: 'Partition', meta: { align: 'right' } },
  {
    accessorKey: 'committed',
    header: 'Committed',
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
    accessorKey: 'lag',
    header: 'Lag',
    meta: { align: 'right' },
    cell: ({ row }) => {
      const p = row.original
      if (p.error) return <span className="text-red-500 text-xs">{p.error}</span>
      if (p.committed < 0)
        return (
          <span className="text-muted-foreground" title="no committed offset">
            —
          </span>
        )
      return (
        <span className={p.lag > 0 ? 'font-medium text-amber-500' : 'text-muted-foreground'}>
          {formatNumber(p.lag)}
        </span>
      )
    },
  },
  {
    accessorKey: 'clientId',
    header: 'Consumer',
    cell: ({ row }) =>
      row.original.clientId ? (
        <span className="text-xs">
          {row.original.clientId} <span className="font-mono text-muted-foreground">{row.original.clientHost}</span>
        </span>
      ) : (
        <span className="text-muted-foreground text-xs">unassigned</span>
      ),
  },
]

export function GroupPage() {
  const { clusterId, group } = useParams({ from: '/clusters/$clusterId/groups/$group' })
  const { data, isPending, error } = useGroup(clusterId, group)
  const canMutate = useCanMutate(clusterId)

  return (
    <>
      <div>
        <Link
          to="/clusters/$clusterId/groups"
          params={{ clusterId }}
          className="inline-flex items-center gap-1 text-muted-foreground text-sm hover:text-foreground"
        >
          <ChevronLeftIcon className="size-4" /> Consumer groups
        </Link>
      </div>
      <PageHeader
        title={group}
        description={data && `${data.protocolType || 'unknown'} · ${data.protocol || 'no assignor'}`}
        actions={
          data && (
            <>
              <StateBadge state={data.state} />
              {canMutate && <GroupActions clusterId={clusterId} group={data} />}
            </>
          )
        }
      />
      {error ? (
        <ErrorState error={error} />
      ) : isPending ? (
        <LoadingRows rows={6} />
      ) : (
        <>
          {data.error && <ErrorState title="Partial data" error={new Error(data.error)} />}
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
            <StatCard label="Members" value={data.members} />
            <StatCard label="Topics" value={data.topics.length} />
            <StatCard label="Total lag" value={formatNumber(data.totalLag)} tone={data.totalLag > 0 ? 'warn' : 'ok'} />
            <StatCard label="Coordinator" value={`Broker ${data.coordinator}`} />
          </div>
          <GroupLagTrend clusterId={clusterId} group={group} />
          <Card>
            <CardHeader>
              <CardTitle>Members</CardTitle>
            </CardHeader>
            <CardContent>
              <DataTable
                columns={memberColumns}
                data={data.memberList}
                getRowId={(m) => m.memberId}
                emptyState={<EmptyState title="No active members" description="The group is empty." />}
              />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Partition lag</CardTitle>
            </CardHeader>
            <CardContent>
              <DataTable
                columns={lagColumns}
                data={data.partitions}
                getRowId={(p) => `${p.topic}-${p.partition}`}
                initialSorting={[{ id: 'lag', desc: true }]}
              />
            </CardContent>
          </Card>
        </>
      )}
    </>
  )
}
