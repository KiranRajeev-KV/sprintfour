package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestLoadDatasetValid(t *testing.T) {
	documentsPath, redactionsPath := writeDatasetFixture(t, 200, nil)

	documents, redactions, err := LoadDataset(documentsPath, redactionsPath)
	if err != nil {
		t.Fatalf("LoadDataset returned error: %v", err)
	}
	if got := len(documents); got != 200 {
		t.Fatalf("expected 200 documents, got %d", got)
	}
	if got := len(redactions); got != 5 {
		t.Fatalf("expected 5 redactions, got %d", got)
	}
}

func TestLoadDatasetRejectsDuplicateDocumentID(t *testing.T) {
	documentsPath, redactionsPath := writeDatasetFixture(t, 200, func(documents []map[string]any, _ []map[string]any) {
		documents[1]["id"] = documents[0]["id"]
	})

	_, _, err := LoadDataset(documentsPath, redactionsPath)
	if err == nil || !strings.Contains(err.Error(), "duplicate document id") {
		t.Fatalf("expected duplicate document id error, got %v", err)
	}
}

func TestLoadDatasetRejectsUnknownRedactionDocumentID(t *testing.T) {
	documentsPath, redactionsPath := writeDatasetFixture(t, 200, func(_ []map[string]any, redactions []map[string]any) {
		redactions[0]["document_id"] = "missing_doc"
	})

	_, _, err := LoadDataset(documentsPath, redactionsPath)
	if err == nil || !strings.Contains(err.Error(), "references unknown document") {
		t.Fatalf("expected unknown document error, got %v", err)
	}
}

func TestLoadDatasetRejectsRedactionSpanMismatch(t *testing.T) {
	documentsPath, redactionsPath := writeDatasetFixture(t, 200, func(_ []map[string]any, redactions []map[string]any) {
		redactions[0]["text"] = "wrong"
	})

	_, _, err := LoadDataset(documentsPath, redactionsPath)
	if err == nil || !strings.Contains(err.Error(), "span text mismatch") {
		t.Fatalf("expected span mismatch error, got %v", err)
	}
}

func TestSummaryCountsIncludeReviewState(t *testing.T) {
	store := testStore(t, nil)

	summary := store.Summary()
	if summary.TotalDocuments != 200 {
		t.Fatalf("expected 200 total documents, got %d", summary.TotalDocuments)
	}
	if summary.Ready != 196 || summary.NeedsReview != 1 || summary.Failed != 2 || summary.Clean != 1 {
		t.Fatalf("unexpected status counts: %+v", summary)
	}
	if summary.TotalRedactions != 5 || summary.SyntheticRedactions != 2 || summary.RegexCandidates != 1 || summary.ControlledFalsePositives != 1 || summary.ControlledMissedPII != 1 {
		t.Fatalf("unexpected source counts: %+v", summary)
	}
	if summary.PendingRedactions != 3 || summary.AcceptedRedactions != 2 || summary.RejectedRedactions != 0 || summary.AddedRedactions != 0 {
		t.Fatalf("unexpected review-state counts: %+v", summary)
	}
	if summary.BlockingReviewDocuments != 2 {
		t.Fatalf("expected 2 blocking review documents, got %+v", summary)
	}
}

func TestListDocumentsFiltersByStatus(t *testing.T) {
	store := testStore(t, nil)
	documents, total := store.Documents("needs_review", "", "", 50, 0)
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(documents) != 1 || documents[0].ID != "doc_0002" {
		t.Fatalf("unexpected filtered documents: %+v", documents)
	}
}

func TestGetDocumentReturns404ForMissingID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := testStore(t, nil)
	router := NewRouter(discardLogger(), store)

	request := httptest.NewRequest(http.MethodGet, "/api/documents/missing_doc", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}

	var payload APIErrorEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error.Code != "not_found" {
		t.Fatalf("expected not_found error code, got %q", payload.Error.Code)
	}
}

func TestAcceptPendingRedactionChangesToAccepted(t *testing.T) {
	store := testStore(t, nil)

	result, err := store.AcceptRedaction("red_000002", time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AcceptRedaction returned error: %v", err)
	}
	if !result.Changed || result.PreviousState != "PENDING" || result.ReviewState != "ACCEPTED" {
		t.Fatalf("unexpected accept result: %+v", result)
	}

	redactions := store.RedactionsByDocumentID("doc_0002")
	for _, redaction := range redactions {
		if redaction.ID == "red_000002" && redaction.ReviewState != "ACCEPTED" {
			t.Fatalf("expected accepted state in snapshot, got %+v", redaction)
		}
	}
}

