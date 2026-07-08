package store

import (
	doc "backend/internal/document"
	"strings"
	"time"
)

func (s *Store) AcceptRedaction(redactionID string, now time.Time) (RedactionMutationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.mutateRedactionLocked(redactionID, "accept", now)
}

func (s *Store) RejectRedaction(redactionID string, now time.Time) (RedactionMutationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.mutateRedactionLocked(redactionID, "reject", now)
}

func (s *Store) BulkAcceptRedactions(redactionIDs []string, now time.Time) (BulkRedactionMutationResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]RedactionMutationResult, 0, len(redactionIDs))
	acceptedCount := 0

	for _, redactionID := range redactionIDs {
		result, err := s.mutateRedactionLocked(redactionID, "accept", now)
		if err != nil {
			return BulkRedactionMutationResponse{}, err
		}
		items = append(items, result)
		if result.Changed {
			acceptedCount++
		}
	}

	return BulkRedactionMutationResponse{
		Requested: len(redactionIDs),
		Accepted:  acceptedCount,
		Skipped:   len(redactionIDs) - acceptedCount,
		Items:     items,
	}, nil
}

func (s *Store) BulkRejectRedactions(redactionIDs []string, now time.Time) (BulkRedactionMutationResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]RedactionMutationResult, 0, len(redactionIDs))
	rejectedCount := 0

	for _, redactionID := range redactionIDs {
		result, err := s.mutateRedactionLocked(redactionID, "reject", now)
		if err != nil {
			return BulkRedactionMutationResponse{}, err
		}
		items = append(items, result)
		if result.Changed {
			rejectedCount++
		}
	}

	return BulkRedactionMutationResponse{
		Requested: len(redactionIDs),
		Rejected:  rejectedCount,
		Skipped:   len(redactionIDs) - rejectedCount,
		Items:     items,
	}, nil
}

func (s *Store) mutateRedactionLocked(redactionID string, action string, now time.Time) (RedactionMutationResult, error) {
	redaction, runtime, err := s.redactionStateLocked(redactionID)
	if err != nil {
		return RedactionMutationResult{}, err
	}

	result := RedactionMutationResult{
		RedactionID:   redaction.ID,
		DocumentID:    redaction.DocumentID,
		PreviousState: runtime.ReviewState,
		ReviewState:   runtime.ReviewState,
	}

	switch action {
	case "accept":
		switch runtime.ReviewState {
		case "ACCEPTED", "ADDED":
			return result, nil
		case "PENDING", "REJECTED":
			runtime.ReviewState = "ACCEPTED"
		default:
			return RedactionMutationResult{}, &StateConflictError{
				Code:    "invalid_state",
				Message: "redaction cannot be accepted from current state",
			}
		}
	case "reject":
		if runtime.ReviewState == "REJECTED" {
			return result, nil
		}
		switch runtime.ReviewState {
		case "PENDING", "ACCEPTED", "ADDED":
			runtime.ReviewState = "REJECTED"
		default:
			return RedactionMutationResult{}, &StateConflictError{
				Code:    "invalid_state",
				Message: "redaction cannot be rejected from current state",
			}
		}
	default:
		return RedactionMutationResult{}, &ValidationError{
			Code:    "invalid_action",
			Message: "invalid redaction action",
		}
	}

	timestamp := now.UTC().Format(time.RFC3339)
	runtime.ReviewedAt = &timestamp
	s.redactionRuntimeByID[redaction.ID] = runtime
	result.ReviewState = runtime.ReviewState
	result.Changed = true
	return result, nil
}

func (s *Store) AddManualRedaction(documentID string, input ManualRedactionInput, now time.Time) (RedactionSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	document, state, err := s.documentStateLocked(documentID)
	if err != nil {
		return RedactionSnapshot{}, err
	}

	switch state.Status {
	case "FAILED":
		return RedactionSnapshot{}, &StateConflictError{
			Code:    "failed_requires_retry",
			Message: "failed documents must be retried before review",
		}
	case "EXPORTED":
		return RedactionSnapshot{}, &StateConflictError{
			Code:    "exported_document_locked",
			Message: "exported documents cannot be modified in this MVP",
		}
	}

	if err := validateManualRedactionInput(document, s.documentRuneLengths[document.ID], input); err != nil {
		return RedactionSnapshot{}, err
	}

	spanText, err := doc.SubstringByRuneIndex(document.Text, input.Start, input.End)
	if err != nil {
		return RedactionSnapshot{}, &ValidationError{
			Code:    "invalid_span",
			Message: "manual redaction span is out of bounds",
		}
	}

	s.manualRedactionSeq++
	redactionID := formatManualRedactionID(s.manualRedactionSeq)
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "User-added manual redaction"
	}
	createdAt := now.UTC().Format(time.RFC3339)
	redaction := &Redaction{
		ID:              redactionID,
		DocumentID:      documentID,
		Start:           input.Start,
		End:             input.End,
		Text:            spanText,
		Type:            strings.ToUpper(strings.TrimSpace(input.Type)),
		Confidence:      float64Pointer(1.0),
		Reason:          reason,
		Source:          "user_added",
		SuggestedStatus: "USER_ADDED",
		IsGroundTruth:   false,
	}
	redactionState := RedactionRuntimeState{
		ReviewState: "ADDED",
		IsUserAdded: true,
		CreatedAt:   &createdAt,
	}

	s.redactionsByDoc[documentID] = append(s.redactionsByDoc[documentID], redaction)
	s.redactionsByID[redactionID] = redaction
	s.redactionRuntimeByID[redactionID] = redactionState

	return s.redactionSnapshotLocked(redaction), nil
}

func validateManualRedactionInput(document *Document, runeLength int, input ManualRedactionInput) error {
	if input.Start < 0 || input.End <= input.Start {
		return &ValidationError{
			Code:    "invalid_span",
			Message: "manual redaction span must satisfy 0 <= start < end",
		}
	}
	if input.End > runeLength {
		return &ValidationError{
			Code:    "invalid_span",
			Message: "manual redaction span is out of bounds",
		}
	}

	redactionType := strings.ToUpper(strings.TrimSpace(input.Type))
	if !doc.IsAllowedRedactionType(redactionType) {
		return &ValidationError{
			Code:    "invalid_type",
			Message: "manual redaction type is invalid",
		}
	}

	spanText, err := doc.SubstringByRuneIndex(document.Text, input.Start, input.End)
	if err != nil {
		return &ValidationError{
			Code:    "invalid_span",
			Message: "manual redaction span is out of bounds",
		}
	}
	if strings.TrimSpace(spanText) == "" {
		return &ValidationError{
			Code:    "invalid_span",
			Message: "manual redaction span cannot be only whitespace",
		}
	}
	if strings.TrimSpace(input.SelectedText) != "" && input.SelectedText != spanText {
		return &ValidationError{
			Code:    "selected_text_mismatch",
			Message: "selected_text does not match the document span",
		}
	}
	return nil
}
