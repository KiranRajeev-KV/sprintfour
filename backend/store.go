package main

import (
	"slices"
	"strings"
)

const lowConfidenceThreshold = 0.70

type Store struct {
	documents         []*Document
	documentsByID     map[string]*Document
	redactionsByDocID map[string][]*Redaction
	summary           BatchSummary
}

func NewStore(documents []*Document, redactions []*Redaction) *Store {
	store := &Store{
		documents:         documents,
		documentsByID:     make(map[string]*Document, len(documents)),
		redactionsByDocID: make(map[string][]*Redaction, len(documents)),
	}

	for _, document := range documents {
		store.documentsByID[document.ID] = document
	}

	for _, redaction := range redactions {
		store.redactionsByDocID[redaction.DocumentID] = append(store.redactionsByDocID[redaction.DocumentID], redaction)
	}

	for _, document := range store.documents {
		redactionsForDocument := store.redactionsByDocID[document.ID]
		document.RedactionCount = len(redactionsForDocument)
		document.PIICount = len(redactionsForDocument)
		for _, redaction := range redactionsForDocument {
			if redaction.Confidence < lowConfidenceThreshold {
				document.LowConfidenceCount++
			}
		}
	}

	store.summary = computeSummary(documents, redactions)
	return store
}

func computeSummary(documents []*Document, redactions []*Redaction) BatchSummary {
	summary := BatchSummary{
		TotalDocuments: len(documents),
		Approved:       0,
		Exported:       0,
	}

	for _, document := range documents {
		switch normalizeStatus(document.Status) {
		case "READY":
			summary.Ready++
		case "NEEDS_REVIEW":
			summary.NeedsReview++
		case "FAILED":
			summary.Failed++
		case "CLEAN":
			summary.Clean++
		}
	}

	for _, redaction := range redactions {
		summary.TotalRedactions++
		switch redaction.Source {
		case "synthetic_injection":
			summary.SyntheticRedactions++
		case "regex_candidate":
			summary.RegexCandidates++
		case "controlled_false_positive":
			summary.ControlledFalsePositives++
		case "controlled_missed_pii":
			summary.ControlledMissedPII++
		}
	}

	return summary
}

func (s *Store) Summary() BatchSummary {
	return s.summary
}

func (s *Store) Documents(status, risk, query string, limit, offset int) ([]*Document, int) {
	if strings.TrimSpace(status) != "" {
		status = normalizeStatus(status)
	}
	if strings.TrimSpace(risk) != "" {
		risk = normalizeRisk(risk)
	}
	query = strings.ToLower(strings.TrimSpace(query))

	filtered := make([]*Document, 0, len(s.documents))
	for _, document := range s.documents {
		if status != "" && normalizeStatus(document.Status) != status {
			continue
		}
		if risk != "" && normalizeRisk(document.RiskLevel) != risk {
			continue
		}
		if query != "" {
			matchSource := strings.Contains(strings.ToLower(document.SourceFile), query)
			matchTitle := strings.Contains(strings.ToLower(document.Title), query)
			matchID := strings.Contains(strings.ToLower(document.ID), query)
			if !matchSource && !matchTitle && !matchID {
				continue
			}
		}
		filtered = append(filtered, document)
	}

	total := len(filtered)
	if offset >= total {
		return []*Document{}, total
	}

	end := min(offset+limit, total)
	return filtered[offset:end], total
}

func (s *Store) DocumentByID(documentID string) (*Document, bool) {
	document, ok := s.documentsByID[documentID]
	return document, ok
}

func (s *Store) RedactionsByDocumentID(documentID string) []*Redaction {
	redactions := s.redactionsByDocID[documentID]
	return slices.Clone(redactions)
}
