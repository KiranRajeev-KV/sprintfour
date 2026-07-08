package store

import (
	"backend/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

func TestExportAppliesAcceptedAndAddedRedactions(t *testing.T) {
	store := testStore(t, func(documents []map[string]any, redactions []map[string]any) {
		documents[0]["text"] = "Intro मे Person Alpha and extra phone end"
		documents[0]["char_count"] = len([]rune(documents[0]["text"].(string)))
		redactions[0]["start"] = len([]rune("Intro मे "))
		redactions[0]["end"] = len([]rune("Intro मे Person Alpha"))
		redactions[0]["text"] = "Person Alpha"
		redactions[0]["type"] = "PERSON"
	})

	if _, err := store.AddManualRedaction("doc_0001", ManualRedactionInput{
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
			Start: testutil.RuneIndex(text, firstStart),
			End:   testutil.RuneIndex(text, firstStart) + len([]rune("PERSON_ONE and PERSON_ONE_EXTENDED")),
			Type:  "PERSON",
		},
		{
			ID:    "red_000002",
			Start: testutil.RuneIndex(text, secondStart),
			End:   testutil.RuneIndex(text, secondStart) + len([]rune("PERSON_ONE_EXTENDED")),
			Type:  "EMAIL",
		},
	}

	redacted, count, stats, err := RedactExportText(text, redactions)
	if err != nil {
		t.Fatalf("RedactExportText returned error: %v", err)
	}
	if count != 1 || stats.skippedOverlap != 1 {
		t.Fatalf("expected one applied and one overlap skip, got count=%d stats=%+v", count, stats)
	}
	if !strings.Contains(redacted, "[EMAIL_REDACTED]") && !strings.Contains(redacted, "[PERSON_REDACTED]") {
		t.Fatalf("expected at least one replacement, got %q", redacted)
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
	if _, err := store.AddManualRedaction(documentID, ManualRedactionInput{
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
	originalExportDir := ExportOutputDir()
	SetExportOutputDir(filepath.Join(t.TempDir(), "exported"))
	defer SetExportOutputDir(originalExportDir)

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

	outputPath := filepath.Join(ExportOutputDir(), "case-note_redacted.txt")
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "[EMAIL_REDACTED]") || !strings.Contains(text, "[PHONE_REDACTED]") {
		t.Fatalf("expected exported file to contain redactions, got %q", text)
	}
}
