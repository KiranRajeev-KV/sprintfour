package store

import (
	"testing"
	"time"
)

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
