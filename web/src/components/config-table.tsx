import type { ColumnDef } from '@tanstack/react-table'
import { LockIcon, PencilIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import type { ConfigEntry } from '@/lib/api'
import { DataTable } from './data-table'

const columns: ColumnDef<ConfigEntry>[] = [
  {
    accessorKey: 'name',
    header: 'Name',
    cell: ({ row }) => (
      <span
        className={row.original.isDefault ? 'font-mono text-muted-foreground text-xs' : 'font-mono font-medium text-xs'}
      >
        {row.original.name}
      </span>
    ),
  },
  {
    accessorKey: 'value',
    header: 'Value',
    cell: ({ row }) =>
      row.original.sensitive ? (
        <LockIcon className="size-3.5 text-muted-foreground" />
      ) : (
        <span className="break-all font-mono text-xs">{row.original.value ?? '—'}</span>
      ),
  },
  {
    accessorKey: 'source',
    header: 'Source',
    cell: ({ row }) =>
      row.original.isDefault ? (
        <span className="text-muted-foreground text-xs">default</span>
      ) : (
        <Badge variant="secondary" className="font-mono text-[10px]">
          {row.original.source}
        </Badge>
      ),
  },
]

type Props = {
  configs: ConfigEntry[]
  loading?: boolean
  /** When set, each non-sensitive row gets an edit button. */
  onEdit?: (c: ConfigEntry) => void
}

export function ConfigTable({ configs, loading, onEdit }: Props) {
  const [onlyOverrides, setOnlyOverrides] = useState(false)
  const cols = useMemo<ColumnDef<ConfigEntry>[]>(
    () =>
      onEdit
        ? [
            ...columns,
            {
              id: 'edit',
              header: '',
              enableSorting: false,
              meta: { align: 'right' },
              cell: ({ row }) =>
                row.original.sensitive ? null : (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`Edit ${row.original.name}`}
                    onClick={() => onEdit(row.original)}
                  >
                    <PencilIcon />
                  </Button>
                ),
            },
          ]
        : columns,
    [onEdit],
  )
  const data = useMemo(() => (onlyOverrides ? configs.filter((c) => !c.isDefault) : configs), [configs, onlyOverrides])
  return (
    <DataTable
      columns={cols}
      data={data}
      loading={loading}
      getRowId={(c) => c.name}
      searchPlaceholder="Search configs…"
      toolbar={
        <div className="flex items-center gap-2">
          <Switch id="overrides" checked={onlyOverrides} onCheckedChange={setOnlyOverrides} />
          <Label htmlFor="overrides" className="text-sm">
            Overrides only
          </Label>
        </div>
      }
    />
  )
}
