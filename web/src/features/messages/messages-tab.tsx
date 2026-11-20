import type { ColumnDef } from '@tanstack/react-table'
import { ChevronDownIcon, LoaderCircleIcon, PlayIcon, RadioIcon, SquareIcon } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { DataTable } from '@/components/data-table'
import { EmptyState, ErrorState } from '@/components/states'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { formatBytes, formatNumber } from '@/lib/format'
import {
  type BrowseParams,
  type FilterField,
  type Format,
  type Message,
  type SeekMode,
  type StopReason,
  useMessageStream,
} from './api'
import { MessageSheet } from './message-sheet'
import { payloadPreview } from './payload-view'

const modes: { value: SeekMode; label: string }[] = [
  { value: 'newest', label: 'Newest' },
  { value: 'oldest', label: 'Oldest' },
  { value: 'offset', label: 'From offset' },
  { value: 'timestamp', label: 'From time' },
  { value: 'live', label: 'Live tail' },
]

const formats: { value: Format; label: string }[] = [
  { value: 'auto', label: 'Auto' },
  { value: 'string', label: 'String' },
  { value: 'json', label: 'JSON' },
  { value: 'base64', label: 'Base64' },
  { value: 'hex', label: 'Hex' },
]

const reasons: Record<StopReason, string> = {
  end: 'reached the end',
  limit: 'hit the result limit',
  scanLimit: 'hit the 100k scan limit — narrow the range or filter',
  timeout: 'hit the time limit',
  cancelled: 'stopped',
}

const columns: ColumnDef<Message>[] = [
  { accessorKey: 'partition', header: 'Part.', meta: { align: 'right' } },
  {
    accessorKey: 'offset',
    header: 'Offset',
    meta: { align: 'right' },
    cell: ({ getValue }) => <span className="tabular-nums">{formatNumber(getValue<number>())}</span>,
  },
  {
    accessorKey: 'timestamp',
    header: 'Timestamp',
    cell: ({ getValue }) => (
      <span className="whitespace-nowrap text-muted-foreground tabular-nums">
        {new Date(getValue<number>()).toLocaleString()}
      </span>
    ),
  },
  {
    id: 'key',
    header: 'Key',
    accessorFn: (m) => m.key?.text ?? '',
    cell: ({ row }) => (
      <span className="block max-w-48 truncate font-mono text-xs" title={row.original.key?.text}>
        {row.original.key ? payloadPreview(row.original.key, 60) : <span className="text-muted-foreground">null</span>}
      </span>
    ),
  },
  {
    id: 'value',
    header: 'Value',
    accessorFn: (m) => m.value?.text ?? '',
    enableSorting: false,
    cell: ({ row }) => (
      <span className="flex items-center gap-2">
        {row.original.value && row.original.value.format !== 'string' && (
          <Badge
            variant={row.original.value.error ? 'destructive' : 'secondary'}
            className="shrink-0"
            title={row.original.value.error}
          >
            {row.original.value.format}
          </Badge>
        )}
        <span className="block max-w-[min(36rem,40vw)] truncate font-mono text-xs">
          {row.original.value ? (
            payloadPreview(row.original.value)
          ) : (
            <span className="text-muted-foreground">null</span>
          )}
        </span>
      </span>
    ),
  },
  {
    id: 'headers',
    header: 'Hdrs',
    meta: { align: 'right' },
    accessorFn: (m) => m.headers.length,
  },
  {
    accessorKey: 'size',
    header: 'Size',
    meta: { align: 'right' },
    cell: ({ getValue }) => formatBytes(getValue<number>()),
  },
]

export const partitionIds = (n: number) => Array.from({ length: n }, (_, i) => i)

