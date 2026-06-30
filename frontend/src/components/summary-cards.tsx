import {
  AlertTriangle,
  CheckCircle2,
  Clock,
  FileCheck2,
  FileOutput,
  Files,
  LoaderCircle,
  ShieldAlert,
  Waypoints,
} from 'lucide-react'
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
    detail: 'Batch size currently loaded into Redactlane',
    icon: Files,
    tone: 'border-white/70 bg-white/80',
  },
  {
    key: 'processing',
    label: 'Processing',
    detail: 'Documents currently being analyzed by the worker pool',
    icon: LoaderCircle,
    tone: 'border-blue-400/30 bg-blue-400/14',
  },
  {
    key: 'needs_review',
    label: 'Needs review',
    detail: 'Open exception files that need focused review first',
    icon: ShieldAlert,
    tone: 'border-amber-500/30 bg-amber-500/18',
  },
  {
    key: 'ready',
    label: 'Ready',
    detail: 'Safe majority available for future bulk approval',
    icon: CheckCircle2,
    tone: 'border-emerald-500/30 bg-emerald-500/14',
  },
  {
    key: 'failed',
    label: 'Failed',
    detail: 'Detection failures to retry in a later step',
    icon: AlertTriangle,
    tone: 'border-rose-500/30 bg-rose-500/16',
  },
]

export function SummaryCards({ summary }: { summary: BatchSummary }) {
  return (
    <section className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-5">
        {cards.map((card) => {
          const Icon = card.icon
          const value = summary[card.key]
          return (
            <Card
              key={card.key}
              className={cn(
                'island-shell gap-0 overflow-hidden border-white/40 py-0',
                card.tone,
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
      </div>

      <Card className="island-shell border-white/40 py-0">
        <CardHeader className="border-b border-black/5 px-5 py-4">
          <CardTitle className="text-base font-semibold text-[var(--sea-ink)]">
            Secondary batch signals
          </CardTitle>
          <CardDescription className="text-sm text-[var(--sea-ink-soft)]">
            Approval, export, clean files, and total review load stay visible without
            crowding the main command row.
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 px-5 py-4 sm:grid-cols-2 xl:grid-cols-6">
          <SecondaryStat icon={Clock} label="Queued" value={summary.queued} />
          <SecondaryStat
            icon={FileCheck2}
            label="Approved"
            value={summary.approved}
          />
          <SecondaryStat
            icon={FileOutput}
            label="Exported"
            value={summary.exported}
          />
          <SecondaryStat icon={CheckCircle2} label="Clean" value={summary.clean} />
          <SecondaryStat
            icon={Waypoints}
            label="Redactions"
            value={summary.total_redactions}
          />
          <SecondaryStat
            icon={ShieldAlert}
            label="Blocking docs"
            value={summary.blocking_review_documents ?? 0}
          />
        </CardContent>
      </Card>
    </section>
  )
}

function SecondaryStat({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof Files
  label: string
  value: number
}) {
  return (
    <div className="rounded-[1.2rem] border border-white/55 bg-white/62 px-4 py-4">
      <div className="flex items-center justify-between gap-3">
        <div className="text-[11px] tracking-[0.16em] text-[var(--sea-ink-soft)] uppercase">
          {label}
        </div>
        <Icon className="size-4 text-[var(--lagoon-deep)]" />
      </div>
      <div className="mt-2 text-2xl font-bold text-[var(--sea-ink)]">
        {value.toLocaleString()}
      </div>
    </div>
  )
}
