package main

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

const lowConfidenceThreshold = 0.70

var (
	ErrDocumentNotFound  = errors.New("document not found")
	ErrRedactionNotFound = errors.New("redaction not found")
)

type StateConflictError struct {
	Code    string
	Message string
}

func (e *StateConflictError) Error() string {
	return e.Message
}

type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

type manualRedactionInput struct {
	Start        int
	End          int
	Type         string
	Reason       string
	SelectedText string
}

type redactionCounts struct {
	total                    int
	pending                  int
	accepted                 int
	rejected                 int
	added                    int
	lowConfidence            int
	regexCandidates          int
	controlledFalsePositives int
	controlledMissedPII      int
	blockingReviewItems      int
}

type exportStats struct {
	applied         int
	skippedRejected int
	skippedPending  int
	skippedOverlap  int
}

type Store struct {
	mu                    sync.RWMutex
	documents             []*Document
	documentsByID         map[string]*Document
	redactionsByDoc       map[string][]*Redaction
	redactionsByID        map[string]*Redaction
	documentRuneLengths   map[string]int
	runtimeByDocID        map[string]DocumentRuntimeState
	redactionRuntimeByID  map[string]RedactionRuntimeState
	latestExport          *ExportSummary
	exportSequence        int
	manualRedactionSeq    int
	uploadDocumentSeq     int
	generatedRedactionSeq int
	uploadBatchSeq        int
}

func NewStore(documents []*Document, redactions []*Redaction) *Store {
	store := &Store{}
	store.rebuildLocked(documents, redactions)
	return store
}

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
			Status:      normalizeStatus(document.Status),
			FailureHint: copyStringPointer(document.FailureHint),
		}
		if sequence, ok := parseUploadDocumentSequence(document.ID); ok && sequence > s.uploadDocumentSeq {
			s.uploadDocumentSeq = sequence
		}
	}

	for _, redaction := range redactions {
		s.redactionsByDoc[redaction.DocumentID] = append(s.redactionsByDoc[redaction.DocumentID], redaction)
		s.redactionsByID[redaction.ID] = redaction
		s.redactionRuntimeByID[redaction.ID] = initialRedactionRuntimeState(redaction)
		if sequence, ok := parseGeneratedRedactionSequence(redaction.ID); ok && sequence > s.generatedRedactionSeq {
			s.generatedRedactionSeq = sequence
		}
		if sequence, ok := parseManualRedactionSequence(redaction.ID); ok && sequence > s.manualRedactionSeq {
			s.manualRedactionSeq = sequence
		}
	}
}

func (s *Store) resetForUploadLocked() {
	s.rebuildLocked(nil, nil)
	s.latestExport = nil
	s.exportSequence = 0
}

func (s *Store) UploadDocuments(mode string, inputs []UploadedDocumentInput, now time.Time) (UploadBatchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	mode = normalizeUploadMode(mode)
	if mode != "replace" && mode != "append" {
		return UploadBatchResult{}, &ValidationError{
			Code:    "invalid_mode",
			Message: "upload mode must be replace or append",
		}
	}

	if mode == "replace" {
		s.resetForUploadLocked()
	}

	s.uploadBatchSeq++
	result := UploadBatchResult{
		BatchID:  formatBatchID(s.uploadBatchSeq),
		Mode:     mode,
		Uploaded: len(inputs),
		Items:    make([]UploadItemResult, 0, len(inputs)),
	}

	for _, input := range inputs {
		document, redactions := s.buildUploadArtifactsLocked(input)

		s.documents = append(s.documents, document)
		s.documentsByID[document.ID] = document
		s.documentRuneLengths[document.ID] = len([]rune(document.Text))
		s.runtimeByDocID[document.ID] = DocumentRuntimeState{
			Status: normalizeStatus(document.Status),
		}

		for _, redaction := range redactions {
			s.redactionsByDoc[document.ID] = append(s.redactionsByDoc[document.ID], redaction)
			s.redactionsByID[redaction.ID] = redaction
			s.redactionRuntimeByID[redaction.ID] = initialRedactionRuntimeState(redaction)
		}

		result.Accepted++
		result.DocumentsCreated++
		result.RedactionsCreated += len(redactions)
		status := normalizeStatus(document.Status)
		risk := normalizeRisk(document.RiskLevel)
		documentID := document.ID
		relativePath := nullableString(input.RelativePath)
		result.Items = append(result.Items, UploadItemResult{
			Filename:       input.Filename,
			RelativePath:   relativePath,
			DocumentID:     &documentID,
			Status:         &status,
			RiskLevel:      &risk,
			RedactionCount: len(redactions),
			Accepted:       true,
			Reason:         "uploaded",
		})
	}

	result.Rejected = result.Uploaded - result.Accepted
	return result, nil
}

