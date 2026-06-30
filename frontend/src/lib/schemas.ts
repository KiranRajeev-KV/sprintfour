import { z } from 'zod'

export const documentStatusSchema = z.enum([
  'READY',
  'NEEDS_REVIEW',
  'FAILED',
  'CLEAN',
  'APPROVED',
  'EXPORTED',
])

export const riskLevelSchema = z.enum(['LOW', 'MEDIUM', 'HIGH', 'UNKNOWN'])

export const reviewStateSchema = z.enum([
  'PENDING',
  'ACCEPTED',
  'REJECTED',
  'ADDED',
])

export const piiTypeSchema = z.enum([
  'PERSON',
  'EMAIL',
  'PHONE',
  'ADDRESS',
  'CASE_ID',
  'CLIENT_ID',
  'BANK_ACCOUNT',
  'PAN_LIKE_ID',
  'DATE_OF_BIRTH',
  'ORGANIZATION_CONTACT',
])

export const batchSummarySchema = z.object({
  total_documents: z.number().int().nonnegative(),
  ready: z.number().int().nonnegative(),
  needs_review: z.number().int().nonnegative(),
  failed: z.number().int().nonnegative(),
  clean: z.number().int().nonnegative(),
  approved: z.number().int().nonnegative(),
  exported: z.number().int().nonnegative(),
  total_redactions: z.number().int().nonnegative(),
  synthetic_redactions: z.number().int().nonnegative(),
  regex_candidates: z.number().int().nonnegative(),
  controlled_false_positives: z.number().int().nonnegative(),
  controlled_missed_pii: z.number().int().nonnegative(),
  pending_redactions: z.number().int().nonnegative().optional(),
  accepted_redactions: z.number().int().nonnegative().optional(),
  rejected_redactions: z.number().int().nonnegative().optional(),
  added_redactions: z.number().int().nonnegative().optional(),
  blocking_review_documents: z.number().int().nonnegative().optional(),
})

export const documentListItemSchema = z.object({
  id: z.string().min(1),
  title: z.string(),
  source: z.string(),
  source_file: z.string(),
  status: documentStatusSchema,
  risk_level: riskLevelSchema,
  char_count: z.number().int().nonnegative(),
  pii_count: z.number().int().nonnegative(),
  low_confidence_count: z.number().int().nonnegative(),
  failure_hint: z.string().nullable(),
  retry_count: z.number().int().nonnegative().optional(),
  pending_redaction_count: z.number().int().nonnegative().optional(),
  blocking_review_count: z.number().int().nonnegative().optional(),
  can_approve: z.boolean().optional(),
})

export const documentListResponseSchema = z.object({
  items: z.array(documentListItemSchema),
  total: z.number().int().nonnegative(),
  limit: z.number().int().positive(),
  offset: z.number().int().nonnegative(),
})

export const documentDetailSchema = z.object({
  id: z.string().min(1),
  title: z.string(),
  source: z.string(),
  source_file: z.string(),
  text: z.string(),
  char_count: z.number().int().nonnegative(),
  status: documentStatusSchema,
  risk_level: riskLevelSchema,
  failure_hint: z.string().nullable(),
  redaction_count: z.number().int().nonnegative(),
  low_confidence_count: z.number().int().nonnegative(),
  retry_count: z.number().int().nonnegative(),
  pending_redaction_count: z.number().int().nonnegative().optional(),
  accepted_redaction_count: z.number().int().nonnegative().optional(),
  rejected_redaction_count: z.number().int().nonnegative().optional(),
  added_redaction_count: z.number().int().nonnegative().optional(),
  blocking_review_count: z.number().int().nonnegative().optional(),
  can_approve: z.boolean().optional(),
})

export const redactionSchema = z.object({
  id: z.string().min(1),
  document_id: z.string().min(1),
  start: z.number().int().nonnegative(),
  end: z.number().int().nonnegative(),
  text: z.string(),
  type: piiTypeSchema,
  confidence: z.number().min(0).max(1),
  reason: z.string(),
  source: z.string(),
  suggested_status: z.string(),
  is_ground_truth: z.boolean(),
  review_state: reviewStateSchema,
  reviewed_at: z.string().nullable().optional(),
  reviewed_by: z.string().nullable().optional(),
  is_user_added: z.boolean(),
  created_at: z.string().nullable().optional(),
})

export const redactionsResponseSchema = z.object({
  document_id: z.string().min(1),
  items: z.array(redactionSchema),
  total: z.number().int().nonnegative(),
})

export const apiErrorSchema = z.object({
  error: z.object({
    code: z.string(),
    message: z.string(),
  }),
})

export const documentMutationResultSchema = z.object({
  document_id: z.string().min(1),
  previous_status: documentStatusSchema,
  status: documentStatusSchema,
  changed: z.boolean(),
  retry_count: z.number().int().nonnegative().optional(),
  reason: z.string().optional(),
})

export const bulkMutationResponseSchema = z.object({
  requested: z.number().int().nonnegative(),
  approved: z.number().int().nonnegative().optional(),
  retried: z.number().int().nonnegative().optional(),
  skipped: z.number().int().nonnegative(),
  items: z.array(documentMutationResultSchema),
})

