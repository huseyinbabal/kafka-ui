import { useNavigate } from '@tanstack/react-router'
import { Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import type { GroupDetail } from '@/lib/api'
import { useDeleteGroup } from './admin-api'
import { ResetOffsetsDialog } from './reset-offsets-dialog'

export function GroupActions({ clusterId, group }: { clusterId: string; group: GroupDetail }) {
  const navigate = useNavigate()
  const [confirm, setConfirm] = useState(false)
  const del = useDeleteGroup(clusterId, group.id)
  const empty = group.state === 'Empty'

  return (
    <>
      <ResetOffsetsDialog clusterId={clusterId} group={group.id} state={group.state} topics={group.topics} />
      <Tooltip>
        <TooltipTrigger asChild>
          {/* span keeps the tooltip working on a disabled button */}
          <span>
            <Button size="sm" variant="outline" disabled={!empty} onClick={() => setConfirm(true)}>
              <Trash2Icon /> Delete
            </Button>
          </span>
        </TooltipTrigger>
        {!empty && <TooltipContent>Only groups without active members can be deleted.</TooltipContent>}
      </Tooltip>
      <ConfirmDialog
        open={confirm}
        onOpenChange={(o) => {
          setConfirm(o)
          if (!o) del.reset()
        }}
        title={`Delete group ${group.id}?`}
        description="Its committed offsets are removed. If its consumers start again, they begin from their auto.offset.reset position."
        confirmLabel="Delete group"
        confirmText={group.id}
        pending={del.isPending}
        error={del.error}
        onConfirm={() =>
          del.mutate(undefined, {
            onSuccess: () => {
              toast.success(`Deleted ${group.id}`)
              navigate({ to: '/clusters/$clusterId/groups', params: { clusterId } })
            },
          })
        }
      />
    </>
  )
}
