package store

import (
	doc "backend/internal/document"
	"fmt"
)

func (s *Store) SetDocumentProcessing(documentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.documentsByID[documentID]; !ok {
		return ErrDocumentNotFound
	}

	state := s.runtimeByDocID[documentID]
	state.Status = "PROCESSING"
	s.runtimeByDocID[documentID] = state
	return nil
}

func (s *Store) SetDocumentProcessed(documentID string, detections []RuntimeDetection) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	storedDoc, ok := s.documentsByID[documentID]
	if !ok {
		return ErrDocumentNotFound
	}

	redactions := make([]*Redaction, 0, len(detections))
	for _, d := range detections {
		s.generatedRedactionSeq++
		redactions = append(redactions, &Redaction{
			ID:              doc.FormatGeneratedRedactionID(s.generatedRedactionSeq),
			DocumentID:      documentID,
			Start:           d.Start,
			End:             d.End,
			Text:            d.Text,
			Type:            d.Type,
			Confidence:      d.Confidence,
			Reason:          d.Reason,
			Source:          d.Source,
			SuggestedStatus: d.SuggestedStatus,
			IsGroundTruth:   false,
		})
	}

	status, risk := doc.ClassifyUploadedDocument(redactions)
	storedDoc.RiskLevel = risk
	storedDoc.RedactionCount = len(redactions)
	storedDoc.PIICount = len(redactions)
	storedDoc.LowConfidenceCount = 0
	for _, r := range redactions {
		if r.Confidence != nil && *r.Confidence < lowConfidenceThreshold {
			storedDoc.LowConfidenceCount++
		}
	}

	s.redactionsByDoc[documentID] = append(s.redactionsByDoc[documentID], redactions...)
	for _, r := range redactions {
		s.redactionsByID[r.ID] = r
		s.redactionRuntimeByID[r.ID] = initialRedactionRuntimeState(r)
	}

	state := s.runtimeByDocID[documentID]
	state.Status = status
	state.FailureHint = nil
	s.runtimeByDocID[documentID] = state

	return nil
}

func (s *Store) SetDocumentFailed(documentID string, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	document, ok := s.documentsByID[documentID]
	if !ok {
		return
	}

	document.RiskLevel = "HIGH"

	state := s.runtimeByDocID[documentID]
	state.Status = "FAILED"
	state.FailureHint = &errMsg
	s.runtimeByDocID[documentID] = state
}

func (s *Store) ApproveDocument(documentID string) (DocumentMutationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	document, state, err := s.documentStateLocked(documentID)
	if err != nil {
		return DocumentMutationResult{}, err
	}

	result := DocumentMutationResult{
		DocumentID:     document.ID,
		PreviousStatus: state.Status,
		Status:         state.Status,
		RetryCount:     state.RetryCount,
	}

	switch state.Status {
	case "QUEUED":
		return DocumentMutationResult{}, &StateConflictError{
			Code:    "not_processed",
			Message: "document is still queued for processing",
		}
	case "PROCESSING":
		return DocumentMutationResult{}, &StateConflictError{
			Code:    "not_processed",
			Message: "document is still being processed",
		}
	case "FAILED":
		return DocumentMutationResult{}, &StateConflictError{
			Code:    "failed_requires_retry",
			Message: "failed documents must be retried before approval",
		}
	case "EXPORTED", "APPROVED":
		return result, nil
	case "READY", "CLEAN", "NEEDS_REVIEW":
		if !s.canApproveLocked(document.ID, state.Status) {
			return DocumentMutationResult{}, &StateConflictError{
				Code:    "unresolved_review_items",
				Message: "document has unresolved redaction review items",
			}
		}
		state.Status = "APPROVED"
		s.runtimeByDocID[document.ID] = state
		result.Status = state.Status
		result.Changed = true
		return result, nil
	default:
		return DocumentMutationResult{}, &StateConflictError{
			Code:    "invalid_state",
			Message: fmt.Sprintf("document status %q cannot be approved", state.Status),
		}
	}
}

func (s *Store) BulkApprove(documentIDs []string) BulkMutationResponse {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]DocumentMutationResult, 0, len(documentIDs))
	approvedCount := 0

	for _, documentID := range documentIDs {
		item, changed := s.bulkApproveDocumentLocked(documentID)
		items = append(items, item)
		if changed {
			approvedCount++
		}
	}

	return BulkMutationResponse{
		Requested: len(documentIDs),
		Approved:  approvedCount,
		Skipped:   len(documentIDs) - approvedCount,
		Items:     items,
	}
}