func (s *Store) buildUploadArtifactsLocked(input UploadedDocumentInput) (*Document, []*Redaction) {
	s.uploadDocumentSeq++
	documentID := formatUploadDocumentID(s.uploadDocumentSeq)
	sourceFile := input.Filename
	if strings.TrimSpace(input.RelativePath) != "" {
		sourceFile = input.RelativePath
	}
	document := &Document{
		ID:                   documentID,
		Title:                titleFromFilename(input.Filename),
		Source:               "USER_UPLOAD_TXT",
		SourceFile:           sourceFile,
		Text:                 input.Text,
		CharCount:            len([]rune(input.Text)),
		SyntheticPIIInjected: false,
	}

	detections := detectRuntimeRedactions(input.Text)
	redactions := make([]*Redaction, 0, len(detections))
	for _, detection := range detections {
		s.generatedRedactionSeq++
		redactions = append(redactions, &Redaction{
			ID:              formatGeneratedRedactionID(s.generatedRedactionSeq),
			DocumentID:      documentID,
			Start:           detection.Start,
			End:             detection.End,
			Text:            detection.Text,
			Type:            detection.Type,
			Confidence:      detection.Confidence,
			Reason:          detection.Reason,
			Source:          detection.Source,
			SuggestedStatus: detection.SuggestedStatus,
			IsGroundTruth:   false,
		})
	}

	status, risk := classifyUploadedDocument(redactions)
	document.Status = status
	document.RiskLevel = risk
	document.RedactionCount = len(redactions)
	for _, redaction := range redactions {
		if redaction.Confidence < lowConfidenceThreshold {
			document.LowConfidenceCount++
		}
	}
	document.PIICount = len(redactions)
	return document, redactions
}

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
		switch normalizeStatus(state.Status) {
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
		status = normalizeStatus(status)
	}
	if strings.TrimSpace(risk) != "" {
		risk = normalizeRisk(risk)
	}
	query = strings.ToLower(strings.TrimSpace(query))

	s.mu.RLock()
	defer s.mu.RUnlock()

	filtered := make([]DocumentSnapshot, 0, len(s.documents))
	for _, document := range s.documents {
		snapshot := s.snapshotLocked(document)
		if status != "" && normalizeStatus(snapshot.Status) != status {
			continue
		}
		if risk != "" && normalizeRisk(snapshot.RiskLevel) != risk {
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
	case "FAILED":
		return DocumentMutationResult{}, &StateConflictError{
			Code:    "failed_requires_retry",
			Message: "failed documents must be retried before approval",
		}
	case "EXPORTED":
		return result, nil
	case "APPROVED":
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

	if state.Status != "FAILED" {
		return result, nil
	}

	state.Status = s.retryTargetStatusLocked(document)
	state.RetryCount++
	state.FailureHint = nil
	s.runtimeByDocID[document.ID] = state

	result.Status = state.Status
	result.RetryCount = state.RetryCount
	result.Changed = true
	return result, nil
}

func (s *Store) BulkRetry(documentIDs []string) BulkMutationResponse {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]DocumentMutationResult, 0, len(documentIDs))
	retriedCount := 0

	for _, documentID := range documentIDs {
		item, changed := s.bulkRetryDocumentLocked(documentID)
		items = append(items, item)
		if changed {
			retriedCount++
		}
	}

	return BulkMutationResponse{
		Requested: len(documentIDs),
		Retried:   retriedCount,
		Skipped:   len(documentIDs) - retriedCount,
		Items:     items,
	}
}

