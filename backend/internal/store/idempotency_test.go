package store

import (
	"backend/internal/document"
	"testing"
	"time"
)

func TestSetDocumentProcessedReplacesRuntimeGeneratedRedactions(t *testing.T) {
	store := NewStore(nil, nil)
	result, err := store.UploadDocuments("replace", []document.UploadedDocumentInput{{
		Filename: "case.txt",
		Text:     "Email person@example.test phone +91 98765 43210",
	}}, time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("UploadDocuments returned error: %v", err)
	}

	documentID := *result.Items[0].DocumentID
	snapshot, ok := store.DocumentByID(documentID)
	if !ok {
		t.Fatalf("uploaded document %s missing", documentID)
	}
	detections := document.DetectRuntimeRedactions(snapshot.Text)

	if err := store.SetDocumentProcessed(documentID, detections); err != nil {
		t.Fatalf("first SetDocumentProcessed: %v", err)
	}
	first := store.RedactionsByDocumentID(documentID)

	if err := store.SetDocumentProcessed(documentID, detections); err != nil {
		t.Fatalf("second SetDocumentProcessed: %v", err)
	}
	second := store.RedactionsByDocumentID(documentID)

	if len(first) == 0 {
		t.Fatal("expected runtime redactions after first processing")
	}
	if len(second) != len(first) {
		t.Fatalf("expected stable redaction count after reprocessing, got first=%d second=%d", len(first), len(second))
	}
}
