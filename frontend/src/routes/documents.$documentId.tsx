import { createFileRoute, Link } from '@tanstack/react-router'
import { ArrowLeft, FileWarning, ShieldEllipsis } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '#/components/ui/card'
import { PageAlert } from '#/components/page-alert'
import { RiskBadge } from '#/components/risk-badge'
import { StatusBadge } from '#/components/status-badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '#/components/ui/table'
import { ApiError, documentDetailQueryOptions, documentRedactionsQueryOptions } from '#/lib/api'

export const Route = createFileRoute('/documents/$documentId')({
  loader: async ({ context, params }) => {
    const [document, redactions] = await Promise.all([
      context.queryClient.ensureQueryData(
        documentDetailQueryOptions(params.documentId),
      ),
      context.queryClient.ensureQueryData(
        documentRedactionsQueryOptions(params.documentId),
      ),
    ])

    return { document, redactions }
  },
  pendingComponent: DocumentDetailPending,
  errorComponent: ({ error }) => {
    const apiError =
      error instanceof ApiError ? error : new ApiError(500, 'unknown', 'request failed')
    return (
      <div className="page-wrap px-4 py-8 lg:px-0 lg:py-10">
        <PageAlert
          title={apiError.status === 404 ? 'Document not found' : 'Unable to load document'}
          message={
            apiError.status === 404
              ? 'The requested document could not be found in the current batch.'
              : 'The backend returned an error while loading the document detail.'
          }
        />
      </div>
    )
  },
  component: DocumentDetailPage,
})

function DocumentDetailPage() {
  const { document, redactions } = Route.useLoaderData()

  return (
    <main className="page-wrap space-y-6 px-4 py-8 lg:px-0 lg:py-10">
      <header className="space-y-4">
        <Link
          to="/"
          className="inline-flex items-center gap-2 rounded-full border border-white/60 bg-white/70 px-4 py-2 text-sm font-semibold text-[var(--sea-ink)] no-underline transition hover:bg-white"
        >
          <ArrowLeft className="size-4" />
          Back to batch dashboard
        </Link>
        <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
          <div className="space-y-3">
            <div className="inline-flex items-center gap-2 rounded-full border border-white/55 bg-white/70 px-3 py-1 text-xs tracking-[0.16em] text-[var(--kicker)] uppercase">
              <ShieldEllipsis className="size-4" />
              Document review surface
            </div>
            <h1 className="display-title max-w-4xl text-4xl leading-tight text-[var(--sea-ink)]">
              {document.title}
            </h1>
            <p className="text-sm text-[var(--sea-ink-soft)]">{document.source_file}</p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <StatusBadge status={document.status} />
            <RiskBadge risk={document.risk_level} />
          </div>
        </div>
      </header>

      {document.failure_hint ? (
        <PageAlert
          title="Failure hint present"
          message={document.failure_hint}
        />
      ) : null}

      <section className="grid gap-4 xl:grid-cols-[1.2fr_0.8fr]">
        <Card className="island-shell border-white/45 py-0">
          <CardHeader className="border-b border-black/5 px-5 py-4">
            <CardTitle className="text-xl text-[var(--sea-ink)]">
              Seeded document preview
            </CardTitle>
          </CardHeader>
          <CardContent className="px-5 py-5">
            <dl className="grid gap-3 sm:grid-cols-2">
              <Metric label="Character count" value={document.char_count.toLocaleString()} />
              <Metric label="Redaction count" value={document.redaction_count.toLocaleString()} />
              <Metric label="Low confidence" value={document.low_confidence_count.toLocaleString()} />
              <Metric label="Source" value={document.source} />
            </dl>
            <div className="mt-6 rounded-[1.25rem] border border-white/55 bg-white/60 p-4">
              <pre className="max-h-[34rem] overflow-auto whitespace-pre-wrap break-words font-sans text-sm leading-6 text-[var(--sea-ink)]">
                {document.text}
              </pre>
            </div>
          </CardContent>
        </Card>

        <Card className="island-shell border-white/45 py-0">
          <CardHeader className="border-b border-black/5 px-5 py-4">
            <CardTitle className="text-xl text-[var(--sea-ink)]">
              Redaction suggestions
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4 px-5 py-5">
            <div className="rounded-[1.25rem] border border-white/55 bg-white/60">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Type</TableHead>
                    <TableHead>Confidence</TableHead>
                    <TableHead>Source</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Offsets</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {redactions.items.map((redaction) => (
                    <TableRow key={redaction.id}>
                      <TableCell className="align-top font-semibold">{redaction.type}</TableCell>
                      <TableCell className="align-top">{redaction.confidence.toFixed(2)}</TableCell>
                      <TableCell className="align-top text-xs text-[var(--sea-ink-soft)]">
                        {redaction.source}
                      </TableCell>
                      <TableCell className="align-top text-xs">{redaction.suggested_status}</TableCell>
                      <TableCell className="align-top text-xs text-[var(--sea-ink-soft)]">
                        {redaction.start} - {redaction.end}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>

            <div className="space-y-3">
              {redactions.items.map((redaction) => (
                <div
                  key={`${redaction.id}-detail`}
                  className="rounded-[1.25rem] border border-white/55 bg-white/65 p-4"
                >
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div>
                      <div className="text-sm font-semibold text-[var(--sea-ink)]">
                        {redaction.type} · {redaction.text}
                      </div>
                      <div className="mt-1 text-xs text-[var(--sea-ink-soft)]">
                        {redaction.source} · {redaction.suggested_status} · {redaction.start}-
                        {redaction.end}
                      </div>
                    </div>
                    {redaction.is_ground_truth ? (
                      <span className="rounded-full border border-emerald-500/25 bg-emerald-500/12 px-2 py-1 text-[11px] uppercase">
                        Ground truth
                      </span>
                    ) : (
                      <span className="rounded-full border border-amber-500/25 bg-amber-500/12 px-2 py-1 text-[11px] uppercase">
                        Review signal
                      </span>
                    )}
                  </div>
                  <p className="mt-3 text-sm leading-6 text-[var(--sea-ink-soft)]">
                    {redaction.reason}
                  </p>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </section>
    </main>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-white/60 bg-white/60 px-4 py-3">
      <dt className="text-[11px] tracking-[0.16em] text-[var(--sea-ink-soft)] uppercase">
        {label}
      </dt>
      <dd className="mt-2 text-lg font-semibold text-[var(--sea-ink)]">{value}</dd>
    </div>
  )
}

function DocumentDetailPending() {
  return (
    <div className="page-wrap px-4 py-8 lg:px-0 lg:py-10">
      <div className="rounded-[1.75rem] border border-white/55 bg-white/65 p-10 text-sm text-[var(--sea-ink-soft)]">
        Loading document detail and redactions…
      </div>
    </div>
  )
}
