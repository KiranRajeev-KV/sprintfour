import type { PIIType, Redaction, ReviewState } from '#/lib/schemas'

export type RenderableSegment =
  | {
      kind: 'text'
      key: string
      text: string
      start: number
      end: number
    }
  | {
      kind: 'highlight'
      key: string
      text: string
      start: number
      end: number
      redaction: Redaction
      hasConflict: boolean
    }

export type HighlightRenderModel = {
  segments: RenderableSegment[]
  overlappingRedactionIDs: Set<string>
}

export type RedactionTone =
  | 'accepted'
  | 'added'
  | 'pending'
  | 'false_positive'
  | 'missed'
  | 'rejected'

export type SelectedDocumentRange = {
  start: number
  end: number
  text: string
}

export type RedactionGroup = {
  key: string
  text: string
  normalizedText: string
  type: Redaction['type']
  source: string
  reason: string
  reviewStates: Record<ReviewState, number>
  redactions: Redaction[]
  representative: Redaction
  minStart: number
  maxEnd: number
  maxConfidence: number | null
}

export const REVIEW_STATE_ORDER: Record<ReviewState, number> = {
  PENDING: 0,
  ADDED: 1,
  ACCEPTED: 2,
  REJECTED: 3,
}

export const PENDING_SOURCE_PRIORITY: Record<string, number> = {
  controlled_missed_pii: 0,
  controlled_false_positive: 1,
  gliner_local: 2,
  regex_candidate: 3,
  synthetic_injection: 4,
  user_added: 5,
}

export const PII_TYPE_OPTIONS: PIIType[] = [
  'PERSON',
  'EMAIL',
  'PHONE',
  'ADDRESS',
  'CASE_ID',
  'CLIENT_ID',
  'BANK_ACCOUNT',
  'ROUTING_NUMBER',
  'PAN_LIKE_ID',
  'DOB',
  'DATE_OF_BIRTH',
  'ORGANIZATION_CONTACT',
  'SSN',
  'EIN',
  'ITIN',
  'CREDIT_CARD',
  'US_PHONE',
  'MAC_ADDRESS',
  'IP_ADDRESS',
  'IBAN',
  'SWIFT_BIC',
  'AADHAAR',
  'MRN',
  'PATIENT_ID',
  'PASSPORT',
  'NPI',
  'DEA',
  'API_KEY',
  'US_DRIVER_LICENSE',
  'MEDICAL_LICENSE',
  'DOMAIN_NAME',
  'URL',
]

export function buildHighlightRenderModel(
  text: string,
  redactions: Redaction[],
): HighlightRenderModel {
  const codePoints = Array.from(text)
  const sorted = [...redactions].sort((left, right) => {
    if (left.start === right.start) {
      return right.end - left.end
    }
    return left.start - right.start
  })

  const segments: RenderableSegment[] = []
  const overlappingRedactionIDs = new Set<string>()
  let cursor = 0

  for (const redaction of sorted) {
    if (
      redaction.start < 0 ||
      redaction.end <= redaction.start ||
      redaction.end > codePoints.length
    ) {
      overlappingRedactionIDs.add(redaction.id)
      continue
    }

    if (redaction.start < cursor) {
      overlappingRedactionIDs.add(redaction.id)
      continue
    }

    if (cursor < redaction.start) {
      segments.push({
        kind: 'text',
        key: `text-${cursor}-${redaction.start}`,
        text: codePoints.slice(cursor, redaction.start).join(''),
        start: cursor,
        end: redaction.start,
      })
    }

    segments.push({
      kind: 'highlight',
      key: redaction.id,
      text: codePoints.slice(redaction.start, redaction.end).join(''),
      start: redaction.start,
      end: redaction.end,
      redaction,
      hasConflict: false,
    })
    cursor = redaction.end
  }

  if (cursor < codePoints.length) {
    segments.push({
      kind: 'text',
      key: `text-${cursor}-${codePoints.length}`,
      text: codePoints.slice(cursor).join(''),
      start: cursor,
      end: codePoints.length,
    })
  }

  return { segments, overlappingRedactionIDs }
}

