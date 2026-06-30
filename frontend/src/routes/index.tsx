import { useEffect, useMemo, useRef, useState } from 'react'
import {
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import {
  Download,
  FolderOpen,
  LoaderCircle,
  Sparkles,
  Upload,
} from 'lucide-react'
import { toast } from 'sonner'
import { DashboardSkeleton } from '#/components/dashboard-skeleton'
import { DocumentsTable } from '#/components/documents-table'
import { PageAlert } from '#/components/page-alert'
import { Progress } from '#/components/ui/progress'
import { SummaryCards } from '#/components/summary-cards'
import { Button } from '#/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '#/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import {
  ApiError,
  batchSummaryQueryOptions,
  bulkApproveDocuments,
  bulkRetryDocuments,
  documentsQueryOptions,
  exportApprovedDocuments,
  latestExportQueryOptions,
  uploadDocuments,
} from '#/lib/api'
import {
  documentStatusSchema,
  riskLevelSchema,
  type BatchSummary,
  type LatestExportResponse,
  type UploadBatchResponse,
} from '#/lib/schemas'
import { z } from 'zod'

const searchSchema = z.object({
  status: documentStatusSchema.optional(),
  risk: riskLevelSchema.optional(),
  q: z.string().optional().default(''),
  limit: z.coerce.number().int().positive().max(100).optional().default(50),
  offset: z.coerce.number().int().nonnegative().optional().default(0),
})

type FeedbackState = {
  tone: 'success' | 'error'
  message: string
}

type UploadMode = 'replace' | 'append'

