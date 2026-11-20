import type { ReactNode } from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { cn } from '@/lib/utils'

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
}) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-4">
      <div className="min-w-0">
        <h1 className="truncate font-semibold text-2xl tracking-tight">{title}</h1>
        {description && <div className="mt-1 text-muted-foreground text-sm">{description}</div>}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
  )
}

export function StatCard({
  label,
  value,
  hint,
  tone = 'default',
  icon,
}: {
  label: string
  value: ReactNode
  hint?: ReactNode
  tone?: 'default' | 'warn' | 'danger' | 'ok'
  icon?: ReactNode
}) {
  return (
    <Card className="gap-0 py-4">
      <CardContent className="px-4">
        <div className="flex items-center justify-between text-muted-foreground text-xs uppercase tracking-wide">
          {label}
          {icon}
        </div>
        <div
          className={cn(
            'mt-1 font-semibold text-2xl tabular-nums',
            tone === 'warn' && 'text-amber-500',
            tone === 'danger' && 'text-red-500',
            tone === 'ok' && 'text-emerald-500',
          )}
        >
          {value}
        </div>
        {hint && <div className="mt-0.5 text-muted-foreground text-xs">{hint}</div>}
      </CardContent>
    </Card>
  )
}

export function StatusDot({ status }: { status: 'online' | 'offline' | 'unknown' }) {
  return (
    <span
      className={cn(
        'inline-block size-2 shrink-0 rounded-full',
        status === 'online' && 'bg-emerald-500 shadow-[0_0_6px] shadow-emerald-500/60',
        status === 'offline' && 'bg-red-500',
        status === 'unknown' && 'bg-muted-foreground/50',
      )}
    />
  )
}
