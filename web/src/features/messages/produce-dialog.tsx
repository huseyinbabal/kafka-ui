import { PlusIcon, SendIcon, Trash2Icon } from 'lucide-react'
import { type FormEvent, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { useSubjects } from '@/features/schemas/api'
import { formatNumber } from '@/lib/format'
import { type Format, type SchemaRef, useProduce } from './api'
import { partitionIds } from './messages-tab'

const formats: { value: Format; label: string }[] = [
  { value: 'string', label: 'String' },
  { value: 'json', label: 'JSON' },
  { value: 'base64', label: 'Base64' },
  { value: 'hex', label: 'Hex' },
]

function FormatSelect({
  id,
  value,
  onChange,
  withSchema,
}: {
  id: string
  value: Format
  onChange: (f: Format) => void
  withSchema: boolean
}) {
  const choices = withSchema ? [...formats, { value: 'schema' as Format, label: 'Schema Registry' }] : formats
  return (
    <Select value={value} onValueChange={(v) => onChange(v as Format)}>
      <SelectTrigger id={id} className="w-40">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {choices.map((f) => (
          <SelectItem key={f.value} value={f.value}>
            {f.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

type HeaderRow = { id: number; key: string; value: string }

/** Picks the subject (and protobuf message) used to serialize a key or value. */
function SchemaPicker({
  id,
  clusterId,
  value,
  onChange,
  preferred,
}: {
  id: string
  clusterId: string
  value: SchemaRef
  onChange: (r: SchemaRef) => void
  preferred: string
}) {
  const subjects = useSubjects(clusterId)
  const items = subjects.data ?? []
  const selected = items.find((s) => s.subject === value.subject)
  // Default to the TopicNameStrategy subject once the list loads.
  useEffect(() => {
    if (!value.subject && items.some((s) => s.subject === preferred)) onChange({ subject: preferred })
  }, [items, preferred, value.subject, onChange])

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Select value={value.subject} onValueChange={(subject) => onChange({ subject })}>
        <SelectTrigger id={id} aria-label="Schema subject" className="min-w-56 flex-1">
          <SelectValue placeholder={subjects.isPending ? 'Loading subjects…' : 'Choose a subject'} />
        </SelectTrigger>
        <SelectContent>
          {items.map((s) => (
            <SelectItem key={s.subject} value={s.subject}>
              {s.subject}{' '}
              <span className="text-muted-foreground">
                · {s.schemaType} v{s.latestVersion}
              </span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {selected?.schemaType === 'PROTOBUF' && (
        <Input
          aria-label="Protobuf message"
          placeholder="message (default: first)"
          className="w-56 font-mono"
          value={value.message ?? ''}
          onChange={(e) => onChange({ ...value, message: e.target.value || undefined })}
        />
      )}
      {subjects.error && <p className="text-destructive text-xs">{subjects.error.message}</p>}
    </div>
  )
}

export function ProduceDialog({
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
  const [keySchema, setKeySchema] = useState<SchemaRef>({ subject: '' })
  const [valueSchema, setValueSchema] = useState<SchemaRef>({ subject: '' })
  const [open, setOpen] = useState(false)
  const [partition, setPartition] = useState('auto')
  const [key, setKey] = useState('')
  const [nullKey, setNullKey] = useState(true)
  const [keyFormat, setKeyFormat] = useState<Format>('string')
  const [value, setValue] = useState('')
  const [valueFormat, setValueFormat] = useState<Format>('json')
  const [headers, setHeaders] = useState<HeaderRow[]>([])
  const produce = useProduce(clusterId, topic)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    produce.mutate(
      {
        partition: partition === 'auto' ? null : Number(partition),
        key: nullKey ? null : key,
        value,
        keyFormat,
        valueFormat,
        keySchema: keyFormat === 'schema' ? keySchema : undefined,
        valueSchema: valueFormat === 'schema' ? valueSchema : undefined,
        headers: headers.filter((h) => h.key !== '').map(({ key, value }) => ({ key, value })),
      },
      {
        onSuccess: (r) => {
          toast.success(`Produced to partition ${r.partition} at offset ${formatNumber(r.offset)}`)
        },
      },
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        setOpen(o)
        if (!o) produce.reset()
      }}
    >
      <DialogTrigger asChild>
        <Button size="sm">
          <SendIcon /> Produce
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-2xl">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle>Produce to {topic}</DialogTitle>
            <DialogDescription>
              The record is written with acks=all. Leave partition on auto to hash the key.
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 sm:grid-cols-[1fr_auto]">
            <div className="flex flex-col gap-2">
              <div className="flex items-center gap-2">
                <Label htmlFor="produce-key">Key</Label>
                <Checkbox
                  id="produce-null-key"
                  className="ml-auto"
                  checked={nullKey}
                  onCheckedChange={(c) => setNullKey(c === true)}
                />
                <Label htmlFor="produce-null-key" className="font-normal text-muted-foreground text-xs">
                  null key
                </Label>
              </div>
              <Input
                id="produce-key"
                value={key}
                disabled={nullKey}
                onChange={(e) => setKey(e.target.value)}
                className="font-mono"
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="produce-key-format">Key format</Label>
              <FormatSelect
                id="produce-key-format"
                value={keyFormat}
                onChange={setKeyFormat}
                withSchema={hasSchemaRegistry}
              />
            </div>
          </div>
          {keyFormat === 'schema' && (
            <SchemaPicker
              id="produce-key-schema"
              clusterId={clusterId}
              value={keySchema}
              onChange={setKeySchema}
              preferred={`${topic}-key`}
            />
          )}

          <div className="flex flex-col gap-2">
            <div className="flex items-end gap-2">
              <Label htmlFor="produce-value">Value</Label>
              <div className="ml-auto flex items-center gap-2">
                <Label htmlFor="produce-value-format" className="text-muted-foreground text-xs">
                  format
                </Label>
                <FormatSelect
                  id="produce-value-format"
                  value={valueFormat}
                  onChange={setValueFormat}
                  withSchema={hasSchemaRegistry}
                />
                <Label htmlFor="produce-partition" className="text-muted-foreground text-xs">
                  partition
                </Label>
                <Select value={partition} onValueChange={setPartition}>
                  <SelectTrigger id="produce-partition" className="w-24">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="auto">auto</SelectItem>
                    {partitionIds(partitions).map((p) => (
                      <SelectItem key={p} value={String(p)}>
                        {p}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            {valueFormat === 'schema' && (
              <SchemaPicker
                id="produce-value-schema"
                clusterId={clusterId}
                value={valueSchema}
                onChange={setValueSchema}
                preferred={`${topic}-value`}
              />
            )}
            <Textarea
              id="produce-value"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              rows={8}
              className="font-mono text-xs"
              placeholder={valueFormat === 'json' || valueFormat === 'schema' ? '{"hello": "kafka"}' : ''}
            />
          </div>

          <div className="flex flex-col gap-2">
            <div className="flex items-center">
              <Label>Headers</Label>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="ml-auto"
                onClick={() => setHeaders((h) => [...h, { id: Date.now(), key: '', value: '' }])}
              >
                <PlusIcon /> Add header
              </Button>
            </div>
            {headers.map((h) => (
              <div key={h.id} className="flex gap-2">
                <Input
                  aria-label="Header key"
                  placeholder="key"
                  value={h.key}
                  className="font-mono"
                  onChange={(e) =>
                    setHeaders((hs) => hs.map((x) => (x.id === h.id ? { ...x, key: e.target.value } : x)))
                  }
                />
                <Input
                  aria-label="Header value"
                  placeholder="value"
                  value={h.value}
                  className="font-mono"
                  onChange={(e) =>
                    setHeaders((hs) => hs.map((x) => (x.id === h.id ? { ...x, value: e.target.value } : x)))
                  }
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label="Remove header"
                  onClick={() => setHeaders((hs) => hs.filter((x) => x.id !== h.id))}
                >
                  <Trash2Icon />
                </Button>
              </div>
            ))}
          </div>

          {produce.error && <p className="text-destructive text-sm">{produce.error.message}</p>}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              Close
            </Button>
            <Button type="submit" disabled={produce.isPending}>
              <SendIcon /> {produce.isPending ? 'Producing…' : 'Produce'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
