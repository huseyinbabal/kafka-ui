import {
  type ColumnDef,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
  type SortingState,
  useReactTable,
} from '@tanstack/react-table'
import { ArrowDownIcon, ArrowUpDownIcon, ArrowUpIcon, SearchIcon } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { cn } from '@/lib/utils'
import { EmptyState, LoadingRows } from './states'

type Props<T> = {
  // biome-ignore lint/suspicious/noExplicitAny: TanStack column value types are heterogeneous
  columns: ColumnDef<T, any>[]
  data: T[]
  loading?: boolean
  searchPlaceholder?: string
  toolbar?: ReactNode
  emptyState?: ReactNode
  onRowClick?: (row: T) => void
  initialSorting?: SortingState
  getRowId?: (row: T) => string
}

export function DataTable<T>({
  columns,
  data,
  loading,
  searchPlaceholder,
  toolbar,
  emptyState,
  onRowClick,
  initialSorting = [],
  getRowId,
}: Props<T>) {
  const [sorting, setSorting] = useState<SortingState>(initialSorting)
  const [globalFilter, setGlobalFilter] = useState('')
  const table = useReactTable({
    data,
    columns,
    state: { sorting, globalFilter },
    onSortingChange: setSorting,
    onGlobalFilterChange: setGlobalFilter,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getRowId,
  })
  const rows = table.getRowModel().rows

  return (
    <div className="grid gap-3">
      {(searchPlaceholder || toolbar) && (
        <div className="flex flex-wrap items-center gap-2">
          {searchPlaceholder && (
            <div className="relative w-full max-w-xs">
              <SearchIcon className="-translate-y-1/2 absolute top-1/2 left-2.5 size-4 text-muted-foreground" />
              <Input
                className="pl-8"
                placeholder={searchPlaceholder}
                value={globalFilter}
                onChange={(e) => setGlobalFilter(e.target.value)}
              />
            </div>
          )}
          <div className="ml-auto flex items-center gap-3">{toolbar}</div>
        </div>
      )}
      {loading ? (
        <LoadingRows />
      ) : (
        <div className="overflow-hidden rounded-lg border">
          <Table>
            <TableHeader className="bg-muted/50">
              {table.getHeaderGroups().map((hg) => (
                <TableRow key={hg.id}>
                  {hg.headers.map((h) => {
                    const sorted = h.column.getIsSorted()
                    const canSort = h.column.getCanSort()
                    const align = (h.column.columnDef.meta as { align?: string } | undefined)?.align
                    return (
                      <TableHead key={h.id} className={cn(align === 'right' && 'text-right')}>
                        {h.isPlaceholder ? null : canSort ? (
                          <button
                            type="button"
                            className={cn(
                              'inline-flex items-center gap-1 hover:text-foreground',
                              align === 'right' && 'flex-row-reverse',
                            )}
                            onClick={h.column.getToggleSortingHandler()}
                          >
                            {flexRender(h.column.columnDef.header, h.getContext())}
                            {sorted === 'asc' ? (
                              <ArrowUpIcon className="size-3" />
                            ) : sorted === 'desc' ? (
                              <ArrowDownIcon className="size-3" />
                            ) : (
                              <ArrowUpDownIcon className="size-3 opacity-40" />
                            )}
                          </button>
                        ) : (
                          flexRender(h.column.columnDef.header, h.getContext())
                        )}
                      </TableHead>
                    )
                  })}
                </TableRow>
              ))}
            </TableHeader>
            <TableBody>
              {rows.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={columns.length}>{emptyState ?? <EmptyState title="No results" />}</TableCell>
                </TableRow>
              ) : (
                rows.map((row) => (
                  <TableRow
                    key={row.id}
                    className={cn(onRowClick && 'cursor-pointer')}
                    onClick={onRowClick ? () => onRowClick(row.original) : undefined}
                  >
                    {row.getVisibleCells().map((cell) => {
                      const align = (cell.column.columnDef.meta as { align?: string } | undefined)?.align
                      return (
                        <TableCell key={cell.id} className={cn(align === 'right' && 'text-right tabular-nums')}>
                          {flexRender(cell.column.columnDef.cell, cell.getContext())}
                        </TableCell>
                      )
                    })}
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
