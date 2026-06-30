import { Badge } from '#/components/ui/badge'
import { cn } from '#/lib/utils'
import type { RiskLevel } from '#/lib/schemas'

const riskStyles: Record<RiskLevel, string> = {
  LOW: 'border-emerald-600/20 bg-emerald-500/10 text-emerald-900 dark:text-emerald-200',
  MEDIUM: 'border-amber-600/20 bg-amber-500/12 text-amber-950 dark:text-amber-100',
  HIGH: 'border-rose-600/20 bg-rose-500/12 text-rose-950 dark:text-rose-100',
  UNKNOWN: 'border-slate-500/20 bg-slate-500/10 text-slate-900 dark:text-slate-200',
}

export function RiskBadge({ risk }: { risk: RiskLevel }) {
  return (
    <Badge
      variant="outline"
      className={cn('rounded-full px-2.5 py-1 text-[11px] uppercase', riskStyles[risk])}
    >
      {risk}
    </Badge>
  )
}