export function getRedactionTone(redaction: Redaction): RedactionTone {
  if (redaction.review_state === 'REJECTED') {
    return 'rejected'
  }
  if (redaction.review_state === 'ADDED') {
    return 'added'
  }
  if (redaction.review_state === 'ACCEPTED') {
    return 'accepted'
  }
  if (redaction.source === 'controlled_missed_pii') {
    return 'missed'
  }
  if (redaction.source === 'controlled_false_positive') {
    return 'false_positive'
  }
  return 'pending'
}

export function sortRedactionsForReview(left: Redaction, right: Redaction) {
  const reviewStateDelta =
    REVIEW_STATE_ORDER[left.review_state] - REVIEW_STATE_ORDER[right.review_state]
  if (reviewStateDelta !== 0) {
    return reviewStateDelta
  }

  if (left.review_state === 'PENDING' && right.review_state === 'PENDING') {
    const leftPriority = PENDING_SOURCE_PRIORITY[left.source] ?? 99
    const rightPriority = PENDING_SOURCE_PRIORITY[right.source] ?? 99
    if (leftPriority !== rightPriority) {
      return leftPriority - rightPriority
    }
  }

  if (left.start === right.start) {
    return right.end - left.end
  }
  return left.start - right.start
}

export function groupRedactionsForReview(redactions: Redaction[]): RedactionGroup[] {
  const grouped = new Map<string, RedactionGroup>()

  for (const redaction of redactions) {
    const normalizedText = normalizeGroupingText(redaction.text)
    const key = `${redaction.type}::${redaction.source}::${normalizedText}`
    const existing = grouped.get(key)
    if (existing) {
      existing.redactions.push(redaction)
      existing.reviewStates[redaction.review_state] += 1
      existing.minStart = Math.min(existing.minStart, redaction.start)
      existing.maxEnd = Math.max(existing.maxEnd, redaction.end)
      if (
        redaction.confidence != null &&
        (existing.maxConfidence == null || redaction.confidence > existing.maxConfidence)
      ) {
        existing.maxConfidence = redaction.confidence
      }
      continue
    }

    grouped.set(key, {
      key,
      text: redaction.text,
      normalizedText,
      type: redaction.type,
      source: redaction.source,
      reason: redaction.reason,
      reviewStates: {
        PENDING: redaction.review_state === 'PENDING' ? 1 : 0,
        ACCEPTED: redaction.review_state === 'ACCEPTED' ? 1 : 0,
        REJECTED: redaction.review_state === 'REJECTED' ? 1 : 0,
        ADDED: redaction.review_state === 'ADDED' ? 1 : 0,
      },
      redactions: [redaction],
      representative: redaction,
      minStart: redaction.start,
      maxEnd: redaction.end,
      maxConfidence: redaction.confidence ?? null,
    })
  }

  return Array.from(grouped.values()).sort((left, right) => {
    const leftState = primaryGroupReviewState(left)
    const rightState = primaryGroupReviewState(right)
    const groupReviewDelta = REVIEW_STATE_ORDER[leftState] - REVIEW_STATE_ORDER[rightState]
    if (groupReviewDelta !== 0) {
      return groupReviewDelta
    }

    if (leftState === 'PENDING' && rightState === 'PENDING') {
      const leftPriority = PENDING_SOURCE_PRIORITY[left.source] ?? 99
      const rightPriority = PENDING_SOURCE_PRIORITY[right.source] ?? 99
      if (leftPriority !== rightPriority) {
        return leftPriority - rightPriority
      }
    }

    if (left.minStart === right.minStart) {
      return left.maxEnd - right.maxEnd
    }
    return left.minStart - right.minStart
  })
}

export function primaryGroupReviewState(group: RedactionGroup): ReviewState {
  if (group.reviewStates.PENDING > 0) {
    return 'PENDING'
  }
  if (group.reviewStates.ADDED > 0) {
    return 'ADDED'
  }
  if (group.reviewStates.ACCEPTED > 0) {
    return 'ACCEPTED'
  }
  return 'REJECTED'
}