export const Route = createFileRoute('/')({
  validateSearch: (search) => searchSchema.parse(search),
  loaderDeps: ({ search }) => search,
  loader: async ({ context, deps }) => {
    await Promise.all([
      context.queryClient.ensureQueryData(batchSummaryQueryOptions()),
      context.queryClient.ensureQueryData(documentsQueryOptions(deps)),
      context.queryClient.ensureQueryData(latestExportQueryOptions()),
    ])

    return { search: deps }
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
  const { search } = Route.useLoaderData()
  const queryClient = useQueryClient()
  const filesInputRef = useRef<HTMLInputElement | null>(null)
  const folderInputRef = useRef<HTMLInputElement | null>(null)
  const [selectedIds, setSelectedIds] = useState<Record<string, boolean>>({})
  const [uploadMode, setUploadMode] = useState<UploadMode>('replace')
  const [uploadFeedback, setUploadFeedback] = useState<FeedbackState | null>(null)
  const [uploadResult, setUploadResult] = useState<UploadBatchResponse | null>(null)
  const [tableFeedback, setTableFeedback] = useState<FeedbackState | null>(null)
  const [exportFeedback, setExportFeedback] = useState<FeedbackState | null>(null)

  const { data: summary } = useSuspenseQuery(batchSummaryQueryOptions())
  const { data: documents } = useSuspenseQuery(documentsQueryOptions(search))
  const { data: latestExport } = useSuspenseQuery(latestExportQueryOptions())

  const inProgress = summary.queued + summary.processing
  const totalWithProgress = summary.total_documents
  const progressPercent =
    totalWithProgress > 0
      ? Math.round(((totalWithProgress - inProgress) / totalWithProgress) * 100)
      : 100
  const previousInProgressRef = useRef(inProgress)

  const visibleDocumentIDs = useMemo(
    () => new Set(documents.items.map((document) => document.id)),
    [documents.items],
  )

  useEffect(() => {
    setSelectedIds((current) => {
      const nextEntries = Object.entries(current).filter(
        ([documentId, selected]) => selected && visibleDocumentIDs.has(documentId),
      )
      const next = Object.fromEntries(nextEntries)
      const currentKeys = Object.keys(current).sort()
      const nextKeys = Object.keys(next).sort()
      if (
        currentKeys.length === nextKeys.length &&
        currentKeys.every((key, index) => key === nextKeys[index])
      ) {
        return current
      }
      return next
    })
  }, [visibleDocumentIDs])

  useEffect(() => {
    const previousInProgress = previousInProgressRef.current

    if (inProgress > 0 || (previousInProgress > 0 && inProgress === 0)) {
      void Promise.all([
        queryClient.invalidateQueries({ queryKey: ['documents'] }),
        queryClient.invalidateQueries({ queryKey: ['document'] }),
      ])
    }

    previousInProgressRef.current = inProgress
  }, [inProgress, queryClient])

  const invalidateDashboardQueries = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['batch-summary'] }),
      queryClient.invalidateQueries({ queryKey: ['documents'] }),
      queryClient.invalidateQueries({ queryKey: ['latest-export'] }),
      queryClient.invalidateQueries({ queryKey: ['document'] }),
    ])
  }

  const bulkApproveMutation = useMutation({
    mutationFn: bulkApproveDocuments,
    onSuccess: async (result) => {
      setSelectedIds({})
      toast.success(`Approved ${result.approved ?? 0} documents. Skipped ${result.skipped}.`)
      await invalidateDashboardQueries()
    },
    onError: (error) => {
      toast.error(
        error instanceof ApiError ? error.message : 'Bulk approve failed. Please try again.',
      )
    },
  })

  const bulkRetryMutation = useMutation({
    mutationFn: bulkRetryDocuments,
    onSuccess: async (result) => {
      setSelectedIds({})
      toast.success(`Retried ${result.retried ?? 0} failed documents. Skipped ${result.skipped}.`)
      await invalidateDashboardQueries()
    },
    onError: (error) => {
      toast.error(
        error instanceof ApiError ? error.message : 'Bulk retry failed. Please try again.',
      )
    },
  })

  const exportMutation = useMutation({
    mutationFn: exportApprovedDocuments,
    onSuccess: async (result) => {
      toast.success(`Exported ${result.exported_documents} approved documents. ${result.needs_review} still need review.`)
      await invalidateDashboardQueries()
    },
    onError: (error) => {
      toast.error(
        error instanceof ApiError ? error.message : 'Export failed. Please try again.',
      )
    },
  })

  const uploadMutation = useMutation({
    mutationFn: uploadDocuments,
    onSuccess: async (result) => {
      setSelectedIds({})
      setUploadResult(result)
      setTableFeedback(null)
      const msg = `Accepted ${result.accepted} file(s). Rejected ${result.rejected}.`
      toast.success(msg)
      await invalidateDashboardQueries()
    },
    onError: (error) => {
      setUploadResult(null)
      toast.error(
        error instanceof ApiError ? error.message : 'Upload failed. Please try again.',
      )
    },
  })

  const setSearch = (next: typeof search) => {
    setTableFeedback(null)
    navigate({
      to: '/',
      search: next,
      replace: true,
    })
  }

  const toggleRowSelection = (documentId: string, checked: boolean) => {
    setTableFeedback(null)
    setSelectedIds((current) => {
      if (!checked) {
        const next = { ...current }
        delete next[documentId]
        return next
      }
      return { ...current, [documentId]: true }
    })
  }

  const togglePageSelection = (checked: boolean) => {
    setTableFeedback(null)
    setSelectedIds((current) => {
      if (!checked) {
        const next = { ...current }
        for (const document of documents.items) {
          delete next[document.id]
        }
        return next
      }

      const next = { ...current }
      for (const document of documents.items) {
        next[document.id] = true
      }
      return next
    })
  }

  const handleApproveSelected = () => {
    const ids = documents.items
      .filter(
        (document) =>
          selectedIds[document.id] &&
          (document.status === 'READY' || document.status === 'CLEAN'),
      )
      .map((document) => document.id)
    bulkApproveMutation.mutate(ids)
  }

  const handleApproveSelectedClean = () => {
    const ids = documents.items
      .filter((document) => selectedIds[document.id] && document.status === 'CLEAN')
      .map((document) => document.id)
    bulkApproveMutation.mutate(ids)
  }

  const handleRetrySelected = () => {
    const ids = documents.items
      .filter((document) => selectedIds[document.id])
      .map((document) => document.id)
    bulkRetryMutation.mutate(ids)
  }

  const triggerFilesUpload = () => filesInputRef.current?.click()

  const triggerFolderUpload = () => folderInputRef.current?.click()

  const handlePickedFiles = (fileList: FileList | null) => {
    if (!fileList || fileList.length === 0) {
      return
    }
    setUploadFeedback(null)
    const files = Array.from(fileList)
    uploadMutation.mutate({ files, mode: uploadMode })
  }

  const emptyStateMessage =
    summary.total_documents === 0
      ? 'No documents loaded. Upload .txt files or a folder to begin.'
      : 'No documents match the current filters.'

  return (
    <main className="page-wrap space-y-6 px-4 py-8 lg:px-0 lg:py-10">
      <input
        ref={filesInputRef}
        type="file"
        accept=".txt,text/plain"
        multiple
        className="hidden"
        onChange={(event) => {
          handlePickedFiles(event.target.files)
          event.currentTarget.value = ''
        }}
      />
      <input
        ref={folderInputRef}
        type="file"
        multiple
        accept=".txt,text/plain"
        className="hidden"
        onChange={(event) => {
          handlePickedFiles(event.target.files)
          event.currentTarget.value = ''
        }}
        {...({
          webkitdirectory: '',
          directory: '',
        } as Record<string, string>)}
      />

      <section className="grid gap-6 xl:grid-cols-[1.05fr_0.95fr]">
        <div className="rounded-[2rem] border border-white/55 bg-[linear-gradient(140deg,rgba(255,255,255,0.9),rgba(255,255,255,0.62))] p-6 shadow-[0_28px_60px_rgba(23,58,64,0.12)]">
          <div className="inline-flex items-center gap-2 rounded-full border border-white/60 bg-white/65 px-3 py-1 text-xs tracking-[0.18em] text-[var(--kicker)] uppercase">
            <Sparkles className="size-4" />
            Redactlane
          </div>
          <h1 className="display-title mt-5 max-w-3xl text-5xl leading-[1.05] text-[var(--sea-ink)]">
            High-volume case file anonymization, built for exception-first review.
          </h1>
          <p className="mt-4 max-w-2xl text-base leading-7 text-[var(--sea-ink-soft)]">
            Redactlane moves safe documents forward in bulk, isolates risky files for
            focused review, and keeps the batch table at the center of the workflow.
          </p>
          <div className="mt-6 grid gap-3 sm:grid-cols-4">
            <CompactSignal label="Loaded" value={summary.total_documents} />
            {inProgress > 0 ? (
              <CompactSignal
                label="In progress"
                value={inProgress}
                icon={LoaderCircle}
                pulse
              />
            ) : null}
            <CompactSignal label="Ready" value={summary.ready} />
            <CompactSignal label="Needs review" value={summary.needs_review} />
          </div>
        </div>

        <UploadPanel
          mode={uploadMode}
          onModeChange={setUploadMode}
          onUploadFiles={triggerFilesUpload}
          onUploadFolder={triggerFolderUpload}
          isUploading={uploadMutation.isPending}
          feedback={uploadFeedback}
          result={uploadResult}
        />
      </section>

      {inProgress > 0 ? (
        <section className="rounded-[1.5rem] border border-blue-400/30 bg-blue-400/8 px-6 py-5">
          <div className="flex items-center gap-3">
            <LoaderCircle className="size-5 animate-spin text-blue-600" />
            <div className="text-sm font-semibold text-blue-900">
              Processing {inProgress} of {totalWithProgress} documents…
            </div>
          </div>
          <Progress value={progressPercent} className="mt-3 h-2" />
          <div className="mt-2 text-xs text-blue-700/70">
            {progressPercent}% complete — the table updates automatically as documents finish.
          </div>
        </section>
      ) : null}

      <SummaryCards summary={summary} />

      <ExportPanel
        latestExport={latestExport}
        summaryApproved={summary.approved}
        summaryExported={summary.exported}
        onExport={() => exportMutation.mutate()}
        isExportPending={exportMutation.isPending}
        feedback={exportFeedback}
      />

      <DocumentsTable
        data={documents}
        search={search}
        onSearchChange={setSearch}
        selectedIds={selectedIds}
        onToggleRow={toggleRowSelection}
        onTogglePage={togglePageSelection}
        onClearSelection={() => {
          setTableFeedback(null)
          setSelectedIds({})
        }}
        onApproveSelected={handleApproveSelected}
        onApproveSelectedClean={handleApproveSelectedClean}
        onRetrySelected={handleRetrySelected}
        feedback={tableFeedback}
        isApprovePending={bulkApproveMutation.isPending}
        isRetryPending={bulkRetryMutation.isPending}
        emptyStateMessage={emptyStateMessage}
      />
    </main>
  )
}