function PartitionPicker({
  count,
  value,
  onChange,
}: {
  count: number
  value: number[]
  onChange: (v: number[]) => void
}) {
  const label = value.length === 0 ? 'All partitions' : `Partitions ${[...value].sort((a, b) => a - b).join(', ')}`
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" className="max-w-56 justify-between font-normal">
          <span className="truncate">{label}</span>
          <ChevronDownIcon className="opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-56 p-2" align="start">
        <div className="flex items-center justify-between px-1 pb-2 text-muted-foreground text-xs">
          <span>{value.length === 0 ? 'All selected' : `${value.length} selected`}</span>
          <button type="button" className="hover:text-foreground" onClick={() => onChange([])}>
            All
          </button>
        </div>
        <div className="grid max-h-64 grid-cols-3 gap-1 overflow-y-auto">
          {partitionIds(count).map((p) => (
            <div key={p} className="flex items-center gap-1.5 rounded px-1 py-1 text-sm hover:bg-accent">
              <Checkbox
                id={`msg-part-${p}`}
                checked={value.includes(p)}
                onCheckedChange={(c) => onChange(c === true ? [...value, p] : value.filter((x) => x !== p))}
              />
              <Label htmlFor={`msg-part-${p}`} className="flex-1 font-normal">
                {p}
              </Label>
            </div>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  )
}

function toLocalInput(ms: number) {
  const d = new Date(ms - new Date(ms).getTimezoneOffset() * 60_000)
  return d.toISOString().slice(0, 16)
}

export function MessagesTab({
  clusterId,
  topic,
  partitions,
  hasSchemaRegistry,
}: {
  clusterId: string
  topic: string
  partitions: number
  hasSchemaRegistry: boolean
}) {
  const formatChoices = hasSchemaRegistry
    ? [...formats, { value: 'schema' as Format, label: 'Schema Registry' }]
    : formats
  const [params, setParams] = useState<BrowseParams>({
    mode: 'newest',
    limit: 100,
    partitions: [],
    offset: 0,
    timestamp: Date.now() - 60 * 60_000,
    q: '',
    qField: 'any',
    regex: false,
    keyFormat: 'auto',
    valueFormat: 'auto',
  })
  const set = <K extends keyof BrowseParams>(k: K, v: BrowseParams[K]) => setParams((p) => ({ ...p, [k]: v }))
  const stream = useMessageStream(clusterId, topic)
  const [selected, setSelected] = useState<Message | null>(null)
  // Formats used for the rows on screen, so the detail view matches them.
  const [shownFormats, setShownFormats] = useState({ key: params.keyFormat, value: params.valueFormat })
  const running = stream.status === 'running'

  const run = (e?: FormEvent) => {
    e?.preventDefault()
    if (running) {
      stream.stop()
      return
    }
    setShownFormats({ key: params.keyFormat, value: params.valueFormat })
    stream.start(params)
  }

  return (
    <div className="flex flex-col gap-4">
      <form onSubmit={run} className="flex flex-col gap-3 rounded-lg border p-3">
        <div className="flex flex-wrap items-end gap-2">
          <div className="flex flex-col gap-1">
            <Label htmlFor="msg-mode" className="text-muted-foreground text-xs">
              Seek
            </Label>
            <Select value={params.mode} onValueChange={(v) => set('mode', v as SeekMode)}>
              <SelectTrigger id="msg-mode" className="w-36">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {modes.map((m) => (
                  <SelectItem key={m.value} value={m.value}>
                    {m.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {params.mode === 'offset' && (
            <div className="flex flex-col gap-1">
              <Label htmlFor="msg-offset" className="text-muted-foreground text-xs">
                Offset
              </Label>
              <Input
                id="msg-offset"
                type="number"
                min={0}
                className="w-32"
                value={params.offset ?? 0}
                onChange={(e) => set('offset', Math.max(0, Number(e.target.value)))}
              />
            </div>
          )}
          {params.mode === 'timestamp' && (
            <div className="flex flex-col gap-1">
              <Label htmlFor="msg-ts" className="text-muted-foreground text-xs">
                From (local time)
              </Label>
              <Input
                id="msg-ts"
                type="datetime-local"
                className="w-52"
                value={toLocalInput(params.timestamp ?? Date.now())}
                onChange={(e) => {
                  const ms = new Date(e.target.value).getTime()
                  if (Number.isFinite(ms)) set('timestamp', ms)
                }}
              />
            </div>
          )}
          <div className="flex flex-col gap-1">
            <Label htmlFor="msg-limit" className="text-muted-foreground text-xs">
              Limit
            </Label>
            <Select value={String(params.limit)} onValueChange={(v) => set('limit', Number(v))}>
              <SelectTrigger id="msg-limit" className="w-24">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[20, 50, 100, 200, 500].map((n) => (
                  <SelectItem key={n} value={String(n)}>
                    {n}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-muted-foreground text-xs">Partitions</span>
            <PartitionPicker count={partitions} value={params.partitions} onChange={(v) => set('partitions', v)} />
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor="msg-kf" className="text-muted-foreground text-xs">
              Key as
            </Label>
            <Select value={params.keyFormat} onValueChange={(v) => set('keyFormat', v as Format)}>
              <SelectTrigger id="msg-kf" className="w-28">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {formatChoices.map((f) => (
                  <SelectItem key={f.value} value={f.value}>
                    {f.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor="msg-vf" className="text-muted-foreground text-xs">
              Value as
            </Label>
            <Select value={params.valueFormat} onValueChange={(v) => set('valueFormat', v as Format)}>
              <SelectTrigger id="msg-vf" className="w-28">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {formatChoices.map((f) => (
                  <SelectItem key={f.value} value={f.value}>
                    {f.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Input
            aria-label="Filter"
            placeholder={params.regex ? 'RE2 regex, e.g. ^order-\\d+$' : 'Contains (case-insensitive)…'}
            className="min-w-64 flex-1 font-mono"
            value={params.q}
            onChange={(e) => set('q', e.target.value)}
          />
          <Select value={params.qField} onValueChange={(v) => set('qField', v as FilterField)}>
            <SelectTrigger aria-label="Filter field" className="w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="any">Anywhere</SelectItem>
              <SelectItem value="key">Key</SelectItem>
              <SelectItem value="value">Value</SelectItem>
              <SelectItem value="header">Headers</SelectItem>
            </SelectContent>
          </Select>
          <div className="flex items-center gap-2 px-1">
            <Switch id="msg-regex" checked={params.regex} onCheckedChange={(c) => set('regex', c)} />
            <Label htmlFor="msg-regex" className="font-normal">
              Regex
            </Label>
          </div>
          <Button type="submit" variant={running ? 'destructive' : 'default'} className="w-28">
            {running ? (
              <>
                <SquareIcon /> Stop
              </>
            ) : params.mode === 'live' ? (
              <>
                <RadioIcon /> Tail
              </>
            ) : (
              <>
                <PlayIcon /> Search
              </>
            )}
          </Button>
        </div>
      </form>

      {stream.status !== 'idle' && (
        <div className="flex items-center gap-2 text-muted-foreground text-sm" aria-live="polite">
          {running && <LoaderCircleIcon className="size-4 animate-spin" />}
          <span className="tabular-nums">
            Scanned {formatNumber(stream.progress.scanned)} · matched {formatNumber(stream.progress.matched)}
            {running && params.mode === 'live' && ' · waiting for new messages'}
          </span>
          {stream.status === 'done' && stream.result && <span>· {reasons[stream.result.reason]}</span>}
        </div>
      )}

      {stream.status === 'error' && stream.error && <ErrorState error={stream.error} title="Scan failed" />}

      {stream.status === 'idle' ? (
        <EmptyState
          title="Search messages"
          description="Pick a seek mode and press Search. Filters run on the server and stream results as they are found."
        />
      ) : (
        <DataTable
          columns={columns}
          data={stream.messages}
          getRowId={(m) => `${m.partition}-${m.offset}`}
          onRowClick={setSelected}
          searchPlaceholder={stream.messages.length > 0 ? 'Filter loaded rows…' : undefined}
          emptyState={
            <EmptyState
              title={running ? 'Searching…' : 'No messages'}
              description={running ? undefined : 'Nothing matched in the scanned range.'}
            />
          }
        />
      )}

      <MessageSheet
        clusterId={clusterId}
        topic={topic}
        message={selected}
        keyFormat={shownFormats.key}
        valueFormat={shownFormats.value}
        onClose={() => setSelected(null)}
      />
    </div>
  )
}
