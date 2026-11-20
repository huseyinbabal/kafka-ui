import { AlertTriangleIcon, InboxIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError } from '@/lib/api'

export function ErrorState({ error, title = 'Something went wrong' }: { error: unknown; title?: string }) {
  const message = error instanceof Error ? error.message : String(error)
  const code = error instanceof ApiError ? error.code : undefined
  return (
    <Alert variant="destructive">
      <AlertTriangleIcon />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>
        {message}
        {code && <span className="font-mono text-xs opacity-70"> ({code})</span>}
      </AlertDescription>
    </Alert>
  )
}

export function EmptyState({ title, description, icon }: { title: string; description?: string; icon?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-12 text-center text-muted-foreground">
      {icon ?? <InboxIcon className="size-8 opacity-50" />}
      <p className="font-medium text-foreground">{title}</p>
      {description && <p className="text-sm">{description}</p>}
    </div>
  )
}

export function LoadingRows({ rows = 5 }: { rows?: number }) {
  return (
    <div className="grid gap-2">
      {Array.from({ length: rows }, (_, i) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: static placeholders
        <Skeleton key={i} className="h-9 w-full" />
      ))}
    </div>
  )
}
