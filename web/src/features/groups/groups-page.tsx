import { useNavigate, useParams } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { DataTable } from '@/components/data-table'
import { PageHeader } from '@/components/page'
import { EmptyState, ErrorState } from '@/components/states'
import { Badge } from '@/components/ui/badge'
import type { GroupSummary } from '@/lib/api'
import { formatNumber } from '@/lib/format'
import { useGroups } from './api'
import { StateBadge } from './state-badge'

const columns: ColumnDef<GroupSummary>[] = [
  {
    accessorKey: 'id',
    header: 'Group',
    cell: ({ getValue }) => <span className="font-medium">{getValue<string>()}</span>,
  },
  { accessorKey: 'state', header: 'State', cell: ({ getValue }) => <StateBadge state={getValue<string>()} /> },
  { accessorKey: 'members', header: 'Members', meta: { align: 'right' } },
  {
    id: 'topics',
    header: 'Topics',
    accessorFn: (g) => g.topics.join(' '),
    enableSorting: false,
    cell: ({ row }) => (
      <span className="flex flex-wrap gap-1">
        {row.original.topics.map((t) => (
          <Badge key={t} variant="secondary" className="font-normal">
            {t}
          </Badge>
        ))}
      </span>
    ),
  },
  { accessorKey: 'coordinator', header: 'Coordinator', meta: { align: 'right' } },
  {
    accessorKey: 'totalLag',
    header: 'Total lag',
    meta: { align: 'right' },
    cell: ({ getValue }) => {
      const v = getValue<number>()
      return <span className={v > 0 ? 'font-medium text-amber-500' : 'text-muted-foreground'}>{formatNumber(v)}</span>
    },
  },
]

export function GroupsPage() {
  const { clusterId } = useParams({ from: '/clusters/$clusterId/groups/' })
  const navigate = useNavigate()
  const { data = [], isPending, error } = useGroups(clusterId)

  return (
    <>
      <PageHeader title="Consumer groups" description={isPending ? undefined : `${data.length} groups`} />
      {error ? (
        <ErrorState error={error} />
      ) : (
        <DataTable
          columns={columns}
          data={data}
          loading={isPending}
          getRowId={(g) => g.id}
          searchPlaceholder="Search groups or topics…"
          initialSorting={[{ id: 'totalLag', desc: true }]}
          onRowClick={(g) => navigate({ to: '/clusters/$clusterId/groups/$group', params: { clusterId, group: g.id } })}
          emptyState={<EmptyState title="No consumer groups" />}
        />
      )}
    </>
  )
}
