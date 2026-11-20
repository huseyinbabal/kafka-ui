import { RotateCcwIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useTopics } from '@/features/topics/api'
import { formatNumber, formatOffset } from '@/lib/format'
import { canReset, type ResetInput, type ResetResult, type ResetStrategy, useResetOffsets } from './admin-api'

const strategies: { value: ResetStrategy; label: string }[] = [
  { value: 'earliest', label: 'Earliest' },
  { value: 'latest', label: 'Latest' },
  { value: 'offset', label: 'Specific offset' },
  { value: 'timestamp', label: 'Timestamp' },
  { value: 'shift', label: 'Shift by' },
]

function toLocalInput(ms: number) {
  return new Date(ms - new Date(ms).getTimezoneOffset() * 60_000).toISOString().slice(0, 16)
}

function Delta({ from, to }: { from: number; to: number }) {
  if (from < 0 || to < 0) return <span className="text-muted-foreground">—</span>
  const d = to - from
  if (d === 0) return <span className="text-muted-foreground">0</span>
  return (
    <span className={d < 0 ? 'text-amber-500' : 'text-emerald-500'}>
      {d > 0 ? '+' : ''}
      {formatNumber(d)}
    </span>
  )
}

type Props = { clusterId: string; group: string; state: string; topics: string[] }

export function ResetOffsetsDialog({ clusterId, group, state, topics }: Props) {
  const allTopics = useTopics(clusterId)
  const [open, setOpen] = useState(false)
  const [topic, setTopic] = useState(topics[0] ?? '')
  const [strategy, setStrategy] = useState<ResetStrategy>('earliest')
  const [value, setValue] = useState(0)
  const [plan, setPlan] = useState<{ input: ResetInput; result: ResetResult } | null>(null)
  const reset = useResetOffsets(clusterId, group)
  const allowed = canReset(state)

  const input: ResetInput = { topic, partitions: [], strategy, value, dryRun: true }
  const planIsCurrent = plan !== null && JSON.stringify(plan.input) === JSON.stringify(input)
  const choices = [...new Set([...topics, ...(allTopics.data ?? []).filter((t) => !t.internal).map((t) => t.name)])]

  const preview = () => reset.mutate(input, { onSuccess: (result) => setPlan({ input, result }) })
  const apply = () =>
    reset.mutate(
      { ...input, dryRun: false },
      {
        onSuccess: (r) => {
          const failed = r.items.filter((i) => i.error)
          if (failed.length > 0) toast.warning(`Applied with ${failed.length} skipped partitions`)
          else toast.success(`Offsets of ${group} on ${topic} were reset`)
          setOpen(false)
        },
      },
    )

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        setOpen(o)
        if (!o) {
          setPlan(null)
          reset.reset()
        }
      }}
    >
      <DialogTrigger asChild>
        <Button size="sm" variant="outline">
          <RotateCcwIcon /> Reset offsets
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Reset offsets of {group}</DialogTitle>
          <DialogDescription>Preview the change first; nothing is committed until you apply it.</DialogDescription>
        </DialogHeader>

        {!allowed && (
          <Alert variant="destructive">
            <AlertDescription>
              The group is {state}. Stop all of its consumers first; offsets can only change while the group is Empty.
            </AlertDescription>
          </Alert>
        )}

        <div className="grid gap-4 sm:grid-cols-3">
          <div className="flex flex-col gap-2">
            <Label htmlFor="reset-topic">Topic</Label>
            <Select value={topic} onValueChange={setTopic}>
              <SelectTrigger id="reset-topic" className="w-full">
                <SelectValue placeholder="Choose a topic" />
              </SelectTrigger>
              <SelectContent>
                {choices.map((t) => (
                  <SelectItem key={t} value={t}>
                    {t}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="reset-strategy">Reset to</Label>
            <Select
              value={strategy}
              onValueChange={(v) => {
                setStrategy(v as ResetStrategy)
                setValue(v === 'timestamp' ? Date.now() - 60 * 60_000 : 0)
              }}
            >
              <SelectTrigger id="reset-strategy" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {strategies.map((s) => (
                  <SelectItem key={s.value} value={s.value}>
                    {s.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {(strategy === 'offset' || strategy === 'shift') && (
            <div className="flex flex-col gap-2">
              <Label htmlFor="reset-value">{strategy === 'offset' ? 'Offset' : 'Shift (negative rewinds)'}</Label>
              <Input
                id="reset-value"
                type="number"
                min={strategy === 'offset' ? 0 : undefined}
                value={value}
                onChange={(e) => setValue(Number(e.target.value))}
              />
            </div>
          )}
          {strategy === 'timestamp' && (
            <div className="flex flex-col gap-2">
              <Label htmlFor="reset-ts">From (local time)</Label>
              <Input
                id="reset-ts"
                type="datetime-local"
                value={toLocalInput(value)}
                onChange={(e) => {
                  const ms = new Date(e.target.value).getTime()
                  if (Number.isFinite(ms)) setValue(ms)
                }}
              />
            </div>
          )}
        </div>

        {plan && (
          <div className="max-h-72 overflow-auto rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Partition</TableHead>
                  <TableHead className="text-right">Current</TableHead>
                  <TableHead className="text-right">New</TableHead>
                  <TableHead className="text-right">Change</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {plan.result.items.map((i) => (
                  <TableRow key={i.partition}>
                    <TableCell>{i.partition}</TableCell>
                    <TableCell className="text-right tabular-nums">{formatOffset(i.current)}</TableCell>
                    <TableCell className="text-right tabular-nums">{formatOffset(i.target)}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      <Delta from={i.current} to={i.target} />
                    </TableCell>
                    <TableCell className="text-muted-foreground text-xs">{i.error}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}

        {reset.error && <p className="text-destructive text-sm">{reset.error.message}</p>}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button type="button" variant="secondary" disabled={!allowed || !topic || reset.isPending} onClick={preview}>
            Preview
          </Button>
          <Button type="button" disabled={!allowed || !planIsCurrent || reset.isPending} onClick={apply}>
            {reset.isPending ? 'Working…' : 'Apply'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
