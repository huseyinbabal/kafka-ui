import { useNavigate } from '@tanstack/react-router'
import { PlusIcon } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
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
import { useOverview } from '@/features/clusters/api'
import { useCreateTopic } from './admin-api'

const retentions = [
  { value: 'default', label: 'Broker default' },
  { value: '3600000', label: '1 hour' },
  { value: '86400000', label: '1 day' },
  { value: '604800000', label: '7 days' },
  { value: '2592000000', label: '30 days' },
  { value: '-1', label: 'Forever' },
]

/** Parses "key=value" lines; blank lines and # comments are ignored. */
export function parseConfigLines(text: string): { configs: Record<string, string>; error?: string } {
  const configs: Record<string, string> = {}
  for (const [i, raw] of text.split('\n').entries()) {
    const line = raw.trim()
    if (!line || line.startsWith('#')) continue
    const eq = line.indexOf('=')
    if (eq <= 0) return { configs, error: `line ${i + 1}: expected key=value` }
    configs[line.slice(0, eq).trim()] = line.slice(eq + 1).trim()
  }
  return { configs }
}

const nameRe = /^[a-zA-Z0-9._-]{1,249}$/

export function CreateTopicDialog({ clusterId }: { clusterId: string }) {
  const navigate = useNavigate()
  const overview = useOverview(clusterId)
  const brokers = overview.data?.brokers.length ?? 0
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [partitions, setPartitions] = useState(3)
  const [rf, setRf] = useState('default')
  const [cleanup, setCleanup] = useState('default')
  const [retention, setRetention] = useState('default')
  const [extra, setExtra] = useState('')
  const create = useCreateTopic(clusterId)

  const parsed = parseConfigLines(extra)
  const nameError =
    name && (!nameRe.test(name) || name === '.' || name === '..')
      ? 'Use 1-249 characters: letters, digits, ".", "_" or "-".'
      : name.startsWith('__')
        ? 'Names starting with "__" are reserved for internal topics.'
        : undefined

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const configs = { ...parsed.configs }
    if (cleanup !== 'default') configs['cleanup.policy'] = cleanup
    if (retention !== 'default') configs['retention.ms'] = retention
    create.mutate(
      { name, partitions, replicationFactor: rf === 'default' ? -1 : Number(rf), configs },
      {
        onSuccess: (t) => {
          toast.success(`Created ${t.name} (${t.partitions} partitions, RF ${t.replicationFactor})`)
          setOpen(false)
          navigate({ to: '/clusters/$clusterId/topics/$topic', params: { clusterId, topic: t.name } })
        },
      },
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        setOpen(o)
        if (!o) create.reset()
      }}
    >
      <DialogTrigger asChild>
        <Button size="sm">
          <PlusIcon /> Create topic
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle>Create topic</DialogTitle>
            <DialogDescription>Partitions can be added later but never removed.</DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            <Label htmlFor="topic-name">Name</Label>
            <Input
              id="topic-name"
              autoFocus
              required
              className="font-mono"
              value={name}
              aria-invalid={!!nameError}
              onChange={(e) => setName(e.target.value)}
            />
            {nameError && <p className="text-destructive text-xs">{nameError}</p>}
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="flex flex-col gap-2">
              <Label htmlFor="topic-partitions">Partitions</Label>
              <Input
                id="topic-partitions"
                type="number"
                min={1}
                max={10000}
                value={partitions}
                onChange={(e) => setPartitions(Math.max(1, Number(e.target.value)))}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="topic-rf">Replication factor</Label>
              <Select value={rf} onValueChange={setRf}>
                <SelectTrigger id="topic-rf" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="default">Broker default</SelectItem>
                  {Array.from({ length: brokers }, (_, i) => String(i + 1)).map((n) => (
                    <SelectItem key={n} value={n}>
                      {n}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="topic-cleanup">Cleanup policy</Label>
              <Select value={cleanup} onValueChange={setCleanup}>
                <SelectTrigger id="topic-cleanup" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="default">Broker default</SelectItem>
                  <SelectItem value="delete">delete</SelectItem>
                  <SelectItem value="compact">compact</SelectItem>
                  <SelectItem value="compact,delete">compact,delete</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="topic-retention">Retention</Label>
              <Select value={retention} onValueChange={setRetention}>
                <SelectTrigger id="topic-retention" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {retentions.map((r) => (
                    <SelectItem key={r.value} value={r.value}>
                      {r.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="topic-extra">Other configs</Label>
            <Textarea
              id="topic-extra"
              rows={3}
              className="font-mono text-xs"
              placeholder={'min.insync.replicas=2\nmax.message.bytes=2097152'}
              value={extra}
              onChange={(e) => setExtra(e.target.value)}
            />
            {parsed.error && <p className="text-destructive text-xs">{parsed.error}</p>}
          </div>
          {create.error && <p className="text-destructive text-sm">{create.error.message}</p>}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!name || !!nameError || !!parsed.error || create.isPending}>
              {create.isPending ? 'Creating…' : 'Create'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
