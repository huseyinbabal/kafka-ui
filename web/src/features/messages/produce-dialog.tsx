import { PlusIcon, SendIcon, Trash2Icon } from 'lucide-react'
import { type FormEvent, useState } from 'react'
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
import { formatNumber } from '@/lib/format'
import { type Format, useProduce } from './api'
import { partitionIds } from './messages-tab'

const formats: { value: Format; label: string }[] = [
  { value: 'string', label: 'String' },
  { value: 'json', label: 'JSON' },
  { value: 'base64', label: 'Base64' },
  { value: 'hex', label: 'Hex' },
]

function FormatSelect({ id, value, onChange }: { id: string; value: Format; onChange: (f: Format) => void }) {
  return (
    <Select value={value} onValueChange={(v) => onChange(v as Format)}>
      <SelectTrigger id={id} className="w-28">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {formats.map((f) => (
          <SelectItem key={f.value} value={f.value}>
            {f.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

type HeaderRow = { id: number; key: string; value: string }

export function ProduceDialog({
  clusterId,
  topic,
  partitions,
}: {
  clusterId: string
  topic: string
  partitions: number
}) {
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
              <FormatSelect id="produce-key-format" value={keyFormat} onChange={setKeyFormat} />
            </div>
          </div>

          <div className="flex flex-col gap-2">
            <div className="flex items-end gap-2">
              <Label htmlFor="produce-value">Value</Label>
              <div className="ml-auto flex items-center gap-2">
                <Label htmlFor="produce-value-format" className="text-muted-foreground text-xs">
                  format
                </Label>
                <FormatSelect id="produce-value-format" value={valueFormat} onChange={setValueFormat} />
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
            <Textarea
              id="produce-value"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              rows={8}
              className="font-mono text-xs"
              placeholder={valueFormat === 'json' ? '{"hello": "kafka"}' : ''}
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
