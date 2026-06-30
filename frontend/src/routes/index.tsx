import { createFileRoute } from '@tanstack/react-router'
import { FileX2, Layers3, Siren, Sparkles } from 'lucide-react'
import { DashboardSkeleton } from '#/components/dashboard-skeleton'
import { DocumentsTable } from '#/components/documents-table'
import { PageAlert } from '#/components/page-alert'
import { SummaryCards } from '#/components/summary-cards'
import {
  ApiError,
  batchSummaryQueryOptions,
  documentsQueryOptions,
} from '#/lib/api'
import { documentStatusSchema, riskLevelSchema } from '#/lib/schemas'
import { z } from 'zod'

const searchSchema = z.object({
  status: documentStatusSchema.optional(),
  risk: riskLevelSchema.optional(),
  q: z.string().optional().default(''),
  limit: z.coerce.number().int().positive().max(100).optional().default(50),
  offset: z.coerce.number().int().nonnegative().optional().default(0),
})

export const Route = createFileRoute('/')({
  validateSearch: (search) => searchSchema.parse(search),
  loaderDeps: ({ search }) => search,
  loader: async ({ context, deps }) => {
    const [summary, documents] = await Promise.all([
      context.queryClient.ensureQueryData(batchSummaryQueryOptions()),
      context.queryClient.ensureQueryData(documentsQueryOptions(deps)),
    ])

    return { summary, documents, search: deps }
  },
  pendingComponent: DashboardSkeleton,
  errorComponent: ({ error }) => {
    const apiError =
      error instanceof ApiError ? error : new ApiError(500, 'unknown', 'request failed')
    return (
      <div className="page-wrap px-4 py-8 lg:px-0 lg:py-10">
        <PageAlert
          title="Dashboard unavailable"
          message={
            apiError.status >= 500
              ? 'The backend could not be reached or returned an invalid response.'
              : apiError.message
          }
        />
      </div>
    )
  },
  component: Home,
})

function Home() {
  const navigate = Route.useNavigate()
  const { summary, documents, search } = Route.useLoaderData()

  const setSearch = (next: typeof search) => {
    navigate({
      to: '/',
      search: next,
      replace: true,
    })
  }

  return (
    <main className="page-wrap space-y-6 px-4 py-8 lg:px-0 lg:py-10">
      <section className="grid gap-6 xl:grid-cols-[1.1fr_0.9fr]">
        <div className="rounded-[2rem] border border-white/55 bg-[linear-gradient(140deg,rgba(255,255,255,0.9),rgba(255,255,255,0.62))] p-6 shadow-[0_28px_60px_rgba(23,58,64,0.12)]">
          <div className="inline-flex items-center gap-2 rounded-full border border-white/60 bg-white/65 px-3 py-1 text-xs tracking-[0.18em] text-[var(--kicker)] uppercase">
            <Sparkles className="size-4" />
            Working at volume
          </div>
          <h1 className="display-title mt-5 max-w-3xl text-5xl leading-[1.05] text-[var(--sea-ink)]">
            Maya&rsquo;s batch command center for pushing the safe majority through first.
          </h1>
          <p className="mt-4 max-w-2xl text-base leading-7 text-[var(--sea-ink-soft)]">
            This view prioritizes exceptions over perfection: what is ready now, what
            needs attention, what failed, and where the review queue is starting to
            spike.
          </p>
        </div>

        <div className="grid gap-4 sm:grid-cols-3 xl:grid-cols-1">
          <SignalTile
            icon={Layers3}
            label="Ready now"
            value={summary.ready}
            copy="Bulk-approval potential for the next step."
          />
          <SignalTile
            icon={Siren}
            label="Review first"
            value={summary.needs_review}
            copy="Exception documents with ambiguity or unusual volume."
            tone="warning"
          />
          <SignalTile
            icon={FileX2}
            label="Failed"
            value={summary.failed}
            copy="Detection failures waiting for retry workflow."
            tone="danger"
          />
        </div>
      </section>

      <SummaryCards summary={summary} />
      <DocumentsTable data={documents} search={search} onSearchChange={setSearch} />
    </main>
  )
}

function SignalTile({
  icon: Icon,
  label,
  value,
  copy,
  tone = 'default',
}: {
  icon: typeof Sparkles
  label: string
  value: number
  copy: string
  tone?: 'default' | 'warning' | 'danger'
}) {
  const toneClass =
    tone === 'warning'
      ? 'border-amber-500/30 bg-amber-500/16'
      : tone === 'danger'
        ? 'border-rose-500/30 bg-rose-500/16'
        : 'border-cyan-500/25 bg-cyan-500/12'

  return (
    <div className={`island-shell rounded-[1.5rem] border px-5 py-5 ${toneClass}`}>
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-[11px] tracking-[0.18em] text-[var(--sea-ink-soft)] uppercase">
            {label}
          </div>
          <div className="mt-3 text-3xl font-black text-[var(--sea-ink)]">
            {value.toLocaleString()}
          </div>
        </div>
        <div className="rounded-full border border-white/55 bg-white/65 p-2.5 text-[var(--lagoon-deep)]">
          <Icon className="size-5" />
        </div>
      </div>
      <p className="mt-4 text-sm leading-6 text-[var(--sea-ink-soft)]">{copy}</p>
    </div>
  )
}
