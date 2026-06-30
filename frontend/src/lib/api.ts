import { queryOptions } from '@tanstack/react-query'
import {
  apiErrorSchema,
  batchSummarySchema,
  documentDetailSchema,
  documentListResponseSchema,
  documentSearchSchema,
  redactionsResponseSchema,
  type DocumentStatus,
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

async function fetchJSON<T>(
  path: string,
  schema: { parse: (value: unknown) => T },
) {
  const response = await fetch(`${API_BASE_URL}${path}`)
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
    queryFn: () => fetchJSON('/api/batch/summary', batchSummarySchema),
  })
}

export function documentsQueryOptions(search: DocumentsSearch) {
  const queryString = buildDocumentsQueryString(search)
  return queryOptions({
    queryKey: ['documents', queryString],
    queryFn: () =>
      fetchJSON(`/api/documents?${queryString}`, documentListResponseSchema),
  })
}

export function documentDetailQueryOptions(documentId: string) {
  return queryOptions({
    queryKey: ['document', documentId],
    queryFn: () =>
      fetchJSON(`/api/documents/${documentId}`, documentDetailSchema),
  })
}

export function documentRedactionsQueryOptions(documentId: string) {
  return queryOptions({
    queryKey: ['document-redactions', documentId],
    queryFn: () =>
      fetchJSON(
        `/api/documents/${documentId}/redactions`,
        redactionsResponseSchema,
      ),
  })
}
