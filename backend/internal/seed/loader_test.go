package seed

import (
	"backend/internal/testutil"
	"strings"
	"testing"
)

func TestLoadDatasetValid(t *testing.T) {
	documentsPath, redactionsPath := testutil.WriteDatasetFixture(t, 200, nil)

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
	documentsPath, redactionsPath := testutil.WriteDatasetFixture(t, 200, func(documents []map[string]any, _ []map[string]any) {
		documents[1]["id"] = documents[0]["id"]
	})

	_, _, err := LoadDataset(documentsPath, redactionsPath)
	if err == nil || !strings.Contains(err.Error(), "duplicate document id") {
		t.Fatalf("expected duplicate document id error, got %v", err)
	}
}

func TestLoadDatasetRejectsUnknownRedactionDocumentID(t *testing.T) {
	documentsPath, redactionsPath := testutil.WriteDatasetFixture(t, 200, func(_ []map[string]any, redactions []map[string]any) {
		redactions[0]["document_id"] = "missing_doc"
	})

	_, _, err := LoadDataset(documentsPath, redactionsPath)
	if err == nil || !strings.Contains(err.Error(), "references unknown document") {
		t.Fatalf("expected unknown document error, got %v", err)
	}
}

func TestLoadDatasetRejectsRedactionSpanMismatch(t *testing.T) {
	documentsPath, redactionsPath := testutil.WriteDatasetFixture(t, 200, func(_ []map[string]any, redactions []map[string]any) {
		redactions[0]["text"] = "wrong"
	})

	_, _, err := LoadDataset(documentsPath, redactionsPath)
	if err == nil || !strings.Contains(err.Error(), "span text mismatch") {
		t.Fatalf("expected span mismatch error, got %v", err)
	}
}