func (s *Store) AcceptRedaction(redactionID string, now time.Time) (RedactionMutationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	switch runtime.ReviewState {
	case "ACCEPTED":
		return result, nil
	case "ADDED":
		result.ReviewState = "ADDED"
		return result, nil
	case "PENDING", "REJECTED":
		runtime.ReviewState = "ACCEPTED"
		timestamp := now.UTC().Format(time.RFC3339)
		runtime.ReviewedAt = &timestamp
		s.redactionRuntimeByID[redaction.ID] = runtime
		result.ReviewState = runtime.ReviewState
		result.Changed = true
		return result, nil
	default:
		return RedactionMutationResult{}, &StateConflictError{
			Code:    "invalid_state",
			Message: "redaction cannot be accepted from current state",
		}
	}
}

func (s *Store) RejectRedaction(redactionID string, now time.Time) (RedactionMutationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	if runtime.ReviewState == "REJECTED" {
		return result, nil
	}

	switch runtime.ReviewState {
	case "PENDING", "ACCEPTED", "ADDED":
		runtime.ReviewState = "REJECTED"
		timestamp := now.UTC().Format(time.RFC3339)
		runtime.ReviewedAt = &timestamp
		s.redactionRuntimeByID[redaction.ID] = runtime
		result.ReviewState = runtime.ReviewState
		result.Changed = true
		return result, nil
	default:
		return RedactionMutationResult{}, &StateConflictError{
			Code:    "invalid_state",
			Message: "redaction cannot be rejected from current state",
		}
	}
}

