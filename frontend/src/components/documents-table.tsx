import { useEffect, useRef } from 'react'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import { ChevronLeft, ChevronRight, Search } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { Button } from '#/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '#/components/ui/card'
import { Input } from '#/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '#/components/ui/table'
import { RiskBadge } from '#/components/risk-badge'
import { StatusBadge } from '#/components/status-badge'
import type { DocumentListItem, DocumentStatus, RiskLevel } from '#/lib/schemas'

type SearchState = {
  status?: DocumentStatus
  risk?: RiskLevel
  q: string
  limit: number
  offset: number
}

type MutationFeedback = {
  tone: 'success' | 'error'
  message: string
}

type DocumentsTableProps = {
  data: {
    items: DocumentListItem[]
    total: number
    limit: number
    offset: number
  }
  search: SearchState
  onSearchChange: (next: SearchState) => void
  selectedIds: Record<string, boolean>
  onToggleRow: (documentId: string, checked: boolean) => void
  onTogglePage: (checked: boolean) => void
  onClearSelection: () => void
  onApproveSelected: () => void
  onApproveSelectedClean: () => void
  onRetrySelected: () => void
  feedback?: MutationFeedback | null
  isApprovePending: boolean
  isRetryPending: boolean
  emptyStateMessage?: string
}

const statusOptions: Array<{ label: string; value: DocumentStatus | 'ALL' }> = [
  { label: 'All statuses', value: 'ALL' },
  { label: 'Ready', value: 'READY' },
  { label: 'Needs Review', value: 'NEEDS_REVIEW' },
  { label: 'Failed', value: 'FAILED' },
  { label: 'Clean', value: 'CLEAN' },
  { label: 'Approved', value: 'APPROVED' },
  { label: 'Exported', value: 'EXPORTED' },
]

const riskOptions: Array<{ label: string; value: RiskLevel | 'ALL' }> = [
  { label: 'All risks', value: 'ALL' },
  { label: 'High', value: 'HIGH' },
  { label: 'Medium', value: 'MEDIUM' },
  { label: 'Low', value: 'LOW' },
  { label: 'Unknown', value: 'UNKNOWN' },
]

const pageSizeOptions = [25, 50, 100]

