import { Badge } from '#/components/ui/badge'
import { cn } from '#/lib/utils'
import type { DocumentStatus } from '#/lib/schemas'

const statusStyles: Record<DocumentStatus, string> = {
  READY:
    'border-emerald-500/30 bg-emerald-500/12 text-emerald-900 dark:text-emerald-200',
  NEEDS_REVIEW:
    'border-amber-500/30 bg-amber-500/18 text-amber-950 dark:text-amber-100',
  FAILED:
    'border-rose-500/30 bg-rose-500/18 text-rose-950 dark:text-rose-100',
  CLEAN:
    'border-sky-500/30 bg-sky-500/14 text-sky-950 dark:text-sky-100',
  APPROVED:
    'border-teal-500/30 bg-teal-500/14 text-teal-950 dark:text-teal-100',
  EXPORTED:
    'border-violet-500/30 bg-violet-500/14 text-violet-950 dark:text-violet-100',
}

export function StatusBadge({ status }: { status: DocumentStatus }) {
  return (
    <Badge
      variant="outline"
      className={cn(
        'rounded-full px-2.5 py-1 text-[11px] tracking-[0.18em] uppercase',
        statusStyles[status],
      )}
    >
      {status.replace('_', ' ')}
    </Badge>
  )
}