function CompactSignal({
  label,
  value,
  icon: Icon,
  pulse,
}: {
  label: string
  value: number
  icon?: typeof LoaderCircle
  pulse?: boolean
}) {
  return (
    <div className="rounded-[1.25rem] border border-white/55 bg-white/58 px-4 py-4">
      <div className="flex items-center justify-between">
        <div className="text-[11px] tracking-[0.16em] text-[var(--sea-ink-soft)] uppercase">
          {label}
        </div>
        {Icon ? (
          <Icon
            className={`size-4 text-blue-500 ${pulse ? 'animate-spin' : ''}`}
          />
        ) : null}
      </div>
      <div className="mt-2 text-2xl font-black text-[var(--sea-ink)]">
        {value.toLocaleString()}
      </div>
    </div>
  )
}

function UploadPanel({
  mode,
  onModeChange,
  onUploadFiles,
  onUploadFolder,
  isUploading,
  feedback,
  result,
}: {
  mode: UploadMode
  onModeChange: (value: UploadMode) => void
  onUploadFiles: () => void
  onUploadFolder: () => void
  isUploading: boolean
  feedback: FeedbackState | null
  result: UploadBatchResponse | null
}) {
  const rejectedItems = result?.items.filter((item) => !item.accepted) ?? []

  return (
    <Card className="island-shell border-white/45 py-0">
      <CardHeader className="border-b border-black/5 px-5 py-5">
        <div className="space-y-1">
          <CardTitle className="display-title text-2xl text-[var(--sea-ink)]">
            Load a batch
          </CardTitle>
          <CardDescription className="max-w-xl text-sm leading-6 text-[var(--sea-ink-soft)]">
            Upload one `.txt` file, several files, or a folder of `.txt` files. The
            backend ingests them in memory and generates deterministic review signals.
          </CardDescription>
        </div>
      </CardHeader>
      <CardContent className="space-y-5 px-5 py-5">
        <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto_auto]">
          <Select value={mode} onValueChange={(value) => onModeChange(value as UploadMode)}>
            <SelectTrigger className="h-11 rounded-full border-white/60 bg-white/70">
              <SelectValue placeholder="Upload mode" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="replace">Replace batch</SelectItem>
              <SelectItem value="append">Add to batch</SelectItem>
            </SelectContent>
          </Select>
          <Button
            type="button"
            className="rounded-full"
            onClick={onUploadFiles}
            disabled={isUploading}
          >
            <Upload className="size-4" />
            {isUploading ? 'Uploading…' : 'Upload .txt files'}
          </Button>
          <Button
            type="button"
            variant="outline"
            className="rounded-full border-white/60 bg-white/70"
            onClick={onUploadFolder}
            disabled={isUploading}
          >
            <FolderOpen className="size-4" />
            Upload folder
          </Button>
        </div>

        <div className="grid gap-4 sm:grid-cols-4">
          <UploadMetric label="Accepted" value={result?.accepted ?? 0} />
          <UploadMetric label="Rejected" value={result?.rejected ?? 0} />
          <UploadMetric label="Documents" value={result?.documents_created ?? 0} />
          <UploadMetric label="Queued" value={result?.documents_created ?? 0} />
        </div>

        {feedback ? (
          <div
            className={
              feedback.tone === 'success'
                ? 'flex items-center gap-2 text-sm text-emerald-900 dark:text-emerald-200'
                : 'flex items-center gap-2 text-sm text-rose-900 dark:text-rose-200'
            }
          >
            {feedback.tone === 'success' ? <LoaderCircle className="size-4 animate-spin" /> : null}
            {feedback.message}
          </div>
        ) : (
          <div className="text-sm text-[var(--sea-ink-soft)]">
            `.txt` only. `replace` resets the in-memory batch. `append` keeps the current
            documents and adds more. Documents are queued and processed asynchronously.
          </div>
        )}

        {rejectedItems.length > 0 ? (
          <div className="rounded-[1.25rem] border border-amber-500/25 bg-amber-500/10 px-4 py-4">
            <div className="text-sm font-semibold text-[var(--sea-ink)]">
              Rejected files
            </div>
            <ul className="mt-3 space-y-2 text-sm text-[var(--sea-ink-soft)]">
              {rejectedItems.slice(0, 6).map((item) => (
                <li key={`${item.filename}-${item.relative_path ?? 'root'}`}>
                  <span className="font-medium text-[var(--sea-ink)]">{item.relative_path ?? item.filename}</span>
                  {' '}· {item.reason}
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}

function UploadMetric({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-[1.15rem] border border-white/55 bg-white/60 px-4 py-4">
      <div className="text-[11px] tracking-[0.16em] text-[var(--sea-ink-soft)] uppercase">
        {label}
      </div>
      <div className="mt-2 text-2xl font-bold text-[var(--sea-ink)]">
        {value.toLocaleString()}
      </div>
    </div>
  )
}

function ExportPanel({
  latestExport,
  summaryApproved,
  summaryExported,
  onExport,
  isExportPending,
  feedback,
}: {
  latestExport: LatestExportResponse
  summaryApproved: number
  summaryExported: number
  onExport: () => void
  isExportPending: boolean
  feedback: FeedbackState | null
}) {
  return (
    <Card className="island-shell border-white/45 py-0">
      <CardHeader className="border-b border-black/5 px-5 py-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div className="space-y-1">
            <CardTitle className="display-title text-2xl text-[var(--sea-ink)]">
              Export gate
            </CardTitle>
            <CardDescription className="max-w-2xl text-sm leading-6 text-[var(--sea-ink-soft)]">
              Export only runs on APPROVED documents. READY still needs approval and
              NEEDS_REVIEW stays out of the safe batch.
            </CardDescription>
          </div>
          <Button
            type="button"
            className="rounded-full"
            disabled={isExportPending || summaryApproved === 0}
            onClick={onExport}
          >
            <Download className="size-4" />
            {isExportPending ? 'Exporting…' : 'Export approved documents'}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-4 px-5 py-5">
        <div className="grid gap-4 md:grid-cols-3">
          <ExportStat label="Approved now" value={summaryApproved} />
          <ExportStat label="Already exported" value={summaryExported} />
          <ExportStat
            label="Latest export"
            value={latestExport.has_export ? latestExport.exported_documents : 0}
          />
        </div>

        {latestExport.has_export ? (
          <div className="space-y-4 rounded-[1.35rem] border border-white/55 bg-white/65 p-4">
            <div className="grid gap-4 md:grid-cols-6">
            <ExportMetric label="Export ID" value={latestExport.export_id} />
            <ExportMetric
              label="Exported"
              value={String(latestExport.exported_documents)}
            />
            <ExportMetric
              label="Skipped"
              value={String(latestExport.skipped_documents)}
            />
            <ExportMetric label="Needs review" value={String(latestExport.needs_review)} />
            <ExportMetric label="Failed" value={String(latestExport.failed)} />
            <ExportMetric label="Ready" value={String(latestExport.ready)} />
            </div>

            {latestExport.output_dir ? (
              <div className="space-y-2">
                <div className="text-[11px] tracking-[0.16em] text-[var(--sea-ink-soft)] uppercase">
                  Export folder
                </div>
                <div className="rounded-[1rem] border border-white/55 bg-white/72 px-3 py-3 font-mono text-xs text-[var(--sea-ink)]">
                  {latestExport.output_dir}
                </div>
              </div>
            ) : null}

            {latestExport.files && latestExport.files.length > 0 ? (
              <div className="space-y-2">
                <div className="text-[11px] tracking-[0.16em] text-[var(--sea-ink-soft)] uppercase">
                  Exported files
                </div>
                <div className="rounded-[1rem] border border-white/55 bg-white/72 px-3 py-3">
                  <ul className="space-y-2 text-sm text-[var(--sea-ink)]">
                    {latestExport.files.slice(0, 6).map((file) => (
                      <li key={file} className="font-mono text-xs">
                        {file}
                      </li>
                    ))}
                  </ul>
                  {latestExport.files.length > 6 ? (
                    <div className="mt-3 text-xs text-[var(--sea-ink-soft)]">
                      +{latestExport.files.length - 6} more
                    </div>
                  ) : null}
                </div>
              </div>
            ) : null}

            <div className="text-xs text-[var(--sea-ink-soft)]">
              Created at {formatTimestamp(latestExport.created_at)}
            </div>
          </div>
        ) : (
          <div className="rounded-[1.35rem] border border-dashed border-white/60 bg-white/50 px-4 py-4 text-sm text-[var(--sea-ink-soft)]">
            No export has been run yet.
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function ExportStat({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-[1.2rem] border border-white/55 bg-white/60 px-4 py-4">
      <div className="text-[11px] tracking-[0.16em] text-[var(--sea-ink-soft)] uppercase">
        {label}
      </div>
      <div className="mt-2 text-3xl font-bold text-[var(--sea-ink)]">
        {value.toLocaleString()}
      </div>
    </div>
  )
}

function ExportMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="space-y-1">
      <div className="text-[11px] tracking-[0.16em] text-[var(--sea-ink-soft)] uppercase">
        {label}
      </div>
      <div className="text-sm font-semibold text-[var(--sea-ink)]">{value}</div>
    </div>
  )
}

function formatTimestamp(value: string) {
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) {
    return value
  }
  return parsed.toISOString().replace('T', ' ').replace('.000Z', ' UTC')
}
