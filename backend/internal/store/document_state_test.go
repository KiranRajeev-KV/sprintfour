package store

import (
	"errors"
	"strings"
	"testing"
	"time"
)

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

func TestBulkApproveApprovesReadyAndCleanButSkipsNeedsReviewAndFailed(t *testing.T) {
	store := testStore(t, nil)

	result := store.BulkApprove([]string{"doc_0001", "doc_0002", "doc_0003", "doc_0004"})
	if result.Approved != 2 || result.Skipped != 2 {
		t.Fatalf("unexpected bulk approve counts: %+v", result)
	}
}

func TestRetryFailedTransitionsDeterministically(t *testing.T) {
	store := testStore(t, nil)
	submitter := &recordingSubmitter{}
	store.SetJobSubmitter(submitter)

	highRiskResult, err := store.RetryDocument("doc_0003")
	if err != nil {
		t.Fatalf("RetryDocument(doc_0003) returned error: %v", err)
	}
	if !highRiskResult.Changed || highRiskResult.Status != "QUEUED" || highRiskResult.RetryCount != 1 {
		t.Fatalf("unexpected retry result for doc_0003: %+v", highRiskResult)
	}

	lowRiskResult, err := store.RetryDocument("doc_0005")
	if err != nil {
		t.Fatalf("RetryDocument(doc_0005) returned error: %v", err)
	}
	if !lowRiskResult.Changed || lowRiskResult.Status != "QUEUED" || lowRiskResult.RetryCount != 1 {
		t.Fatalf("unexpected retry result for doc_0005: %+v", lowRiskResult)
	}

	if len(submitter.documentIDs) != 2 {
		t.Fatalf("expected both failed documents to be resubmitted, got %+v", submitter.documentIDs)
	}
}

func TestBulkRetryResubmitsOnlyFailedDocuments(t *testing.T) {
	store := testStore(t, nil)
	submitter := &recordingSubmitter{}
	store.SetJobSubmitter(submitter)

	result := store.BulkRetry([]string{"doc_0001", "doc_0003", "doc_0005", "missing_doc"})
	if result.Retried != 2 || result.Skipped != 2 {
		t.Fatalf("unexpected bulk retry result: %+v", result)
	}
	if got := strings.Join(submitter.documentIDs, ","); got != "doc_0003,doc_0005" {
		t.Fatalf("expected failed documents to be resubmitted in order, got %s", got)
	}
}
