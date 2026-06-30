import { describe, expect, it } from 'vitest'
import {
  buildHighlightRenderModel,
  groupRedactionsForReview,
  sortRedactionsForReview,
} from '#/lib/redaction-review'
import type { Redaction } from '#/lib/schemas'

describe('buildHighlightRenderModel', () => {
  it('slices highlighted spans by Unicode code points', () => {
    const text = 'Intro मे PERSON done'
    const redaction: Redaction = {
      id: 'red_1',
      document_id: 'doc_1',
      start: 9,
      end: 15,
      text: 'PERSON',
      type: 'PERSON',
      confidence: 0.98,
      reason: 'synthetic',
      source: 'synthetic_injection',
      suggested_status: 'ACCEPTED',
      is_ground_truth: true,
      review_state: 'ACCEPTED',
      reviewed_at: null,
      reviewed_by: null,
      is_user_added: false,
      created_at: null,
    }

    const model = buildHighlightRenderModel(text, [redaction])
    const highlight = model.segments.find((segment) => segment.kind === 'highlight')

    expect(highlight?.kind).toBe('highlight')
    expect(highlight?.text).toBe('PERSON')
    expect(model.overlappingRedactionIDs.size).toBe(0)
  })

  it('marks overlapping spans and keeps the first valid highlight only', () => {
    const text = 'abcdefghi'
    const redactions: Redaction[] = [
      {
        id: 'red_1',
        document_id: 'doc_1',
        start: 2,
        end: 6,
        text: 'cdef',
        type: 'PERSON',
        confidence: 0.95,
        reason: 'synthetic',
        source: 'synthetic_injection',
        suggested_status: 'ACCEPTED',
        is_ground_truth: true,
        review_state: 'ACCEPTED',
        reviewed_at: null,
        reviewed_by: null,
        is_user_added: false,
        created_at: null,
      },
      {
        id: 'red_2',
        document_id: 'doc_1',
        start: 4,
        end: 7,
        text: 'efg',
        type: 'EMAIL',
        confidence: 0.56,
        reason: 'overlap',
        source: 'regex_candidate',
        suggested_status: 'REVIEW',
        is_ground_truth: false,
        review_state: 'PENDING',
        reviewed_at: null,
        reviewed_by: null,
        is_user_added: false,
        created_at: null,
      },
    ]

    const model = buildHighlightRenderModel(text, redactions)
    const highlights = model.segments.filter((segment) => segment.kind === 'highlight')

    expect(highlights).toHaveLength(1)
    expect(highlights[0]?.kind === 'highlight' ? highlights[0].redaction.id : '').toBe(
      'red_1',
    )
    expect(model.overlappingRedactionIDs.has('red_2')).toBe(true)
  })

  it('sorts review cards by review state priority and pending-source priority', () => {
    const redactions: Redaction[] = [
      {
        id: 'accepted',
        document_id: 'doc_1',
        start: 12,
        end: 18,
        text: 'accept',
        type: 'PERSON',
        confidence: 0.9,
        reason: 'accepted',
        source: 'synthetic_injection',
        suggested_status: 'ACCEPTED',
        is_ground_truth: true,
        review_state: 'ACCEPTED',
        reviewed_at: null,
        reviewed_by: null,
        is_user_added: false,
        created_at: null,
      },
      {
        id: 'missed',
        document_id: 'doc_1',
        start: 0,
        end: 5,
        text: 'missed',
        type: 'PERSON',
        confidence: 0.3,
        reason: 'missed',
        source: 'controlled_missed_pii',
        suggested_status: 'REVIEW',
        is_ground_truth: false,
        review_state: 'PENDING',
        reviewed_at: null,
        reviewed_by: null,
        is_user_added: false,
        created_at: null,
      },
      {
        id: 'added',
        document_id: 'doc_1',
        start: 20,
        end: 25,
        text: 'added',
        type: 'EMAIL',
        confidence: 1,
        reason: 'user added',
        source: 'user_added',
        suggested_status: 'USER_ADDED',
        is_ground_truth: false,
        review_state: 'ADDED',
        reviewed_at: null,
        reviewed_by: null,
        is_user_added: true,
        created_at: '2026-06-30T00:00:00Z',
      },
    ]

    const sorted = [...redactions].sort(sortRedactionsForReview)
    expect(sorted.map((item) => item.id)).toEqual(['missed', 'added', 'accepted'])
  })

  it('groups duplicate terms by type and source for one-click review', () => {
    const redactions: Redaction[] = [
      {
        id: 'gliner_1',
        document_id: 'doc_1',
        start: 0,
        end: 9,
        text: 'Purchaser',
        type: 'PERSON',
        confidence: 0.81,
        reason: 'Detected person via local GLiNER sidecar',
        source: 'gliner_local',
        suggested_status: 'PENDING',
        is_ground_truth: false,
        review_state: 'PENDING',
        reviewed_at: null,
        reviewed_by: null,
        is_user_added: false,
        created_at: null,
      },
      {
        id: 'gliner_2',
        document_id: 'doc_1',
        start: 22,
        end: 31,
        text: 'Purchaser',
        type: 'PERSON',
        confidence: 0.79,
        reason: 'Detected person via local GLiNER sidecar',
        source: 'gliner_local',
        suggested_status: 'PENDING',
        is_ground_truth: false,
        review_state: 'PENDING',
        reviewed_at: null,
        reviewed_by: null,
        is_user_added: false,
        created_at: null,
      },
      {
        id: 'regex_1',
        document_id: 'doc_1',
        start: 40,
        end: 49,
        text: 'Purchaser',
        type: 'PERSON',
        confidence: null,
        reason: 'Runtime regex',
        source: 'regex_candidate',
        suggested_status: 'PENDING',
        is_ground_truth: false,
        review_state: 'PENDING',
        reviewed_at: null,
        reviewed_by: null,
        is_user_added: false,
        created_at: null,
      },
    ]

    const groups = groupRedactionsForReview(redactions)

    expect(groups).toHaveLength(2)
    expect(groups[0]?.redactions.map((item) => item.id)).toEqual(['gliner_1', 'gliner_2'])
    expect(groups[0]?.reviewStates.PENDING).toBe(2)
    expect(groups[1]?.redactions.map((item) => item.id)).toEqual(['regex_1'])
  })
})
