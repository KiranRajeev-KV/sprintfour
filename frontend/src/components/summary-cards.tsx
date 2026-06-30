import { AlertTriangle, CheckCircle2, FileSearch, Files, ShieldAlert, Waypoints } from 'lucide-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '#/components/ui/card'
import type { BatchSummary } from '#/lib/schemas'
import { cn } from '#/lib/utils'

type SummaryCardConfig = {
  key: keyof BatchSummary
  label: string
  detail: string
  icon: typeof Files
  tone: string
}

const cards: SummaryCardConfig[] = [
  {
    key: 'total_documents',
    label: 'Total documents',
    detail: 'Batch size loaded into the command center',
    icon: Files,
    tone: 'border-white/70 bg-white/80',
  },
  {
    key: 'needs_review',
    label: 'Needs review',
    detail: 'Open exceptions Maya should triage first',
    icon: ShieldAlert,
    tone: 'border-amber-500/30 bg-amber-500/18',
  },
  {
    key: 'failed',
    label: 'Failed',
    detail: 'Detection failures to retry in a later step',
    icon: AlertTriangle,
    tone: 'border-rose-500/30 bg-rose-500/16',
  },
  {
    key: 'ready',
    label: 'Ready',
    detail: 'Safe majority available for future bulk approval',
    icon: CheckCircle2,
    tone: 'border-emerald-500/30 bg-emerald-500/14',
  },
  {
    key: 'clean',
    label: 'Clean',
    detail: 'Documents with no seeded suggestions or failure hints',
    icon: FileSearch,
    tone: 'border-sky-500/30 bg-sky-500/14',
  },
  {
    key: 'total_redactions',
    label: 'Redactions',
    detail: 'Total seeded review signals across the batch',
    icon: Waypoints,
    tone: 'border-cyan-500/30 bg-cyan-500/14',
  },
]

export function SummaryCards({ summary }: { summary: BatchSummary }) {
  return (
    <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-6">
      {cards.map((card) => {
        const Icon = card.icon
        const value = summary[card.key]
        return (
          <Card
            key={card.key}
            className={cn(
              'island-shell gap-0 overflow-hidden border-white/40 py-0',
              card.tone,
              card.key === 'needs_review' && 'xl:col-span-2',
            )}
          >
            <CardHeader className="border-b border-black/5 px-5 py-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <CardDescription className="text-[11px] font-semibold tracking-[0.18em] text-[var(--sea-ink-soft)] uppercase">
                    {card.label}
                  </CardDescription>
                  <CardTitle className="mt-2 text-3xl font-extrabold tracking-tight text-[var(--sea-ink)]">
                    {value.toLocaleString()}
                  </CardTitle>
                </div>
                <div className="rounded-full border border-white/55 bg-white/60 p-2.5 text-[var(--lagoon-deep)] shadow-sm">
                  <Icon className="size-5" />
                </div>
              </div>
            </CardHeader>
            <CardContent className="px-5 py-4 text-sm leading-6 text-[var(--sea-ink-soft)]">
              {card.detail}
            </CardContent>
          </Card>
        )
      })}
    </section>
  )
}