export const exportSummarySchema = z.object({
  export_id: z.string().min(1),
  exported_documents: z.number().int().nonnegative(),
  skipped_documents: z.number().int().nonnegative(),
  needs_review: z.number().int().nonnegative(),
  failed: z.number().int().nonnegative(),
  ready: z.number().int().nonnegative(),
  approved_blocked_by_review: z.number().int().nonnegative().optional(),
  applied_redactions: z.number().int().nonnegative().optional(),
  skipped_rejected_redactions: z.number().int().nonnegative().optional(),
  skipped_pending_redactions: z.number().int().nonnegative().optional(),
  skipped_overlap_redactions: z.number().int().nonnegative().optional(),
  created_at: z.string().min(1),
})

export const latestExportResponseSchema = z.discriminatedUnion('has_export', [
  z.object({
    has_export: z.literal(false),
  }),
  z.object({
    has_export: z.literal(true),
    export_id: z.string().min(1),
    exported_documents: z.number().int().nonnegative(),
    skipped_documents: z.number().int().nonnegative(),
    needs_review: z.number().int().nonnegative(),
    failed: z.number().int().nonnegative(),
    ready: z.number().int().nonnegative(),
    approved_blocked_by_review: z.number().int().nonnegative().optional(),
    applied_redactions: z.number().int().nonnegative().optional(),
    skipped_rejected_redactions: z.number().int().nonnegative().optional(),
    skipped_pending_redactions: z.number().int().nonnegative().optional(),
    skipped_overlap_redactions: z.number().int().nonnegative().optional(),
    created_at: z.string().min(1),
  }),
])

export const reviewSummarySchema = z.object({
  document_id: z.string().min(1),
  status: documentStatusSchema,
  risk_level: riskLevelSchema,
  total_redactions: z.number().int().nonnegative(),
  pending: z.number().int().nonnegative(),
  accepted: z.number().int().nonnegative(),
  rejected: z.number().int().nonnegative(),
  added: z.number().int().nonnegative(),
  low_confidence: z.number().int().nonnegative(),
  regex_candidates: z.number().int().nonnegative(),
  controlled_false_positives: z.number().int().nonnegative(),
  controlled_missed_pii: z.number().int().nonnegative(),
  blocking_review_items: z.number().int().nonnegative(),
  can_approve: z.boolean(),
})

export const redactionMutationResultSchema = z.object({
  redaction_id: z.string().min(1),
  document_id: z.string().min(1),
  previous_state: reviewStateSchema,
  review_state: reviewStateSchema,
  changed: z.boolean(),
})

export const uploadItemResultSchema = z.object({
  filename: z.string(),
  relative_path: z.string().nullable().optional(),
  document_id: z.string().nullable().optional(),
  status: documentStatusSchema.nullable().optional(),
  risk_level: riskLevelSchema.nullable().optional(),
  redaction_count: z.number().int().nonnegative(),
  accepted: z.boolean(),
  reason: z.string(),
})

export const uploadBatchResponseSchema = z.object({
  batch_id: z.string().min(1),
  mode: z.enum(['replace', 'append']),
  uploaded: z.number().int().nonnegative(),
  accepted: z.number().int().nonnegative(),
  rejected: z.number().int().nonnegative(),
  documents_created: z.number().int().nonnegative(),
  redactions_created: z.number().int().nonnegative(),
  items: z.array(uploadItemResultSchema),
})

export const documentSearchSchema = z.object({
  status: documentStatusSchema.optional(),
  risk: riskLevelSchema.optional(),
  q: z.string().optional(),
  limit: z.number().int().positive().max(100),
  offset: z.number().int().nonnegative(),
})

export type BatchSummary = z.infer<typeof batchSummarySchema>
export type DocumentListItem = z.infer<typeof documentListItemSchema>
export type DocumentListResponse = z.infer<typeof documentListResponseSchema>
export type DocumentDetail = z.infer<typeof documentDetailSchema>
export type Redaction = z.infer<typeof redactionSchema>
export type RedactionsResponse = z.infer<typeof redactionsResponseSchema>
export type DocumentStatus = z.infer<typeof documentStatusSchema>
export type RiskLevel = z.infer<typeof riskLevelSchema>
export type ReviewState = z.infer<typeof reviewStateSchema>
export type PIIType = z.infer<typeof piiTypeSchema>
export type DocumentMutationResult = z.infer<typeof documentMutationResultSchema>
export type BulkMutationResponse = z.infer<typeof bulkMutationResponseSchema>
export type ExportSummary = z.infer<typeof exportSummarySchema>
export type LatestExportResponse = z.infer<typeof latestExportResponseSchema>
export type ReviewSummary = z.infer<typeof reviewSummarySchema>
export type RedactionMutationResult = z.infer<typeof redactionMutationResultSchema>
export type UploadBatchResponse = z.infer<typeof uploadBatchResponseSchema>
export type UploadItemResult = z.infer<typeof uploadItemResultSchema>
