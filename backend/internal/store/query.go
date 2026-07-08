package store

import (
	doc "backend/internal/document"
	"slices"
	"sort"
	"strings"
)

func (s *Store) Summary() BatchSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summary := BatchSummary{
		TotalDocuments: len(s.documents),
	}

	for _, document := range s.documents {
		state := s.runtimeByDocID[document.ID]
		counts := s.redactionCountsLocked(document.ID)
		if counts.blockingReviewItems > 0 {
			summary.BlockingReviewDocuments++
		}
		switch doc.NormalizeStatus(state.Status) {
		case "QUEUED":
			summary.Queued++
		case "PROCESSING":
			summary.Processing++
		case "READY":
			summary.Ready++
		case "NEEDS_REVIEW":
			summary.NeedsReview++
		case "FAILED":
			summary.Failed++
		case "CLEAN":
			summary.Clean++
		case "APPROVED":
			summary.Approved++
		case "EXPORTED":
			summary.Exported++
		}
	}

	for _, redaction := range s.allRedactionsLocked() {
		runtime := s.redactionRuntimeByID[redaction.ID]
		summary.TotalRedactions++
		switch redaction.Source {
		case "synthetic_injection":
			summary.SyntheticRedactions++
		case "regex_candidate", "runtime_regex":
			summary.RegexCandidates++
		case "controlled_false_positive":
			summary.ControlledFalsePositives++
		case "controlled_missed_pii":
			summary.ControlledMissedPII++
		}
		switch runtime.ReviewState {
		case "PENDING":
			summary.PendingRedactions++
		case "ACCEPTED":
			summary.AcceptedRedactions++
		case "REJECTED":
			summary.RejectedRedactions++
		case "ADDED":
			summary.AddedRedactions++
		}
	}

	return summary
}

func (s *Store) Documents(status, risk, query string, limit, offset int) ([]DocumentSnapshot, int) {
	if strings.TrimSpace(status) != "" {
		status = doc.NormalizeStatus(status)
	}
	if strings.TrimSpace(risk) != "" {
		risk = doc.NormalizeRisk(risk)
	}
	query = strings.ToLower(strings.TrimSpace(query))

	s.mu.RLock()
	defer s.mu.RUnlock()

	filtered := make([]DocumentSnapshot, 0, len(s.documents))
	for _, document := range s.documents {
		snapshot := s.snapshotLocked(document)
		if status != "" && doc.NormalizeStatus(snapshot.Status) != status {
			continue
		}
		if risk != "" && doc.NormalizeRisk(snapshot.RiskLevel) != risk {
			continue
		}
		if query != "" {
			matchSource := strings.Contains(strings.ToLower(snapshot.SourceFile), query)
			matchTitle := strings.Contains(strings.ToLower(snapshot.Title), query)
			matchID := strings.Contains(strings.ToLower(snapshot.ID), query)
			if !matchSource && !matchTitle && !matchID {
				continue
			}
		}
		filtered = append(filtered, snapshot)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return documentStatusPriority(filtered[i].Status) < documentStatusPriority(filtered[j].Status)
	})

	total := len(filtered)
	if offset >= total {
		return []DocumentSnapshot{}, total
	}

	end := min(offset+limit, total)
	return slices.Clone(filtered[offset:end]), total
}

func (s *Store) DocumentByID(documentID string) (DocumentSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	document, ok := s.documentsByID[documentID]
	if !ok {
		return DocumentSnapshot{}, false
	}

	return s.snapshotLocked(document), true
}

func (s *Store) RedactionsByDocumentID(documentID string) []RedactionSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.redactionSnapshotsLocked(documentID)
}

func (s *Store) ReviewSummary(documentID string) (ReviewSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	document, state, err := s.documentStateLocked(documentID)
	if err != nil {
		return ReviewSummary{}, err
	}
	counts := s.redactionCountsLocked(document.ID)
	return ReviewSummary{
		DocumentID:               document.ID,
		Status:                   state.Status,
		RiskLevel:                document.RiskLevel,
		TotalRedactions:          counts.total,
		Pending:                  counts.pending,
		Accepted:                 counts.accepted,
		Rejected:                 counts.rejected,
		Added:                    counts.added,
		LowConfidence:            counts.lowConfidence,
		RegexCandidates:          counts.regexCandidates,
		ControlledFalsePositives: counts.controlledFalsePositives,
		ControlledMissedPII:      counts.controlledMissedPII,
		BlockingReviewItems:      counts.blockingReviewItems,
		CanApprove:               s.canApproveLocked(document.ID, state.Status),
	}, nil
}