func TestRejectAcceptedRedactionPreventsItFromExport(t *testing.T) {
	store := testStore(t, nil)

	if _, err := store.RejectRedaction("red_000001", time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("RejectRedaction returned error: %v", err)
	}
	if _, err := store.ApproveDocument("doc_0001"); err != nil {
		t.Fatalf("ApproveDocument returned error: %v", err)
	}

	summary, changed, err := store.ExportApprovedDocuments(time.Date(2026, 6, 30, 10, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ExportApprovedDocuments returned error: %v", err)
	}
	if !changed || summary.ExportedDocuments != 1 {
		t.Fatalf("unexpected export result: %+v changed=%v", summary, changed)
	}
	if summary.AppliedRedactions != 0 || summary.SkippedRejected != 1 {
		t.Fatalf("expected rejected redaction to be skipped, got %+v", summary)
	}
}

func TestAddManualRedactionStoresCorrectTextFromRuneOffsets(t *testing.T) {
	store := testStore(t, func(documents []map[string]any, redactions []map[string]any) {
		documents[0]["text"] = "Intro मे +91 98765 43210 outro"
		documents[0]["char_count"] = len([]rune(documents[0]["text"].(string)))
		redactions[0]["start"] = len([]rune("Intro मे "))
		redactions[0]["end"] = len([]rune("Intro मे ")) + len([]rune("+91 98765 43210"))
		redactions[0]["text"] = "+91 98765 43210"
		redactions[0]["type"] = "PHONE"
	})

	text := "Intro मे +91 98765 43210 outro"
	start := len([]rune("Intro मे "))
	end := start + len([]rune("+91 98765 43210"))

	redaction, err := store.AddManualRedaction("doc_0001", manualRedactionInput{
		Start:        start,
		End:          end,
		Type:         "PHONE",
		Reason:       "User selected visible phone number",
		SelectedText: "+91 98765 43210",
	}, time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AddManualRedaction returned error: %v", err)
	}
	if redaction.Text != "+91 98765 43210" {
		t.Fatalf("expected stored text to match rune span, got %q from %q", redaction.Text, text)
	}
	if redaction.ReviewState != "ADDED" || !redaction.IsUserAdded {
		t.Fatalf("unexpected manual redaction snapshot: %+v", redaction)
	}
}

func TestAddManualRedactionRejectsInvalidSpan(t *testing.T) {
	store := testStore(t, nil)

	_, err := store.AddManualRedaction("doc_0001", manualRedactionInput{
		Start: 10,
		End:   10,
		Type:  "PHONE",
	}, time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC))
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if validationErr.Code != "invalid_span" {
		t.Fatalf("expected invalid_span code, got %+v", validationErr)
	}
}

func TestManualRedactionOnFailedDocumentReturnsConflict(t *testing.T) {
	store := testStore(t, nil)

	_, err := store.AddManualRedaction("doc_0003", manualRedactionInput{
		Start: 0,
		End:   6,
		Type:  "PERSON",
	}, time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC))
	var conflictErr *StateConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected StateConflictError, got %v", err)
	}
	if conflictErr.Code != "failed_requires_retry" {
		t.Fatalf("unexpected conflict: %+v", conflictErr)
	}
}

func TestNeedsReviewDocumentWithBlockingItemsCannotBeApproved(t *testing.T) {
	store := testStore(t, nil)

	_, err := store.ApproveDocument("doc_0002")
	var conflictErr *StateConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected StateConflictError, got %v", err)
	}
	if conflictErr.Code != "unresolved_review_items" {
		t.Fatalf("unexpected conflict: %+v", conflictErr)
	}
}

func TestNeedsReviewDocumentCanBeApprovedAfterBlockingItemsResolved(t *testing.T) {
	store := testStore(t, nil)
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)

	if _, err := store.AcceptRedaction("red_000002", now); err != nil {
		t.Fatalf("AcceptRedaction(red_000002) returned error: %v", err)
	}
	if _, err := store.RejectRedaction("red_000003", now); err != nil {
		t.Fatalf("RejectRedaction(red_000003) returned error: %v", err)
	}

	result, err := store.ApproveDocument("doc_0002")
	if err != nil {
		t.Fatalf("ApproveDocument returned error: %v", err)
	}
	if !result.Changed || result.Status != "APPROVED" {
		t.Fatalf("unexpected approve result: %+v", result)
	}
}