export function DocumentsTable({
  data,
  search,
  onSearchChange,
  selectedIds,
  onToggleRow,
  onTogglePage,
  onClearSelection,
  onApproveSelected,
  onApproveSelectedClean,
  onRetrySelected,
  feedback,
  isApprovePending,
  isRetryPending,
  emptyStateMessage,
}: DocumentsTableProps) {
  const selectedItems = data.items.filter((document) => selectedIds[document.id])
  const selectedCount = selectedItems.length
  const selectedReadyCount = selectedItems.filter((document) => document.status === 'READY').length
  const selectedCleanCount = selectedItems.filter((document) => document.status === 'CLEAN').length
  const selectedNeedsReviewCount = selectedItems.filter((document) => document.status === 'NEEDS_REVIEW').length
  const selectedFailedCount = selectedItems.filter((document) => document.status === 'FAILED').length
  const selectedApprovedCount = selectedItems.filter((document) => document.status === 'APPROVED').length
  const selectedExportedCount = selectedItems.filter((document) => document.status === 'EXPORTED').length
  const pageSelectedCount = selectedItems.length
  const allPageSelected = data.items.length > 0 && pageSelectedCount === data.items.length
  const somePageSelected = pageSelectedCount > 0 && !allPageSelected

  const columns: ColumnDef<DocumentListItem>[] = [
    {
      id: 'selection',
      header: () => (
        <div className="flex justify-center">
          <SelectionCheckbox
            checked={allPageSelected}
            indeterminate={somePageSelected}
            onCheckedChange={(checked) => onTogglePage(checked)}
            ariaLabel="Select current page"
          />
        </div>
      ),
      cell: ({ row }) => (
        <div className="flex justify-center">
          <SelectionCheckbox
            checked={Boolean(selectedIds[row.original.id])}
            onCheckedChange={(checked) => onToggleRow(row.original.id, checked)}
            ariaLabel={`Select ${row.original.id}`}
          />
        </div>
      ),
    },
    {
      accessorKey: 'title',
      header: 'Title',
      cell: ({ row }) => {
        const fullTitle = row.original.title
        const displayTitle = truncateTitle(fullTitle, 200)

        return (
          <div className="min-w-[18rem] max-w-[34rem] space-y-1">
            <Link
              to="/documents/$documentId"
              params={{ documentId: row.original.id }}
              title={fullTitle}
              className="line-clamp-3 font-semibold text-[var(--sea-ink)] underline-offset-4 hover:underline"
            >
              {displayTitle}
            </Link>
            <div className="text-xs text-[var(--sea-ink-soft)]">{row.original.id}</div>
          </div>
        )
      },
    },
    {
      accessorKey: 'status',
      header: 'Status',
      cell: ({ row }) => <StatusBadge status={row.original.status} />,
    },
    {
      accessorKey: 'risk_level',
      header: 'Risk',
      cell: ({ row }) => <RiskBadge risk={row.original.risk_level} />,
    },
    {
      accessorKey: 'pii_count',
      header: 'PII Count',
      cell: ({ row }) => <span className="font-semibold">{row.original.pii_count}</span>,
    },
    {
      accessorKey: 'low_confidence_count',
      header: 'Low Confidence',
      cell: ({ row }) => (
        <span
          className={
            row.original.low_confidence_count > 0
              ? 'font-semibold text-amber-700 dark:text-amber-200'
              : ''
          }
        >
          {row.original.low_confidence_count}
        </span>
      ),
    },
  ]

  const table = useReactTable({
    data: data.items,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getRowId: (row) => row.id,
  })

  const currentPage = Math.floor(data.offset / data.limit) + 1
  const totalPages = Math.max(1, Math.ceil(data.total / data.limit))
  const start = data.total === 0 ? 0 : data.offset + 1
  const end = Math.min(data.offset + data.items.length, data.total)

  return (
    <Card className="island-shell border-white/45 py-0">
      <CardHeader className="border-b border-black/5 px-5 py-5">
        <div className="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
          <div className="space-y-1">
            <CardTitle className="display-title text-3xl text-[var(--sea-ink)]">
              Review queue
            </CardTitle>
            <CardDescription className="max-w-2xl text-sm leading-6 text-[var(--sea-ink-soft)]">
              Scan the safe majority quickly, then drill into risky or failed contracts
              without opening all 220 files.
            </CardDescription>
          </div>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <div className="relative xl:col-span-2">
              <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-[var(--sea-ink-soft)]" />
              <Input
                value={search.q}
                onChange={(event) =>
                  onSearchChange({
                    ...search,
                    q: event.target.value,
                    offset: 0,
                  })
                }
                placeholder="Search id, title, or source file"
                className="h-10 rounded-full border-white/60 bg-white/70 pl-10"
              />
            </div>
            <Select
              value={search.status ?? 'ALL'}
              onValueChange={(value) =>
                onSearchChange({
                  ...search,
                  status: value === 'ALL' ? undefined : (value as DocumentStatus),
                  offset: 0,
                })
              }
            >
              <SelectTrigger className="h-10 w-full rounded-full border-white/60 bg-white/70">
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                {statusOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={search.risk ?? 'ALL'}
              onValueChange={(value) =>
                onSearchChange({
                  ...search,
                  risk: value === 'ALL' ? undefined : (value as RiskLevel),
                  offset: 0,
                })
              }
            >
              <SelectTrigger className="h-10 w-full rounded-full border-white/60 bg-white/70">
                <SelectValue placeholder="Risk" />
              </SelectTrigger>
              <SelectContent>
                {riskOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
      </CardHeader>

      <CardContent className="space-y-5 px-0 py-0">
        {selectedCount > 0 ? (
          <div className="border-b border-black/5 px-5 py-4">
            <div className="flex flex-col gap-4 rounded-[1.4rem] border border-cyan-500/20 bg-cyan-500/8 px-4 py-4">
              <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                <div className="space-y-2">
                  <div className="text-sm font-semibold text-[var(--sea-ink)]">
                    {selectedCount} selected on this page
                  </div>
                  <div className="flex flex-wrap gap-2 text-xs text-[var(--sea-ink-soft)]">
                    <SelectionPill label="READY" count={selectedReadyCount} />
                    <SelectionPill label="CLEAN" count={selectedCleanCount} />
                    <SelectionPill label="NEEDS_REVIEW" count={selectedNeedsReviewCount} />
                    <SelectionPill label="FAILED" count={selectedFailedCount} />
                    <SelectionPill label="APPROVED" count={selectedApprovedCount} />
                    <SelectionPill label="EXPORTED" count={selectedExportedCount} />
                  </div>
                  <div className="text-xs leading-5 text-[var(--sea-ink-soft)]">
                    Bulk approve now changes READY and CLEAN rows. Retry only changes
                    FAILED rows. NEEDS_REVIEW is intentionally excluded from bulk approval.
                  </div>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    className="rounded-full"
                    disabled={
                      selectedReadyCount + selectedCleanCount === 0 ||
                      isApprovePending ||
                      isRetryPending
                    }
                    onClick={onApproveSelected}
                  >
                    {isApprovePending
                      ? 'Approving…'
                      : `Approve selected READY + CLEAN (${selectedReadyCount + selectedCleanCount})`}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    className="rounded-full border-white/60 bg-white/70"
                    disabled={selectedCleanCount === 0 || isApprovePending || isRetryPending}
                    onClick={onApproveSelectedClean}
                  >
                    {isApprovePending
                      ? 'Approving…'
                      : `Approve selected CLEAN (${selectedCleanCount})`}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    className="rounded-full border-white/60 bg-white/70"
                    disabled={selectedFailedCount === 0 || isRetryPending || isApprovePending}
                    onClick={onRetrySelected}
                  >
                    {isRetryPending ? 'Retrying…' : `Retry selected FAILED (${selectedFailedCount})`}
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    className="rounded-full"
                    disabled={isApprovePending || isRetryPending}
                    onClick={onClearSelection}
                  >
                    Clear selection
                  </Button>
                </div>
              </div>

              {feedback ? (
                <div
                  className={
                    feedback.tone === 'success'
                      ? 'text-sm text-emerald-900 dark:text-emerald-200'
                      : 'text-sm text-rose-900 dark:text-rose-200'
                  }
                >
                  {feedback.message}
                </div>
              ) : null}
            </div>
          </div>
        ) : feedback ? (
          <div className="border-b border-black/5 px-5 py-4">
            <div
              className={
                feedback.tone === 'success'
                  ? 'text-sm text-emerald-900 dark:text-emerald-200'
                  : 'text-sm text-rose-900 dark:text-rose-200'
              }
            >
              {feedback.message}
            </div>
          </div>
        ) : null}

        <div className="flex flex-col gap-3 border-b border-black/5 px-5 py-4 text-sm text-[var(--sea-ink-soft)] sm:flex-row sm:items-center sm:justify-between">
          <div>
            Showing <span className="font-semibold text-[var(--sea-ink)]">{start}</span>
            {' '}to{' '}
            <span className="font-semibold text-[var(--sea-ink)]">{end}</span>
            {' '}of{' '}
            <span className="font-semibold text-[var(--sea-ink)]">{data.total}</span>
            {' '}documents
          </div>
          <div className="flex items-center gap-3">
            <span>Rows per page</span>
            <Select
              value={String(search.limit)}
              onValueChange={(value) =>
                onSearchChange({
                  ...search,
                  limit: Number(value),
                  offset: 0,
                })
              }
            >
              <SelectTrigger className="h-9 rounded-full border-white/60 bg-white/70">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {pageSizeOptions.map((pageSize) => (
                  <SelectItem key={pageSize} value={String(pageSize)}>
                    {pageSize}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        <div className="px-2 pb-2">
          <Table>
            <TableHeader>
              {table.getHeaderGroups().map((headerGroup) => (
                <TableRow key={headerGroup.id}>
                  {headerGroup.headers.map((header) => (
                    <TableHead
                      key={header.id}
                      className="px-3 py-3 text-xs tracking-[0.16em] uppercase"
                    >
                      {header.isPlaceholder
                        ? null
                        : flexRender(header.column.columnDef.header, header.getContext())}
                    </TableHead>
                  ))}
                </TableRow>
              ))}
            </TableHeader>
            <TableBody>
              {table.getRowModel().rows.length > 0 ? (
                table.getRowModel().rows.map((row) => (
                  <TableRow key={row.id}>
                    {row.getVisibleCells().map((cell) => (
                      <TableCell key={cell.id} className="px-3 py-3 align-top">
                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                      </TableCell>
                    ))}
                  </TableRow>
                ))
              ) : (
                <TableRow>
                  <TableCell
                    colSpan={columns.length}
                    className="px-3 py-14 text-center text-sm text-[var(--sea-ink-soft)]"
                  >
                    {emptyStateMessage ?? 'No documents match the current filters.'}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>

        <div className="flex flex-col gap-3 border-t border-black/5 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="text-sm text-[var(--sea-ink-soft)]">
            Page <span className="font-semibold text-[var(--sea-ink)]">{currentPage}</span> of{' '}
            <span className="font-semibold text-[var(--sea-ink)]">{totalPages}</span>
          </div>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              className="rounded-full border-white/60 bg-white/70"
              disabled={data.offset === 0}
              onClick={() =>
                onSearchChange({
                  ...search,
                  offset: Math.max(0, data.offset - data.limit),
                })
              }
            >
              <ChevronLeft className="size-4" />
              Previous
            </Button>
            <Button
              type="button"
              variant="outline"
              className="rounded-full border-white/60 bg-white/70"
              disabled={data.offset + data.limit >= data.total}
              onClick={() =>
                onSearchChange({
                  ...search,
                  offset: data.offset + data.limit,
                })
              }
            >
              Next
              <ChevronRight className="size-4" />
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function SelectionCheckbox({
  checked,
  indeterminate = false,
  onCheckedChange,
  ariaLabel,
}: {
  checked: boolean
  indeterminate?: boolean
  onCheckedChange: (checked: boolean) => void
  ariaLabel: string
}) {
  const ref = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (ref.current) {
      ref.current.indeterminate = indeterminate
    }
  }, [indeterminate])

  return (
    <input
      ref={ref}
      type="checkbox"
      checked={checked}
      aria-label={ariaLabel}
      onChange={(event) => onCheckedChange(event.target.checked)}
      className="size-4 rounded border border-[rgba(22,74,83,0.25)] accent-[var(--lagoon-deep)]"
    />
  )
}

function SelectionPill({ label, count }: { label: string; count: number }) {
  return (
    <span className="rounded-full border border-white/60 bg-white/70 px-2.5 py-1">
      {label}: <span className="font-semibold text-[var(--sea-ink)]">{count}</span>
    </span>
  )
}

function truncateTitle(value: string, maxChars: number) {
  const chars = Array.from(value)
  if (chars.length <= maxChars) {
    return value
  }
  return `${chars.slice(0, maxChars).join('')}…`
}
