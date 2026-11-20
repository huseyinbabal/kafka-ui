import { useNavigate } from '@tanstack/react-router'
import { EllipsisVerticalIcon, EraserIcon, ListPlusIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { TopicDetail } from '@/lib/api'
import { formatNumber } from '@/lib/format'
import { useDeleteTopic, usePurgeTopic, useSetPartitions } from './admin-api'

type Open = 'partitions' | 'purge' | 'delete' | null

export function TopicActions({ clusterId, topic }: { clusterId: string; topic: TopicDetail }) {
  const navigate = useNavigate()
  const [open, setOpen] = useState<Open>(null)
  const [count, setCount] = useState(topic.partitions + 1)
  const setPartitions = useSetPartitions(clusterId, topic.name)
  const purge = usePurgeTopic(clusterId, topic.name)
  const del = useDeleteTopic(clusterId, topic.name)

  const close = () => {
    setOpen(null)
    setPartitions.reset()
    purge.reset()
    del.reset()
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="icon" aria-label="Topic actions">
            <EllipsisVerticalIcon />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-48">
          <DropdownMenuItem
            onSelect={() => {
              setCount(topic.partitions + 1)
              setOpen('partitions')
            }}
          >
            <ListPlusIcon /> Add partitions
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => setOpen('purge')}>
            <EraserIcon /> Purge messages
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={() => setOpen('delete')}>
            <Trash2Icon /> Delete topic
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={open === 'partitions'} onOpenChange={(o) => !o && close()}>
        <DialogContent className="sm:max-w-md">
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault()
              setPartitions.mutate(count, {
                onSuccess: () => {
                  toast.success(`${topic.name} now has ${count} partitions`)
                  close()
                },
              })
            }}
          >
            <DialogHeader>
              <DialogTitle>Add partitions</DialogTitle>
              <DialogDescription>
                Partitions cannot be removed later. Keyed records will map to different partitions, which breaks per-key
                ordering across the change.
              </DialogDescription>
            </DialogHeader>
            <div className="flex flex-col gap-2">
              <Label htmlFor="partition-count">New partition count (currently {topic.partitions})</Label>
              <Input
                id="partition-count"
                type="number"
                min={topic.partitions + 1}
                value={count}
                onChange={(e) => setCount(Number(e.target.value))}
              />
            </div>
            {setPartitions.error && <p className="text-destructive text-sm">{setPartitions.error.message}</p>}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={close}>
                Cancel
              </Button>
              <Button type="submit" disabled={count <= topic.partitions || setPartitions.isPending}>
                {setPartitions.isPending ? 'Saving…' : 'Add partitions'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={open === 'purge'}
        onOpenChange={(o) => !o && close()}
        title={`Purge ${topic.name}?`}
        description={
          <>
            Deletes all {formatNumber(topic.messageCount)} messages in every partition. Consumers that have not read
            them will skip ahead. This cannot be undone.
          </>
        }
        confirmLabel="Purge messages"
        confirmText={topic.name}
        pending={purge.isPending}
        error={purge.error}
        onConfirm={() =>
          purge.mutate(undefined, {
            onSuccess: (r) => {
              const failed = r.items.filter((i) => i.error)
              if (failed.length > 0) toast.error(`${failed.length} partitions failed: ${failed[0].error}`)
              else toast.success(`Purged ${topic.name}`)
              close()
            },
          })
        }
      />

      <ConfirmDialog
        open={open === 'delete'}
        onOpenChange={(o) => !o && close()}
        title={`Delete ${topic.name}?`}
        description="The topic, its messages and its configuration are removed from the cluster. Producers and consumers using it will fail. This cannot be undone."
        confirmLabel="Delete topic"
        confirmText={topic.name}
        pending={del.isPending}
        error={del.error}
        onConfirm={() =>
          del.mutate(undefined, {
            onSuccess: () => {
              toast.success(`Deleted ${topic.name}`)
              close()
              navigate({ to: '/clusters/$clusterId/topics', params: { clusterId } })
            },
          })
        }
      />
    </>
  )
}
