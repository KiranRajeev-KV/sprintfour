package store

import (
	doc "backend/internal/document"
	"slices"
	"strings"
)

func initialRedactionRuntimeState(redaction *Redaction) RedactionRuntimeState {
	return RedactionRuntimeState{
		ReviewState: initialReviewState(redaction),
		IsUserAdded: false,
	}
}

func initialReviewState(redaction *Redaction) string {
	if strings.EqualFold(strings.TrimSpace(redaction.SuggestedStatus), "ACCEPTED") {
		return "ACCEPTED"
	}
	switch redaction.Source {
	case "synthetic_injection":
		if strings.EqualFold(strings.TrimSpace(redaction.SuggestedStatus), "ACCEPTED") {
			return "ACCEPTED"
		}
	case "regex_candidate", "controlled_false_positive", "controlled_missed_pii":
		return "PENDING"
	}
	if strings.EqualFold(strings.TrimSpace(redaction.SuggestedStatus), "REVIEW") {
		return "PENDING"
	}
	return "PENDING"
}

func (s *Store) rebuildLocked(documents []*Document, redactions []*Redaction) {
	s.documents = slices.Clone(documents)
	s.documentsByID = make(map[string]*Document, len(documents))
	s.redactionsByDoc = make(map[string][]*Redaction, len(documents))
	s.redactionsByID = make(map[string]*Redaction, len(redactions))
	s.documentRuneLengths = make(map[string]int, len(documents))
	s.runtimeByDocID = make(map[string]DocumentRuntimeState, len(documents))
	s.redactionRuntimeByID = make(map[string]RedactionRuntimeState, len(redactions))
	s.manualRedactionSeq = 0
	s.uploadDocumentSeq = 0
	s.generatedRedactionSeq = 0

	for _, document := range documents {
		s.documentsByID[document.ID] = document
		s.documentRuneLengths[document.ID] = len([]rune(document.Text))
		s.runtimeByDocID[document.ID] = DocumentRuntimeState{
			Status:      doc.NormalizeStatus(document.Status),
			FailureHint: copyStringPointer(document.FailureHint),
		}
		if sequence, ok := doc.ParseUploadDocumentSequence(document.ID); ok && sequence > s.uploadDocumentSeq {
			s.uploadDocumentSeq = sequence
		}
	}

	for _, redaction := range redactions {
		s.redactionsByDoc[redaction.DocumentID] = append(s.redactionsByDoc[redaction.DocumentID], redaction)
		s.redactionsByID[redaction.ID] = redaction
		s.redactionRuntimeByID[redaction.ID] = initialRedactionRuntimeState(redaction)
		if sequence, ok := doc.ParseGeneratedRedactionSequence(redaction.ID); ok && sequence > s.generatedRedactionSeq {
			s.generatedRedactionSeq = sequence
		}
		if sequence, ok := doc.ParseManualRedactionSequence(redaction.ID); ok && sequence > s.manualRedactionSeq {
			s.manualRedactionSeq = sequence
		}
	}
}

func (s *Store) resetForUploadLocked() {
	s.rebuildLocked(nil, nil)
	s.latestExport = nil
	s.exportSequence = 0
}

func (s *Store) documentStateLocked(documentID string) (*Document, DocumentRuntimeState, error) {
	document, ok := s.documentsByID[documentID]
	if !ok {
		return nil, DocumentRuntimeState{}, ErrDocumentNotFound
	}
	return document, s.runtimeByDocID[documentID], nil
}

func (s *Store) redactionStateLocked(redactionID string) (*Redaction, RedactionRuntimeState, error) {
	redaction, ok := s.redactionsByID[redactionID]
	if !ok {
		return nil, RedactionRuntimeState{}, ErrRedactionNotFound
	}
	return redaction, s.redactionRuntimeByID[redactionID], nil
}

func copyStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func float64Pointer(value float64) *float64 {
	return &value
}

func isRuntimeGeneratedRedaction(redaction *Redaction) bool {
	if redaction == nil {
		return false
	}
	if _, ok := doc.ParseGeneratedRedactionSequence(redaction.ID); ok {
		return true
	}
	switch redaction.Source {
	case "runtime_regex", "gliner_local":
		return true
	default:
		return false
	}
}
