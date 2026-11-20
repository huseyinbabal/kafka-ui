import { type ReactNode, useState } from 'react'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: ReactNode
  confirmLabel: string
  /** When set, the user must type this exact text to enable the action. */
  confirmText?: string
  pending?: boolean
  error?: Error | null
  onConfirm: () => void
}

/** A destructive confirmation that stays open while the action runs and shows its error. */
export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  confirmText,
  pending,
  error,
  onConfirm,
}: Props) {
  const [typed, setTyped] = useState('')
  const ready = !confirmText || typed === confirmText
  return (
    <AlertDialog
      open={open}
      onOpenChange={(o) => {
        if (!o) setTyped('')
        onOpenChange(o)
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div>{description}</div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        {confirmText && (
          <div className="flex flex-col gap-2">
            <Label htmlFor="confirm-text" className="font-normal text-sm">
              Type <span className="font-mono font-semibold">{confirmText}</span> to confirm
            </Label>
            <Input
              id="confirm-text"
              autoComplete="off"
              className="font-mono"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
            />
          </div>
        )}
        {error && <p className="text-destructive text-sm">{error.message}</p>}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>Cancel</AlertDialogCancel>
          {/* A plain button, so the dialog stays open until the request finishes. */}
          <Button variant="destructive" disabled={!ready || pending} onClick={onConfirm}>
            {pending ? 'Working…' : confirmLabel}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
