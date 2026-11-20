import { useNavigate, useParams } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { DataTable } from '@/components/data-table'
import { PageHeader } from '@/components/page'
import { EmptyState, ErrorState } from '@/components/states'
import { Badge } from '@/components/ui/badge'
import { useCanMutate } from '@/features/auth/api'
import { type SubjectSummary, useSubjects } from './api'
import { RegisterSchemaDialog } from './register-dialog'

const typeLabel = { AVRO: 'Avro', PROTOBUF: 'Protobuf', JSON: 'JSON Schema' } as const

export const SchemaTypeBadge = ({ type }: { type: SubjectSummary['schemaType'] }) => (
  <Badge variant="secondary">{typeLabel[type] ?? type}</Badge>
)

const columns: ColumnDef<SubjectSummary>[] = [
  {
    accessorKey: 'subject',
    header: 'Subject',
    cell: ({ getValue }) => <span className="font-medium font-mono text-sm">{getValue<string>()}</span>,
  },
  { accessorKey: 'schemaType', header: 'Type', cell: ({ row }) => <SchemaTypeBadge type={row.original.schemaType} /> },
  { accessorKey: 'latestVersion', header: 'Latest version', meta: { align: 'right' } },
  { accessorKey: 'versions', header: 'Versions', meta: { align: 'right' } },
  { accessorKey: 'schemaId', header: 'Schema ID', meta: { align: 'right' } },
]

export function SchemasPage() {
  const { clusterId } = useParams({ from: '/clusters/$clusterId/schemas/' })
  const navigate = useNavigate()
  const { data = [], isPending, error } = useSubjects(clusterId)
  const canMutate = useCanMutate(clusterId)
  const open = (subject: string) =>
    navigate({ to: '/clusters/$clusterId/schemas/$subject', params: { clusterId, subject } })

  return (
    <>
      <PageHeader
        title="Schema Registry"
        description={isPending ? undefined : `${data.length} subjects`}
        actions={canMutate && <RegisterSchemaDialog clusterId={clusterId} onRegistered={open} />}
      />
      {error ? (
        <ErrorState error={error} />
      ) : (
        <DataTable
          columns={columns}
          data={data}
          loading={isPending}
          getRowId={(s) => s.subject}
          searchPlaceholder="Search subjects…"
          initialSorting={[{ id: 'subject', desc: false }]}
          onRowClick={(s) => open(s.subject)}
          emptyState={
            <EmptyState
              title="No subjects yet"
              description="Subjects appear when a producer registers a schema or you register one here."
            />
          }
        />
      )}
    </>
  )
}