func (s *Store) AddManualRedaction(documentID string, input manualRedactionInput, now time.Time) (RedactionSnapshot, error) {
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

	spanText, err := substringByRuneIndex(document.Text, input.Start, input.End)
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
		Confidence:      1.0,
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

func validateManualRedactionInput(document *Document, runeLength int, input manualRedactionInput) error {
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
	if !isAllowedRedactionType(redactionType) {
		return &ValidationError{
			Code:    "invalid_type",
			Message: "manual redaction type is invalid",
		}
	}

	spanText, err := substringByRuneIndex(document.Text, input.Start, input.End)
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

func (s *Store) ExportApprovedDocuments(now time.Time) (ExportSummary, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	approvedDocuments := make([]*Document, 0)
	approvedBlockedByReview := 0
	for _, document := range s.documents {
		if s.runtimeByDocID[document.ID].Status != "APPROVED" {
			continue
		}
		if s.blockingReviewCountLocked(document.ID) > 0 {
			approvedBlockedByReview++
			continue
		}
		approvedDocuments = append(approvedDocuments, document)
	}

	if len(approvedDocuments) == 0 {
		if s.latestExport != nil {
			return *s.latestExport, false, nil
		}

		s.exportSequence++
		s.latestExport = &ExportSummary{
			ExportID:                formatExportID(s.exportSequence),
			ExportedDocuments:       0,
			SkippedDocuments:        len(s.documents),
			NeedsReview:             s.countDocumentsByStatusLocked("NEEDS_REVIEW"),
			Failed:                  s.countDocumentsByStatusLocked("FAILED"),
			Ready:                   s.countDocumentsByStatusLocked("READY"),
			ApprovedBlockedByReview: approvedBlockedByReview,
			CreatedAt:               now.UTC().Format(time.RFC3339),
			Documents:               []ExportedDocument{},
		}
		return *s.latestExport, true, nil
	}

	exportedDocuments := make([]ExportedDocument, 0, len(approvedDocuments))
	stats := exportStats{}
	for _, document := range approvedDocuments {
		documentStats := s.exportDecisionStatsLocked(document.ID)
		redactedText, appliedCount, renderStats, err := redactExportText(document.Text, s.exportableRedactionsLocked(document.ID))
		if err != nil {
			return ExportSummary{}, false, err
		}
		stats.applied += renderStats.applied
		stats.skippedRejected += documentStats.skippedRejected
		stats.skippedPending += documentStats.skippedPending
		stats.skippedOverlap += renderStats.skippedOverlap
		exportedDocuments = append(exportedDocuments, ExportedDocument{
			DocumentID:            document.ID,
			Title:                 document.Title,
			SourceFile:            document.SourceFile,
			RedactedText:          redactedText,
			AppliedRedactionCount: appliedCount,
		})
	}

	if err := writeExportedDocuments(exportedDocuments); err != nil {
		return ExportSummary{}, false, err
	}

	for _, document := range approvedDocuments {
		state := s.runtimeByDocID[document.ID]
		state.Status = "EXPORTED"
		s.runtimeByDocID[document.ID] = state
	}

	s.exportSequence++
	s.latestExport = &ExportSummary{
		ExportID:                formatExportID(s.exportSequence),
		ExportedDocuments:       len(approvedDocuments),
		SkippedDocuments:        len(s.documents) - len(approvedDocuments),
		NeedsReview:             s.countDocumentsByStatusLocked("NEEDS_REVIEW"),
		Failed:                  s.countDocumentsByStatusLocked("FAILED"),
		Ready:                   s.countDocumentsByStatusLocked("READY"),
		ApprovedBlockedByReview: approvedBlockedByReview,
		AppliedRedactions:       stats.applied,
		SkippedRejected:         stats.skippedRejected,
		SkippedPending:          stats.skippedPending,
		SkippedOverlap:          stats.skippedOverlap,
		CreatedAt:               now.UTC().Format(time.RFC3339),
		Documents:               exportedDocuments,
	}

	return *s.latestExport, true, nil
}

func (s *Store) LatestExport() (*ExportSummary, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.latestExport == nil {
		return nil, false
	}

	summary := *s.latestExport
	return &summary, true
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

func (s *Store) bulkRetryDocumentLocked(documentID string) (DocumentMutationResult, bool) {
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

	if state.Status != "FAILED" {
		item.Reason = "not_failed"
		return item, false
	}

	state.Status = s.retryTargetStatusLocked(document)
	state.RetryCount++
	state.FailureHint = nil
	s.runtimeByDocID[document.ID] = state

	item.Status = state.Status
	item.RetryCount = state.RetryCount
	item.Changed = true
	item.Reason = "retried"
	return item, true
}

func (s *Store) retryTargetStatusLocked(document *Document) string {
	if normalizeRisk(document.RiskLevel) == "HIGH" {
		return "NEEDS_REVIEW"
	}
	if s.redactionCountsLocked(document.ID).lowConfidence > 0 {
		return "NEEDS_REVIEW"
	}
	for _, redaction := range s.redactionsByDoc[document.ID] {
		switch redaction.Source {
		case "controlled_missed_pii", "controlled_false_positive":
			return "NEEDS_REVIEW"
		}
	}
	return "READY"
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
		if redaction.Confidence < lowConfidenceThreshold {
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
	status = normalizeStatus(status)
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

func redactExportText(text string, redactions []RedactionSnapshot) (string, int, exportStats, error) {
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

func copyStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func formatExportID(sequence int) string {
	return fmt.Sprintf("export_%06d", sequence)
}

func formatManualRedactionID(sequence int) string {
	return fmt.Sprintf("user_red_%06d", sequence)
}

func isAllowedRedactionType(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "PERSON", "EMAIL", "PHONE", "ADDRESS", "CASE_ID", "CLIENT_ID", "BANK_ACCOUNT", "PAN_LIKE_ID", "DATE_OF_BIRTH", "ORGANIZATION_CONTACT":
		return true
	default:
		return false
	}
}
