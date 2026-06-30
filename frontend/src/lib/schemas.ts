import { z } from 'zod'

export const documentStatusSchema = z.enum([
  'READY',
  'NEEDS_REVIEW',
  'FAILED',
  'CLEAN',
])

export const riskLevelSchema = z.enum(['LOW', 'MEDIUM', 'HIGH', 'UNKNOWN'])

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
})

export const redactionSchema = z.object({
  id: z.string().min(1),
  document_id: z.string().min(1),
  start: z.number().int().nonnegative(),
  end: z.number().int().nonnegative(),
  text: z.string(),
  type: z.string(),
  confidence: z.number().min(0).max(1),
  reason: z.string(),
  source: z.string(),
  suggested_status: z.string(),
  is_ground_truth: z.boolean(),
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
