package store

import (
	"errors"
	"testing"
	"time"
)

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

func TestBulkAcceptRedactionsAcceptsMatchingGroup(t *testing.T) {
	store := testStore(t, func(documents []map[string]any, redactions []map[string]any) {
		documents[1]["text"] = "Purchaser signed. Purchaser confirmed."
		documents[1]["char_count"] = len([]rune(documents[1]["text"].(string)))
		redactions[1]["start"] = 0
		redactions[1]["end"] = len([]rune("Purchaser"))
		redactions[1]["text"] = "Purchaser"
		redactions[1]["type"] = "PERSON"
		redactions[1]["confidence"] = 0.82
		redactions[1]["reason"] = "Detected person via local GLiNER sidecar"
		redactions[1]["source"] = "gliner_local"
		redactions[2]["start"] = len([]rune("Purchaser signed. "))
		redactions[2]["end"] = len([]rune("Purchaser signed. Purchaser"))
		redactions[2]["text"] = "Purchaser"
		redactions[2]["type"] = "PERSON"
		redactions[2]["confidence"] = 0.82
		redactions[2]["reason"] = "Detected person via local GLiNER sidecar"
		redactions[2]["source"] = "gliner_local"
	})

	result, err := store.BulkAcceptRedactions([]string{"red_000002", "red_000003"}, time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("BulkAcceptRedactions returned error: %v", err)
	}
	if result.Accepted != 2 || result.Skipped != 0 {
		t.Fatalf("unexpected bulk accept result: %+v", result)
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

	start := len([]rune("Intro मे "))
	end := start + len([]rune("+91 98765 43210"))

	redaction, err := store.AddManualRedaction("doc_0001", ManualRedactionInput{
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
		t.Fatalf("expected stored text to match rune span, got %q", redaction.Text)
	}
	if redaction.ReviewState != "ADDED" || !redaction.IsUserAdded {
		t.Fatalf("unexpected manual redaction snapshot: %+v", redaction)
	}
}

func TestAddManualRedactionRejectsInvalidSpan(t *testing.T) {
	store := testStore(t, nil)

	_, err := store.AddManualRedaction("doc_0001", ManualRedactionInput{
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

	_, err := store.AddManualRedaction("doc_0003", ManualRedactionInput{
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
