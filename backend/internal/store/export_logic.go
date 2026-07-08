package store

import (
	"fmt"
	"slices"
	"sort"
	"time"
)

func (s *Store) ExportApprovedDocuments(now time.Time) (ExportSummary, bool, error) {
	s.mu.Lock()
	plan := s.buildExportPlanLocked()
	if len(plan.documents) == 0 {
		if s.latestExport != nil {
			summary := cloneExportSummary(*s.latestExport)
			s.mu.Unlock()
			return summary, false, nil
		}

		s.exportSequence++
		s.latestExport = &ExportSummary{
			ExportID:                formatExportID(s.exportSequence),
			ExportedDocuments:       0,
			SkippedDocuments:        len(s.documents),
			NeedsReview:             s.countDocumentsByStatusLocked("NEEDS_REVIEW"),
			Failed:                  s.countDocumentsByStatusLocked("FAILED"),
			Ready:                   s.countDocumentsByStatusLocked("READY"),
			ApprovedBlockedByReview: plan.approvedBlockedByReview,
			OutputDir:               exportOutputDir,
			Files:                   []string{},
			CreatedAt:               now.UTC().Format(time.RFC3339),
			Documents:               []ExportedDocument{},
		}
		summary := cloneExportSummary(*s.latestExport)
		s.mu.Unlock()
		return summary, true, nil
	}
	s.mu.Unlock()

	exportedDocuments := make([]ExportedDocument, 0, len(plan.documents))
	stats := exportStats{}
	for _, document := range plan.documents {
		redactedText, appliedCount, renderStats, err := RedactExportText(document.Text, document.redactions)
		if err != nil {
			return ExportSummary{}, false, err
		}
		stats.applied += renderStats.applied
		stats.skippedRejected += document.stats.skippedRejected
		stats.skippedPending += document.stats.skippedPending
		stats.skippedOverlap += renderStats.skippedOverlap
		exportedDocuments = append(exportedDocuments, ExportedDocument{
			DocumentID:            document.DocumentID,
			Title:                 document.Title,
			SourceFile:            document.SourceFile,
			RedactedText:          redactedText,
			AppliedRedactionCount: appliedCount,
		})
	}

	if err := writeExportedDocuments(exportedDocuments); err != nil {
		return ExportSummary{}, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, document := range plan.documents {
		state := s.runtimeByDocID[document.DocumentID]
		state.Status = "EXPORTED"
		s.runtimeByDocID[document.DocumentID] = state
	}

	s.exportSequence++
	s.latestExport = &ExportSummary{
		ExportID:                formatExportID(s.exportSequence),
		ExportedDocuments:       len(plan.documents),
		SkippedDocuments:        len(s.documents) - len(plan.documents),
		NeedsReview:             s.countDocumentsByStatusLocked("NEEDS_REVIEW"),
		Failed:                  s.countDocumentsByStatusLocked("FAILED"),
		Ready:                   s.countDocumentsByStatusLocked("READY"),
		ApprovedBlockedByReview: plan.approvedBlockedByReview,
		AppliedRedactions:       stats.applied,
		SkippedRejected:         stats.skippedRejected,
		SkippedPending:          stats.skippedPending,
		SkippedOverlap:          stats.skippedOverlap,
		OutputDir:               exportOutputDir,
		Files:                   collectExportOutputFilenames(exportedDocuments),
		CreatedAt:               now.UTC().Format(time.RFC3339),
		Documents:               exportedDocuments,
	}

	return cloneExportSummary(*s.latestExport), true, nil
}

func (s *Store) LatestExport() (*ExportSummary, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.latestExport == nil {
		return nil, false
	}

	summary := cloneExportSummary(*s.latestExport)
	return &summary, true
}

func RedactExportText(text string, redactions []RedactionSnapshot) (string, int, exportStats, error) {
	if len(redactions) == 0 {
		return text, 0, exportStats{}, nil
	}

	runes := []rune(text)
	sorted := slices.Clone(redactions)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Start == sorted[j].Start {
			return sorted[i].End > sorted[j].End
		}
		return sorted[i].Start > sorted[j].Start
	})

	nextBoundary := len(runes)
	stats := exportStats{}
	applied := 0
	for _, redaction := range sorted {
		if redaction.Start < 0 || redaction.End < redaction.Start || redaction.End > len(runes) {
			return "", 0, exportStats{}, fmt.Errorf("export redaction %q out of bounds", redaction.ID)
		}
		if redaction.End > nextBoundary {
			stats.skippedOverlap++
			continue
		}
		replacement := []rune(fmt.Sprintf("[%s_REDACTED]", redaction.Type))
		runes = slices.Concat(runes[:redaction.Start], replacement, runes[redaction.End:])
		nextBoundary = redaction.Start
		applied++
		stats.applied++
	}

	return string(runes), applied, stats, nil
}

func (s *Store) exportableRedactionsLocked(documentID string) []RedactionSnapshot {
	candidates := s.redactionsByDoc[documentID]
	exportable := make([]RedactionSnapshot, 0, len(candidates))
	for _, redaction := range candidates {
		runtime := s.redactionRuntimeByID[redaction.ID]
		switch runtime.ReviewState {
		case "ACCEPTED", "ADDED":
			exportable = append(exportable, s.redactionSnapshotLocked(redaction))
		}
	}
	return exportable
}

func (s *Store) exportDecisionStatsLocked(documentID string) exportStats {
	stats := exportStats{}
	for _, redaction := range s.redactionsByDoc[documentID] {
		switch s.redactionRuntimeByID[redaction.ID].ReviewState {
		case "PENDING":
			stats.skippedPending++
		case "REJECTED":
			stats.skippedRejected++
		}
	}
	return stats
}

func formatExportID(sequence int) string {
	return fmt.Sprintf("export_%06d", sequence)
}

func collectExportOutputFilenames(documents []ExportedDocument) []string {
	files := make([]string, 0, len(documents))
	for _, document := range documents {
		if document.OutputFilename == "" {
			continue
		}
		files = append(files, document.OutputFilename)
	}
	return files
}

func (s *Store) buildExportPlanLocked() exportPlan {
	plan := exportPlan{
		documents: make([]exportPlanDocument, 0),
	}
	for _, document := range s.documents {
		if s.runtimeByDocID[document.ID].Status != "APPROVED" {
			continue
		}
		if s.blockingReviewCountLocked(document.ID) > 0 {
			plan.approvedBlockedByReview++
			continue
		}
		plan.documents = append(plan.documents, exportPlanDocument{
			DocumentID: document.ID,
			Title:      document.Title,
			SourceFile: document.SourceFile,
			Text:       document.Text,
			redactions: s.exportableRedactionsLocked(document.ID),
			stats:      s.exportDecisionStatsLocked(document.ID),
		})
	}
	return plan
}

func cloneExportSummary(summary ExportSummary) ExportSummary {
	summary.Files = slices.Clone(summary.Files)
	summary.Documents = slices.Clone(summary.Documents)
	return summary
}

func formatManualRedactionID(sequence int) string {
	return fmt.Sprintf("user_red_%06d", sequence)
}