function normalizeGroupingText(value: string) {
  return value.trim().replace(/\s+/g, ' ').toLocaleLowerCase()
}

export function truncatePreview(value: string, maxChars: number) {
  const chars = Array.from(value)
  if (chars.length <= maxChars) {
    return value
  }
  return `${chars.slice(0, maxChars).join('')}…`
}

export function getSelectedDocumentRange(
  root: HTMLElement,
  text: string,
  selection: Selection | null,
): SelectedDocumentRange | null {
  if (!selection || selection.rangeCount === 0 || selection.isCollapsed) {
    return null
  }

  const range = selection.getRangeAt(0)
  if (!root.contains(range.commonAncestorContainer)) {
    return null
  }

  const start = resolveBoundaryOffset(root, range.startContainer, range.startOffset, false)
  const end = resolveBoundaryOffset(root, range.endContainer, range.endOffset, true)
  if (start == null || end == null || start === end) {
    return null
  }

  const normalizedStart = Math.min(start, end)
  const normalizedEnd = Math.max(start, end)
  const selectedText = Array.from(text).slice(normalizedStart, normalizedEnd).join('')
  if (!selectedText.trim()) {
    return null
  }

  return {
    start: normalizedStart,
    end: normalizedEnd,
    text: selectedText,
  }
}

function resolveBoundaryOffset(
  root: HTMLElement,
  container: Node,
  offset: number,
  isEndBoundary: boolean,
) {
  if (container.nodeType === Node.TEXT_NODE) {
    const textNode = container as Text
    const segment = closestSegmentElement(textNode.parentElement)
    if (!segment) {
      return null
    }
    const segmentStart = Number(segment.dataset.segmentStart)
    const segmentText = textNode.data
    return segmentStart + countCodePoints(segmentText.slice(0, offset))
  }

  if (container.nodeType === Node.ELEMENT_NODE) {
    const element = container as Element
    const children = Array.from(element.childNodes)
    if (children.length === 0) {
      const segment = closestSegmentElement(element)
      if (!segment) {
        return null
      }
      return isEndBoundary
        ? Number(segment.dataset.segmentEnd)
        : Number(segment.dataset.segmentStart)
    }

    if (offset < children.length) {
      const target = findBoundarySegment(children[offset]!, 'start')
      if (target) {
        return Number(target.dataset.segmentStart)
      }
    }

    if (offset > 0) {
      const target = findBoundarySegment(children[offset - 1]!, 'end')
      if (target) {
        return Number(target.dataset.segmentEnd)
      }
    }

    if (element === root) {
      return isEndBoundary ? Array.from(root.textContent ?? '').length : 0
    }

    const segment = closestSegmentElement(element)
    if (!segment) {
      return null
    }
    return isEndBoundary
      ? Number(segment.dataset.segmentEnd)
      : Number(segment.dataset.segmentStart)
  }

  return null
}

function closestSegmentElement(node: Element | null) {
  return node?.closest<HTMLElement>('[data-segment-start][data-segment-end]') ?? null
}

function findBoundarySegment(node: Node, edge: 'start' | 'end') {
  if (node.nodeType === Node.TEXT_NODE) {
    return closestSegmentElement(node.parentElement)
  }

  if (node.nodeType !== Node.ELEMENT_NODE) {
    return null
  }

  const element = node as HTMLElement
  if (element.matches('[data-segment-start][data-segment-end]')) {
    return element
  }

  const walker = document.createTreeWalker(element, NodeFilter.SHOW_ELEMENT)
  const matches: HTMLElement[] = []
  let current = walker.nextNode()
  while (current) {
    if (
      current instanceof HTMLElement &&
      current.matches('[data-segment-start][data-segment-end]')
    ) {
      matches.push(current)
    }
    current = walker.nextNode()
  }

  if (matches.length === 0) {
    return null
  }
  return edge === 'start' ? matches[0]! : matches[matches.length - 1]!
}

function countCodePoints(value: string) {
  return Array.from(value).length
}
