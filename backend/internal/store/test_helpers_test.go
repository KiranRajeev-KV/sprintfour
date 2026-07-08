package store

import (
	"backend/internal/document"
	"backend/internal/seed"
	"backend/internal/testutil"
	"testing"
)

type recordingSubmitter struct {
	documentIDs []string
}

func (s *recordingSubmitter) Submit(documentID, _ string) {
	s.documentIDs = append(s.documentIDs, documentID)
}

func processUploadSync(t *testing.T, store *Store) {
	t.Helper()

	docs, total := store.Documents("QUEUED", "", "", 1000, 0)
	if total == 0 {
		t.Fatal("no queued documents to process")
	}
	for _, doc := range docs {
		if err := store.SetDocumentProcessed(doc.ID, document.DetectRuntimeRedactions(doc.Text)); err != nil {
			t.Fatalf("sync process doc %s: %v", doc.ID, err)
		}
	}
}

func testStore(t *testing.T, mutate func([]map[string]any, []map[string]any)) *Store {
	t.Helper()

	documentsPath, redactionsPath := testutil.WriteDatasetFixture(t, 200, mutate)
	documents, redactions, err := seed.LoadDataset(documentsPath, redactionsPath)
	if err != nil {
		t.Fatalf("LoadDataset returned error: %v", err)
	}
	return NewStore(documents, redactions)
}
