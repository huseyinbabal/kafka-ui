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
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { ConfigEntry } from '@/lib/api'
import { useAlterTopicConfigs } from './admin-api'

type Props = { clusterId: string; topic: string; entry: ConfigEntry | null; onClose: () => void }

export function ConfigEditDialog({ clusterId, topic, entry, onClose }: Props) {
  return (
    <Dialog open={!!entry} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        {/* Keyed so each opened entry starts with fresh form state. */}
        {entry && <ConfigForm key={entry.name} clusterId={clusterId} topic={topic} entry={entry} onClose={onClose} />}
      </DialogContent>
    </Dialog>
  )
}

function ConfigForm({ clusterId, topic, entry, onClose }: Props & { entry: ConfigEntry }) {
  const [value, setValue] = useState(entry.value ?? '')
  const alter = useAlterTopicConfigs(clusterId, topic)

  const apply = (v: string | null) => {
    alter.mutate([{ name: entry.name, value: v }], {
      onSuccess: () => {
        toast.success(v === null ? `${entry.name} reset to default` : `${entry.name} updated`)
        onClose()
      },
    })
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    apply(value)
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <DialogHeader>
        <DialogTitle className="font-mono text-base">{entry.name}</DialogTitle>
        <DialogDescription>
          {entry.isDefault ? 'Currently the default value.' : `Currently set at ${entry.source}.`} The broker validates
          the new value.
        </DialogDescription>
      </DialogHeader>
      <div className="flex flex-col gap-2">
        <Label htmlFor="config-value">Value</Label>
        <Input
          id="config-value"
          autoFocus
          className="font-mono"
          value={value}
          onChange={(e) => setValue(e.target.value)}
        />
      </div>
      {alter.error && <p className="text-destructive text-sm">{alter.error.message}</p>}
      <DialogFooter className="sm:justify-between">
        <Button type="button" variant="ghost" disabled={entry.isDefault || alter.isPending} onClick={() => apply(null)}>
          Reset to default
        </Button>
        <div className="flex gap-2">
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={alter.isPending || value === (entry.value ?? '')}>
            {alter.isPending ? 'Saving…' : 'Save'}
          </Button>
        </div>
      </DialogFooter>
    </form>
  )
}
