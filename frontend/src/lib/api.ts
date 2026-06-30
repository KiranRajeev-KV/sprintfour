import { queryOptions } from '@tanstack/react-query'
import {
  apiErrorSchema,
  batchSummarySchema,
  bulkMutationResponseSchema,
  documentDetailSchema,
  documentListResponseSchema,
  documentMutationResultSchema,
  documentSearchSchema,
  exportSummarySchema,
  latestExportResponseSchema,
  piiTypeSchema,
  redactionMutationResultSchema,
  redactionSchema,
  redactionsResponseSchema,
  reviewSummarySchema,
  uploadBatchResponseSchema,
  type DocumentStatus,
  type PIIType,
  type RiskLevel,
} from '#/lib/schemas'

const configuredAPIBaseURL = import.meta.env.VITE_API_BASE_URL?.trim()
const API_BASE_URL =
  configuredAPIBaseURL || (import.meta.env.SSR ? 'http://127.0.0.1:8080' : '')

export class ApiError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

export type DocumentsSearch = {
  status?: DocumentStatus
  risk?: RiskLevel
  q?: string
  limit: number
  offset: number
}

async function requestJSON<T>(
  path: string,
  schema: { parse: (value: unknown) => T },
  init?: RequestInit,
) {
  const response = await fetch(`${API_BASE_URL}${path}`, init)
  const payload = await response.json().catch(() => null)

  if (!response.ok) {
    const parsedError = apiErrorSchema.safeParse(payload)
    if (parsedError.success) {
      throw new ApiError(
        response.status,
        parsedError.data.error.code,
        parsedError.data.error.message,
      )
    }

    throw new ApiError(response.status, 'http_error', 'request failed')
  }

  return schema.parse(payload)
}

function buildDocumentsQueryString(search: DocumentsSearch) {
  const parsed = documentSearchSchema.parse(search)
  const params = new URLSearchParams()
  if (parsed.status) {
    params.set('status', parsed.status)
  }
  if (parsed.risk) {
    params.set('risk', parsed.risk)
  }
  if (parsed.q?.trim()) {
    params.set('q', parsed.q.trim())
  }
  params.set('limit', String(parsed.limit))
  params.set('offset', String(parsed.offset))
  return params.toString()
}

export function batchSummaryQueryOptions() {
  return queryOptions({
    queryKey: ['batch-summary'],
    queryFn: () => requestJSON('/api/batch/summary', batchSummarySchema),
  })
}

export function documentsQueryOptions(search: DocumentsSearch) {
  const queryString = buildDocumentsQueryString(search)
  return queryOptions({
    queryKey: ['documents', queryString],
    queryFn: () =>
      requestJSON(`/api/documents?${queryString}`, documentListResponseSchema),
  })
}

export function documentDetailQueryOptions(documentId: string) {
  return queryOptions({
    queryKey: ['document', documentId],
    queryFn: () =>
      requestJSON(`/api/documents/${documentId}`, documentDetailSchema),
  })
}

export function documentRedactionsQueryOptions(documentId: string) {
  return queryOptions({
    queryKey: ['document-redactions', documentId],
    queryFn: () =>
      requestJSON(
        `/api/documents/${documentId}/redactions`,
        redactionsResponseSchema,
      ),
  })
}

export function documentReviewSummaryQueryOptions(documentId: string) {
  return queryOptions({
    queryKey: ['document-review-summary', documentId],
    queryFn: () =>
      requestJSON(
        `/api/documents/${documentId}/review-summary`,
        reviewSummarySchema,
      ),
  })
}

export function latestExportQueryOptions() {
  return queryOptions({
    queryKey: ['latest-export'],
    queryFn: () => requestJSON('/api/export/latest', latestExportResponseSchema),
  })
}

function postJSON<T>(
  path: string,
  schema: { parse: (value: unknown) => T },
  body?: unknown,
) {
  return requestJSON(path, schema, {
    method: 'POST',
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}

export function approveDocument(documentId: string) {
  return postJSON(
    `/api/documents/${documentId}/approve`,
    documentMutationResultSchema,
  )
}

export function bulkApproveDocuments(documentIds: string[]) {
  return postJSON('/api/documents/bulk-approve', bulkMutationResponseSchema, {
    document_ids: documentIds,
  })
}

export function retryDocument(documentId: string) {
  return postJSON(
    `/api/documents/${documentId}/retry`,
    documentMutationResultSchema,
  )
}

export function bulkRetryDocuments(documentIds: string[]) {
  return postJSON('/api/documents/bulk-retry', bulkMutationResponseSchema, {
    document_ids: documentIds,
  })
}

export function exportApprovedDocuments() {
  return postJSON('/api/export', exportSummarySchema)
}

export function acceptRedaction(redactionId: string) {
  return postJSON(
    `/api/redactions/${redactionId}/accept`,
    redactionMutationResultSchema,
  )
}

export function rejectRedaction(redactionId: string) {
  return postJSON(
    `/api/redactions/${redactionId}/reject`,
    redactionMutationResultSchema,
  )
}

export function addManualRedaction(input: {
  documentId: string
  start: number
  end: number
  type: PIIType
  reason: string
  selectedText: string
}) {
  return postJSON(
    `/api/documents/${input.documentId}/redactions`,
    redactionSchema,
    {
      start: input.start,
      end: input.end,
      type: piiTypeSchema.parse(input.type),
      reason: input.reason,
      selected_text: input.selectedText,
    },
  )
}

export function uploadDocuments(input: {
  files: File[]
  mode: 'replace' | 'append'
}) {
  const formData = new FormData()
  for (const file of input.files) {
    const relativePath =
      file instanceof File &&
      'webkitRelativePath' in file &&
      typeof file.webkitRelativePath === 'string' &&
      file.webkitRelativePath.trim() !== ''
        ? file.webkitRelativePath
        : file.name
    formData.append('files', file, relativePath)
  }
  formData.append('mode', input.mode)

  return requestJSON('/api/uploads/documents', uploadBatchResponseSchema, {
    method: 'POST',
    body: formData,
  })
}
