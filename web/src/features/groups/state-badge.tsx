import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

const tones: Record<string, string> = {
  Stable: 'border-emerald-500/40 text-emerald-600 dark:text-emerald-400',
  Empty: 'border-muted-foreground/30 text-muted-foreground',
  PreparingRebalance: 'border-amber-500/40 text-amber-600 dark:text-amber-400',
  CompletingRebalance: 'border-amber-500/40 text-amber-600 dark:text-amber-400',
  Dead: 'border-red-500/40 text-red-500',
}

export function StateBadge({ state }: { state: string }) {
  return (
    <Badge variant="outline" className={cn('font-normal', tones[state])}>
      {state || 'Unknown'}
    </Badge>
  )
}
