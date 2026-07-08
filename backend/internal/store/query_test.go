package store

import "testing"

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

func TestListDocumentsPrioritizesNeedsReviewBeforePagination(t *testing.T) {
	store := testStore(t, nil)

	documents, total := store.Documents("", "", "", 1, 0)
	if total == 0 {
		t.Fatal("expected documents in store")
	}
	if len(documents) != 1 {
		t.Fatalf("expected one paginated document, got %d", len(documents))
	}
	if documents[0].Status != "NEEDS_REVIEW" {
		t.Fatalf("expected first paginated document to need review, got %+v", documents[0])
	}
}