func (s *Store) RetryDocument(documentID string) (DocumentMutationResult, error) {
	s.mu.Lock()

	document, state, err := s.documentStateLocked(documentID)
	if err != nil {
		s.mu.Unlock()
		return DocumentMutationResult{}, err
	}

	result := DocumentMutationResult{
		DocumentID:     document.ID,
		PreviousStatus: state.Status,
		Status:         state.Status,
		RetryCount:     state.RetryCount,
	}

	if state.Status != "FAILED" {
		s.mu.Unlock()
		return result, nil
	}

	state.Status = "QUEUED"
	state.RetryCount++
	state.FailureHint = nil
	s.runtimeByDocID[document.ID] = state
	queued := queuedDocument{documentID: document.ID, text: document.Text}
	s.mu.Unlock()

	result.Status = state.Status
	result.RetryCount = state.RetryCount
	result.Changed = true
	s.submitQueuedDocuments([]queuedDocument{queued})
	return result, nil
}

func (s *Store) BulkRetry(documentIDs []string) BulkMutationResponse {
	s.mu.Lock()

	items := make([]DocumentMutationResult, 0, len(documentIDs))
	retriedCount := 0
	queuedDocs := make([]queuedDocument, 0, len(documentIDs))

	for _, documentID := range documentIDs {
		item, queued, changed := s.bulkRetryDocumentLocked(documentID)
		items = append(items, item)
		if changed {
			retriedCount++
			queuedDocs = append(queuedDocs, queued)
		}
	}
	s.mu.Unlock()

	s.submitQueuedDocuments(queuedDocs)

	return BulkMutationResponse{
		Requested: len(documentIDs),
		Retried:   retriedCount,
		Skipped:   len(documentIDs) - retriedCount,
		Items:     items,
	}
}

func (s *Store) bulkApproveDocumentLocked(documentID string) (DocumentMutationResult, bool) {
	document, state, err := s.documentStateLocked(documentID)
	if err != nil {
		return DocumentMutationResult{
			DocumentID: documentID,
			Changed:    false,
			Reason:     "not_found",
		}, false
	}

	item := DocumentMutationResult{
		DocumentID:     document.ID,
		PreviousStatus: state.Status,
		Status:         state.Status,
		RetryCount:     state.RetryCount,
		Changed:        false,
	}

	switch state.Status {
	case "READY", "CLEAN":
		if s.blockingReviewCountLocked(document.ID) > 0 {
			item.Reason = "blocking_review_items"
			return item, false
		}
		state.Status = "APPROVED"
		s.runtimeByDocID[document.ID] = state
		item.Status = state.Status
		item.Changed = true
		item.Reason = "approved"
		return item, true
	case "APPROVED":
		item.Reason = "already_approved"
	case "EXPORTED":
		item.Reason = "already_exported"
	case "NEEDS_REVIEW":
		item.Reason = "needs_review_not_bulk_approved"
	case "FAILED":
		item.Reason = "failed_not_approvable"
	default:
		item.Reason = "unsupported_status"
	}

	return item, false
}

func (s *Store) bulkRetryDocumentLocked(documentID string) (DocumentMutationResult, queuedDocument, bool) {
	document, state, err := s.documentStateLocked(documentID)
	if err != nil {
		return DocumentMutationResult{
			DocumentID: documentID,
			Changed:    false,
			Reason:     "not_found",
		}, queuedDocument{}, false
	}

	item := DocumentMutationResult{
		DocumentID:     document.ID,
		PreviousStatus: state.Status,
		Status:         state.Status,
		RetryCount:     state.RetryCount,
		Changed:        false,
	}

	if state.Status != "FAILED" {
		item.Reason = "not_failed"
		return item, queuedDocument{}, false
	}

	state.Status = "QUEUED"
	state.RetryCount++
	state.FailureHint = nil
	s.runtimeByDocID[document.ID] = state

	item.Status = state.Status
	item.RetryCount = state.RetryCount
	item.Changed = true
	item.Reason = "retried"
	return item, queuedDocument{
		documentID: document.ID,
		text:       document.Text,
	}, true
}