func documentStatusPriority(status string) int {
	switch status {
	case "NEEDS_REVIEW":
		return 0
	case "FAILED":
		return 1
	case "PROCESSING":
		return 2
	case "QUEUED":
		return 3
	case "READY":
		return 4
	case "CLEAN":
		return 5
	case "APPROVED":
		return 6
	case "EXPORTED":
		return 7
	default:
		return 99
	}
}

func (s *Store) snapshotLocked(document *Document) DocumentSnapshot {
	state := s.runtimeByDocID[document.ID]
	counts := s.redactionCountsLocked(document.ID)
	return DocumentSnapshot{
		ID:                     document.ID,
		Title:                  document.Title,
		Source:                 document.Source,
		SourceFile:             document.SourceFile,
		Text:                   document.Text,
		CharCount:              document.CharCount,
		SyntheticPIIInjected:   document.SyntheticPIIInjected,
		Status:                 state.Status,
		RiskLevel:              document.RiskLevel,
		FailureHint:            copyStringPointer(state.FailureHint),
		PIICount:               counts.total,
		LowConfidenceCount:     counts.lowConfidence,
		RedactionCount:         counts.total,
		RetryCount:             state.RetryCount,
		PendingRedactionCount:  counts.pending,
		AcceptedRedactionCount: counts.accepted,
		RejectedRedactionCount: counts.rejected,
		AddedRedactionCount:    counts.added,
		BlockingReviewCount:    counts.blockingReviewItems,
		CanApprove:             s.canApproveLocked(document.ID, state.Status),
	}
}

func (s *Store) redactionSnapshotsLocked(documentID string) []RedactionSnapshot {
	redactions := s.redactionsByDoc[documentID]
	items := make([]RedactionSnapshot, 0, len(redactions))
	for _, redaction := range redactions {
		items = append(items, s.redactionSnapshotLocked(redaction))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Start == items[j].Start {
			return items[i].End < items[j].End
		}
		return items[i].Start < items[j].Start
	})
	return items
}

func (s *Store) redactionSnapshotLocked(redaction *Redaction) RedactionSnapshot {
	runtime := s.redactionRuntimeByID[redaction.ID]
	return RedactionSnapshot{
		ID:              redaction.ID,
		DocumentID:      redaction.DocumentID,
		Start:           redaction.Start,
		End:             redaction.End,
		Text:            redaction.Text,
		Type:            redaction.Type,
		Confidence:      redaction.Confidence,
		Reason:          redaction.Reason,
		Source:          redaction.Source,
		SuggestedStatus: redaction.SuggestedStatus,
		IsGroundTruth:   redaction.IsGroundTruth,
		ReviewState:     runtime.ReviewState,
		ReviewedAt:      copyStringPointer(runtime.ReviewedAt),
		ReviewedBy:      nil,
		IsUserAdded:     runtime.IsUserAdded,
		CreatedAt:       copyStringPointer(runtime.CreatedAt),
	}
}

func (s *Store) countDocumentsByStatusLocked(status string) int {
	count := 0
	for _, document := range s.documents {
		if s.runtimeByDocID[document.ID].Status == status {
			count++
		}
	}
	return count
}

func (s *Store) redactionCountsLocked(documentID string) redactionCounts {
	counts := redactionCounts{}
	for _, redaction := range s.redactionsByDoc[documentID] {
		runtime := s.redactionRuntimeByID[redaction.ID]
		counts.total++
		if redaction.Confidence != nil && *redaction.Confidence < lowConfidenceThreshold {
			counts.lowConfidence++
		}
		switch redaction.Source {
		case "regex_candidate", "runtime_regex":
			counts.regexCandidates++
		case "controlled_false_positive":
			counts.controlledFalsePositives++
		case "controlled_missed_pii":
			counts.controlledMissedPII++
		}
		switch runtime.ReviewState {
		case "PENDING":
			counts.pending++
			counts.blockingReviewItems++
		case "ACCEPTED":
			counts.accepted++
		case "REJECTED":
			counts.rejected++
		case "ADDED":
			counts.added++
		}
	}
	return counts
}

func (s *Store) canApproveLocked(documentID, status string) bool {
	status = doc.NormalizeStatus(status)
	if status == "FAILED" || status == "APPROVED" || status == "EXPORTED" {
		return false
	}
	return s.blockingReviewCountLocked(documentID) == 0
}

func (s *Store) blockingReviewCountLocked(documentID string) int {
	return s.redactionCountsLocked(documentID).blockingReviewItems
}

func (s *Store) allRedactionsLocked() []*Redaction {
	all := make([]*Redaction, 0, len(s.redactionsByID))
	for _, redaction := range s.redactionsByID {
		all = append(all, redaction)
	}
	return all
}