func TestExportAppliesAcceptedAndAddedRedactions(t *testing.T) {
	store := testStore(t, func(documents []map[string]any, redactions []map[string]any) {
		documents[0]["text"] = "Intro मे Person Alpha and extra phone end"
		documents[0]["char_count"] = len([]rune(documents[0]["text"].(string)))
		redactions[0]["start"] = len([]rune("Intro मे "))
		redactions[0]["end"] = len([]rune("Intro मे Person Alpha"))
		redactions[0]["text"] = "Person Alpha"
		redactions[0]["type"] = "PERSON"
	})

	if _, err := store.AddManualRedaction("doc_0001", manualRedactionInput{
		Start:        len([]rune("Intro मे Person Alpha and ")),
		End:          len([]rune("Intro मे Person Alpha and extra phone")),
		Type:         "PHONE",
		SelectedText: "extra phone",
	}, time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("AddManualRedaction returned error: %v", err)
	}
	if _, err := store.ApproveDocument("doc_0001"); err != nil {
		t.Fatalf("ApproveDocument returned error: %v", err)
	}

	summary, _, err := store.ExportApprovedDocuments(time.Date(2026, 6, 30, 10, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ExportApprovedDocuments returned error: %v", err)
	}
	if summary.AppliedRedactions != 2 {
		t.Fatalf("expected 2 applied redactions, got %+v", summary)
	}
}

func TestExportSkipsRejectedRedactionsAndBlocksPendingApprovedDocuments(t *testing.T) {
	store := testStore(t, func(documents []map[string]any, _ []map[string]any) {
		documents[1]["workflow_hint"] = "APPROVED"
	})
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)

	if _, err := store.RejectRedaction("red_000001", now); err != nil {
		t.Fatalf("RejectRedaction returned error: %v", err)
	}

	summary, _, err := store.ExportApprovedDocuments(time.Date(2026, 6, 30, 10, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ExportApprovedDocuments returned error: %v", err)
	}
	if summary.ExportedDocuments != 0 || summary.ApprovedBlockedByReview != 1 {
		t.Fatalf("expected approved document with pending review to be blocked, got %+v", summary)
	}
	if summary.SkippedRejected != 0 || summary.SkippedPending != 0 {
		t.Fatalf("expected no per-document skip counts when blocked before export, got %+v", summary)
	}

	store = testStore(t, nil)
	if _, err := store.RejectRedaction("red_000001", now); err != nil {
		t.Fatalf("RejectRedaction returned error: %v", err)
	}
	if _, err := store.ApproveDocument("doc_0001"); err != nil {
		t.Fatalf("ApproveDocument(doc_0001) returned error: %v", err)
	}

	summary, _, err = store.ExportApprovedDocuments(time.Date(2026, 6, 30, 10, 10, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ExportApprovedDocuments returned error: %v", err)
	}
	if summary.SkippedRejected != 1 {
		t.Fatalf("unexpected skip counts: %+v", summary)
	}
}

func TestExportHandlesOverlappingAcceptedRedactionsSafely(t *testing.T) {
	text := "Intro मे PERSON_ONE and PERSON_ONE_EXTENDED end"
	firstStart := strings.Index(text, "PERSON_ONE")
	secondStart := strings.Index(text, "PERSON_ONE_EXTENDED")

	redactions := []RedactionSnapshot{
		{
			ID:    "red_000001",
			Start: runeIndex(text, firstStart),
			End:   runeIndex(text, firstStart) + len([]rune("PERSON_ONE and PERSON_ONE_EXTENDED")),
			Type:  "PERSON",
		},
		{
			ID:    "red_000002",
			Start: runeIndex(text, secondStart),
			End:   runeIndex(text, secondStart) + len([]rune("PERSON_ONE_EXTENDED")),
			Type:  "EMAIL",
		},
	}

	redacted, count, stats, err := redactExportText(text, redactions)
	if err != nil {
		t.Fatalf("redactExportText returned error: %v", err)
	}
	if count != 1 || stats.skippedOverlap != 1 {
		t.Fatalf("expected one applied and one overlap skip, got count=%d stats=%+v", count, stats)
	}
	if !strings.Contains(redacted, "[EMAIL_REDACTED]") && !strings.Contains(redacted, "[PERSON_REDACTED]") {
		t.Fatalf("expected at least one replacement, got %q", redacted)
	}
}

func TestReviewSummaryEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := testStore(t, nil)
	router := NewRouter(discardLogger(), store)

	request := httptest.NewRequest(http.MethodGet, "/api/documents/doc_0002/review-summary", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var payload ReviewSummary
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.BlockingReviewItems != 2 || payload.CanApprove {
		t.Fatalf("unexpected review summary: %+v", payload)
	}
}

func TestExportLatestEndpointReflectsLatestRun(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := testStore(t, nil)
	router := NewRouter(discardLogger(), store)

	if _, err := store.ApproveDocument("doc_0001"); err != nil {
		t.Fatalf("ApproveDocument returned error: %v", err)
	}
	if _, _, err := store.ExportApprovedDocuments(time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ExportApprovedDocuments returned error: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/export/latest", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["has_export"] != true {
		t.Fatalf("expected has_export=true, got %+v", payload)
	}
	if payload["exported_documents"].(float64) != 1 {
		t.Fatalf("expected exported_documents=1, got %+v", payload)
	}
}

func TestBulkApproveApprovesReadyAndCleanButSkipsNeedsReviewAndFailed(t *testing.T) {
	store := testStore(t, nil)

	result := store.BulkApprove([]string{"doc_0001", "doc_0002", "doc_0003", "doc_0004"})
	if result.Approved != 2 || result.Skipped != 2 {
		t.Fatalf("unexpected bulk approve counts: %+v", result)
	}

	expected := map[string]string{
		"doc_0001": "approved",
		"doc_0002": "needs_review_not_bulk_approved",
		"doc_0003": "failed_not_approvable",
		"doc_0004": "approved",
	}
	for _, item := range result.Items {
		if expected[item.DocumentID] != item.Reason {
			t.Fatalf("unexpected bulk approve reason for %s: %+v", item.DocumentID, item)
		}
	}
}

func TestRetryFailedTransitionsDeterministically(t *testing.T) {
	store := testStore(t, nil)

	highRiskResult, err := store.RetryDocument("doc_0003")
	if err != nil {
		t.Fatalf("RetryDocument(doc_0003) returned error: %v", err)
	}
	if !highRiskResult.Changed || highRiskResult.Status != "NEEDS_REVIEW" || highRiskResult.RetryCount != 1 {
		t.Fatalf("unexpected retry result for doc_0003: %+v", highRiskResult)
	}

	lowRiskResult, err := store.RetryDocument("doc_0005")
	if err != nil {
		t.Fatalf("RetryDocument(doc_0005) returned error: %v", err)
	}
	if !lowRiskResult.Changed || lowRiskResult.Status != "READY" || lowRiskResult.RetryCount != 1 {
		t.Fatalf("unexpected retry result for doc_0005: %+v", lowRiskResult)
	}
}

func TestExportIsIdempotentAfterApprovedDocumentsAreConsumed(t *testing.T) {
	store := testStore(t, nil)

	if _, err := store.ApproveDocument("doc_0001"); err != nil {
		t.Fatalf("ApproveDocument returned error: %v", err)
	}

	firstSummary, changed, err := store.ExportApprovedDocuments(time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("first ExportApprovedDocuments returned error: %v", err)
	}
	if !changed {
		t.Fatalf("expected first export to change state")
	}

	secondSummary, changed, err := store.ExportApprovedDocuments(time.Date(2026, 6, 30, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("second ExportApprovedDocuments returned error: %v", err)
	}
	if changed {
		t.Fatalf("expected second export to be a no-op")
	}
	if secondSummary.ExportID != firstSummary.ExportID {
		t.Fatalf("expected export id to remain stable, got %s then %s", firstSummary.ExportID, secondSummary.ExportID)
	}
}

func TestBackendStartsEmptyWithoutDataset(t *testing.T) {
	store := NewStore(nil, nil)
	summary := store.Summary()
	if summary.TotalDocuments != 0 || summary.TotalRedactions != 0 {
		t.Fatalf("expected empty startup summary, got %+v", summary)
	}

	gin.SetMode(gin.TestMode)
	router := NewRouter(discardLogger(), store)
	request := httptest.NewRequest(http.MethodGet, "/api/batch/summary", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
}

func TestUploadRejectsNonTXT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(discardLogger(), NewStore(nil, nil))
	body, contentType := multipartUploadBody(t, "replace", []uploadFixtureFile{
		{name: "scan.pdf", body: "not supported"},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/uploads/documents", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var payload UploadBatchResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Accepted != 0 || payload.Rejected != 1 || payload.DocumentsCreated != 0 {
		t.Fatalf("unexpected upload result: %+v", payload)
	}
	if payload.Items[0].Reason != "only_txt_supported" {
		t.Fatalf("unexpected rejection reason: %+v", payload.Items[0])
	}
}

func TestUploadRejectsEmptyTXT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(discardLogger(), NewStore(nil, nil))
	body, contentType := multipartUploadBody(t, "replace", []uploadFixtureFile{
		{name: "blank.txt", body: "   \n"},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/uploads/documents", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var payload UploadBatchResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Accepted != 0 || payload.Rejected != 1 {
		t.Fatalf("unexpected upload result: %+v", payload)
	}
	if payload.Items[0].Reason != "empty_file" {
		t.Fatalf("unexpected rejection reason: %+v", payload.Items[0])
	}
}

func TestUploadAcceptsTXTAndCreatesRegexRedactions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewStore(nil, nil)
	router := NewRouter(discardLogger(), store)
	text := "Client email ananya.raman@example.test phone +91 98765 43210 PAN ABCDE1234F"
	body, contentType := multipartUploadBody(t, "replace", []uploadFixtureFile{
		{name: "case-note.txt", body: text},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/uploads/documents", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var payload UploadBatchResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Accepted != 1 || payload.DocumentsCreated != 1 {
		t.Fatalf("unexpected upload result: %+v", payload)
	}
	if payload.Items[0].Status == nil || *payload.Items[0].Status != "QUEUED" {
		t.Fatalf("expected QUEUED upload item, got %+v", payload.Items[0])
	}

	processUploadSync(t, store)

	documents, total := store.Documents("", "", "", 50, 0)
	if total != 1 || len(documents) != 1 {
		t.Fatalf("expected one stored document, got total=%d documents=%+v", total, documents)
	}
	redactions := store.RedactionsByDocumentID(documents[0].ID)
	if len(redactions) < 3 {
		t.Fatalf("expected regex redactions, got %+v", redactions)
	}
}

func TestUploadReplaceClearsPreviousBatch(t *testing.T) {
	store := NewStore(nil, nil)
	if _, err := store.UploadDocuments("replace", []UploadedDocumentInput{{
		Filename: "one.txt",
		Text:     "Contact ananya.raman@example.test",
	}}, time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("initial upload failed: %v", err)
	}
	if _, err := store.UploadDocuments("replace", []UploadedDocumentInput{{
		Filename: "two.txt",
		Text:     "No pii here",
	}}, time.Date(2026, 6, 30, 10, 5, 0, 0, time.UTC)); err != nil {
		t.Fatalf("replace upload failed: %v", err)
	}

	documents, total := store.Documents("", "", "", 50, 0)
	if total != 1 || len(documents) != 1 || documents[0].Title != "two" {
		t.Fatalf("expected only replacement batch to remain, got total=%d documents=%+v", total, documents)
	}
}

func TestUploadAppendPreservesPreviousBatch(t *testing.T) {
	store := NewStore(nil, nil)
	if _, err := store.UploadDocuments("replace", []UploadedDocumentInput{{
		Filename: "one.txt",
		Text:     "Contact ananya.raman@example.test",
	}}, time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("initial upload failed: %v", err)
	}
	if _, err := store.UploadDocuments("append", []UploadedDocumentInput{{
		Filename: "two.txt",
		Text:     "No pii here",
	}}, time.Date(2026, 6, 30, 10, 5, 0, 0, time.UTC)); err != nil {
		t.Fatalf("append upload failed: %v", err)
	}

	_, total := store.Documents("", "", "", 50, 0)
	if total != 2 {
		t.Fatalf("expected append to preserve both documents, got total=%d", total)
	}
}

func TestExportStillUsesAcceptedAndAddedRedactionsAfterUpload(t *testing.T) {
	store := NewStore(nil, nil)
	if _, err := store.UploadDocuments("replace", []UploadedDocumentInput{{
		Filename: "note.txt",
		Text:     "Owner Maya. Email ananya.raman@example.test phone +91 98765 43210",
	}}, time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	processUploadSync(t, store)

	documents, total := store.Documents("", "", "", 50, 0)
	if total != 1 {
		t.Fatalf("expected one document after upload, got %d", total)
	}
	documentID := documents[0].ID
	if _, err := store.AddManualRedaction(documentID, manualRedactionInput{
		Start:        len([]rune("Owner ")),
		End:          len([]rune("Owner Maya")),
		Type:         "PERSON",
		SelectedText: "Maya",
	}, time.Date(2026, 6, 30, 10, 2, 0, 0, time.UTC)); err != nil {
		t.Fatalf("AddManualRedaction failed: %v", err)
	}
	if _, err := store.ApproveDocument(documentID); err != nil {
		t.Fatalf("ApproveDocument failed: %v", err)
	}

	summary, _, err := store.ExportApprovedDocuments(time.Date(2026, 6, 30, 10, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ExportApprovedDocuments failed: %v", err)
	}
	if summary.ExportedDocuments != 1 || summary.AppliedRedactions < 3 {
		t.Fatalf("unexpected export summary after upload: %+v", summary)
	}
}

func TestExportWritesRedactedFilesToExportedFolder(t *testing.T) {
	store := NewStore(nil, nil)
	originalExportDir := exportOutputDir
	exportOutputDir = filepath.Join(t.TempDir(), "exported")
	defer func() {
		exportOutputDir = originalExportDir
	}()

	if _, err := store.UploadDocuments("replace", []UploadedDocumentInput{{
		Filename: "case-note.txt",
		Text:     "Email ananya.raman@example.test phone +91 98765 43210",
	}}, time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	processUploadSync(t, store)

	documents, total := store.Documents("", "", "", 50, 0)
	if total != 1 {
		t.Fatalf("expected one uploaded document, got %d", total)
	}
	if _, err := store.ApproveDocument(documents[0].ID); err != nil {
		t.Fatalf("ApproveDocument failed: %v", err)
	}

	summary, _, err := store.ExportApprovedDocuments(time.Date(2026, 6, 30, 10, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ExportApprovedDocuments failed: %v", err)
	}
	if summary.ExportedDocuments != 1 {
		t.Fatalf("expected one exported document, got %+v", summary)
	}

	outputPath := filepath.Join(exportOutputDir, "case-note_redacted.txt")
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "[EMAIL_REDACTED]") || !strings.Contains(text, "[PHONE_REDACTED]") {
		t.Fatalf("expected exported file to contain redactions, got %q", text)
	}
}

func writeDatasetFixture(t *testing.T, documentCount int, mutate func([]map[string]any, []map[string]any)) (string, string) {
	t.Helper()

	root := t.TempDir()
	documentsPath := filepath.Join(root, "documents_seed.jsonl")
	redactionsPath := filepath.Join(root, "mock_redactions.jsonl")

	documents := make([]map[string]any, 0, documentCount)
	for i := 1; i <= documentCount; i++ {
		id := formatDocID(i)
		status := "READY"
		risk := "LOW"
		var failureHint any
		if i == 2 {
			status = "NEEDS_REVIEW"
			risk = "MEDIUM"
		}
		if i == 3 {
			status = "FAILED"
			risk = "HIGH"
			failureHint = "SIMULATED_DETECTION_TIMEOUT"
		}
		if i == 4 {
			status = "CLEAN"
			risk = "LOW"
		}
		if i == 5 {
			status = "FAILED"
			risk = "LOW"
			failureHint = "SIMULATED_QUEUE_RETRY"
		}

		text := "Notice contact Person Alpha email person.alpha@example.test phone +91 98765 43210."
		documents = append(documents, map[string]any{
			"id":                     id,
			"title":                  "Document " + id,
			"source":                 "CUAD_v1",
			"source_file":            id + ".txt",
			"text":                   text,
			"char_count":             len([]rune(text)),
			"synthetic_pii_injected": i == 1 || i == 5,
			"workflow_hint":          status,
			"risk_level_hint":        risk,
			"failure_hint":           failureHint,
		})
	}

	baseText := documents[0]["text"].(string)
	emailStart := strings.Index(baseText, "person.alpha@example.test")
	phoneStart := strings.Index(baseText, "+91 98765 43210")
	noticeStart := strings.Index(baseText, "Notice")
	personStart := strings.Index(baseText, "Person Alpha")

	redactions := []map[string]any{
		{
			"id":               "red_000001",
			"document_id":      "doc_0001",
			"start":            runeIndex(baseText, emailStart),
			"end":              runeIndex(baseText, emailStart) + len([]rune("person.alpha@example.test")),
			"text":             "person.alpha@example.test",
			"type":             "EMAIL",
			"confidence":       0.98,
			"reason":           "Synthetic email",
			"source":           "synthetic_injection",
			"suggested_status": "ACCEPTED",
			"is_ground_truth":  true,
		},
		{
			"id":               "red_000002",
			"document_id":      "doc_0002",
			"start":            runeIndex(baseText, phoneStart),
			"end":              runeIndex(baseText, phoneStart) + len([]rune("+91 98765 43210")),
			"text":             "+91 98765 43210",
			"type":             "PHONE",
			"confidence":       0.57,
			"reason":           "Regex phone",
			"source":           "regex_candidate",
			"suggested_status": "REVIEW",
			"is_ground_truth":  false,
		},
		{
			"id":               "red_000003",
			"document_id":      "doc_0002",
			"start":            runeIndex(baseText, noticeStart),
			"end":              runeIndex(baseText, noticeStart) + len([]rune("Notice")),
			"text":             "Notice",
			"type":             "ORGANIZATION_CONTACT",
			"confidence":       0.28,
			"reason":           "Controlled false positive",
			"source":           "controlled_false_positive",
			"suggested_status": "REVIEW",
			"is_ground_truth":  false,
		},
		{
			"id":               "red_000004",
			"document_id":      "doc_0003",
			"start":            runeIndex(baseText, personStart),
			"end":              runeIndex(baseText, personStart) + len([]rune("Person Alpha")),
			"text":             "Person Alpha",
			"type":             "PERSON",
			"confidence":       0.22,
			"reason":           "Controlled missed PII warning",
			"source":           "controlled_missed_pii",
			"suggested_status": "REVIEW",
			"is_ground_truth":  false,
		},
		{
			"id":               "red_000005",
			"document_id":      "doc_0005",
			"start":            runeIndex(baseText, emailStart),
			"end":              runeIndex(baseText, emailStart) + len([]rune("person.alpha@example.test")),
			"text":             "person.alpha@example.test",
			"type":             "EMAIL",
			"confidence":       0.95,
			"reason":           "Synthetic email",
			"source":           "synthetic_injection",
			"suggested_status": "ACCEPTED",
			"is_ground_truth":  true,
		},
	}

	if mutate != nil {
		mutate(documents, redactions)
	}

	writeJSONLFixture(t, documentsPath, documents)
	writeJSONLFixture(t, redactionsPath, redactions)
	return documentsPath, redactionsPath
}

func writeJSONLFixture(t *testing.T, path string, records []map[string]any) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture %s: %v", path, err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatalf("encode fixture %s: %v", path, err)
		}
	}
}

type uploadFixtureFile struct {
	name string
	body string
}

func multipartUploadBody(t *testing.T, mode string, files []uploadFixtureFile) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("mode", mode); err != nil {
		t.Fatalf("write mode field: %v", err)
	}
	for _, file := range files {
		part, err := writer.CreateFormFile("files", file.name)
		if err != nil {
			t.Fatalf("create form file %s: %v", file.name, err)
		}
		if _, err := io.WriteString(part, file.body); err != nil {
			t.Fatalf("write form file %s: %v", file.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body, writer.FormDataContentType()
}

func testStore(t *testing.T, mutate func([]map[string]any, []map[string]any)) *Store {
	t.Helper()
	documentsPath, redactionsPath := writeDatasetFixture(t, 200, mutate)
	documents, redactions, err := LoadDataset(documentsPath, redactionsPath)
	if err != nil {
		t.Fatalf("LoadDataset returned error: %v", err)
	}
	return NewStore(documents, redactions)
}

func formatDocID(i int) string {
	return fmt.Sprintf("doc_%04d", i)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func runeIndex(text string, byteIndex int) int {
	return len([]rune(text[:byteIndex]))
}

// processUploadSync runs regex detection for all QUEUED documents and commits
// results synchronously. Used in tests that need inline processing.
func processUploadSync(t *testing.T, store *Store) {
	t.Helper()
	docs, total := store.Documents("QUEUED", "", "", 1000, 0)
	if total == 0 {
		t.Fatal("no queued documents to process")
	}
	for _, doc := range docs {
		detections := detectRuntimeRedactions(doc.Text)
		if err := store.SetDocumentProcessed(doc.ID, detections); err != nil {
			t.Fatalf("sync process doc %s: %v", doc.ID, err)
		}
	}
}
