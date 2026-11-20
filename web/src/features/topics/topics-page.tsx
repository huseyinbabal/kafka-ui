import { useNavigate, useParams } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo, useState } from 'react'
import { DataTable } from '@/components/data-table'
import { PageHeader } from '@/components/page'
import { EmptyState, ErrorState } from '@/components/states'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useCanMutate } from '@/features/auth/api'
import type { TopicSummary } from '@/lib/api'
import { formatBytes, formatNumber } from '@/lib/format'
import { useTopics } from './api'
import { CreateTopicDialog } from './create-topic-dialog'

const columns: ColumnDef<TopicSummary>[] = [
  {
    accessorKey: 'name',
    header: 'Name',
    cell: ({ row }) => (
      <span className="inline-flex items-center gap-2 font-medium">
        {row.original.name}
        {row.original.internal && (
          <Badge variant="outline" className="text-[10px]">
            internal
          </Badge>
        )}
      </span>
    ),
  },
  { accessorKey: 'partitions', header: 'Partitions', meta: { align: 'right' } },
  { accessorKey: 'replicationFactor', header: 'RF', meta: { align: 'right' } },
  {
    accessorKey: 'underReplicatedPartitions',
    header: 'URP',
    meta: { align: 'right' },
    cell: ({ getValue }) => {
      const v = getValue<number>()
      return v > 0 ? (
        <span className="font-medium text-amber-500">{v}</span>
      ) : (
        <span className="text-muted-foreground">0</span>
      )
    },
  },
  {
    accessorKey: 'messageCount',
    header: 'Messages',
    meta: { align: 'right' },
    cell: ({ getValue }) => formatNumber(getValue<number>()),
  },
  {
    accessorKey: 'sizeBytes',
    header: 'Size',
    meta: { align: 'right' },
    cell: ({ getValue }) => formatBytes(getValue<number>()),
  },
]

export function TopicsPage() {
  const { clusterId } = useParams({ from: '/clusters/$clusterId/topics/' })
  const navigate = useNavigate()
  const { data = [], isPending, error } = useTopics(clusterId)
  const [showInternal, setShowInternal] = useState(false)
  const canMutate = useCanMutate(clusterId)
  const rows = useMemo(() => (showInternal ? data : data.filter((t) => !t.internal)), [data, showInternal])

  return (
    <>
      <PageHeader
        title="Topics"
        description={isPending ? undefined : `${rows.length} topics`}
        actions={canMutate && <CreateTopicDialog clusterId={clusterId} />}
      />
      {error ? (
        <ErrorState error={error} />
      ) : (
        <DataTable
          columns={columns}
          data={rows}
          loading={isPending}
          getRowId={(t) => t.name}
          searchPlaceholder="Search topics…"
          initialSorting={[{ id: 'name', desc: false }]}
          onRowClick={(t) =>
            navigate({ to: '/clusters/$clusterId/topics/$topic', params: { clusterId, topic: t.name } })
          }
          emptyState={<EmptyState title="No topics" description="This cluster has no topics yet." />}
          toolbar={
            <div className="flex items-center gap-2">
              <Switch id="internal" checked={showInternal} onCheckedChange={setShowInternal} />
              <Label htmlFor="internal" className="text-sm">
                Show internal
              </Label>
            </div>
          }
        />
      )}
    </>
  )
}
