package worker

import (
	"backend/internal/document"
	"backend/internal/store"
	"backend/internal/testutil"
	"context"
	"sync"
	"testing"
	"time"
)

func TestUploadProcessingTransitionsQueuedToProcessingToReady(t *testing.T) {
	store := store.NewStore(nil, nil)
	detector := &blockingDetector{
		started: make(chan string, 1),
		release: make(chan struct{}),
	}
	workerPool := NewWorkerPool(testutil.DiscardLogger(), store, detector, 1, 4)
	store.SetJobSubmitter(workerPool)
	workerPool.Start()
	t.Cleanup(func() {
		detector.Release()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := workerPool.Shutdown(shutdownCtx); err != nil {
			t.Fatalf("shutdown worker pool: %v", err)
		}
	})

	result, err := store.UploadDocuments("replace", []document.UploadedDocumentInput{{
		Filename: "queued.txt",
		Text:     "Client phone (408) 555-0199",
	}}, time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("UploadDocuments returned error: %v", err)
	}
	documentID := *result.Items[0].DocumentID

	select {
	case startedDocumentID := <-detector.started:
		if startedDocumentID != documentID {
			t.Fatalf("expected detector to start %s, got %s", documentID, startedDocumentID)
		}
	case <-time.After(time.Second):
		t.Fatal("expected worker to start processing uploaded document")
	}

	snapshot, ok := store.DocumentByID(documentID)
	if !ok {
		t.Fatalf("expected uploaded document %s to exist", documentID)
	}
	if snapshot.Status != "PROCESSING" {
		t.Fatalf("expected uploaded document to be PROCESSING, got %+v", snapshot)
	}

	detector.Release()

	waitForDocumentStatus(t, store, documentID, "CLEAN")
}

type blockingDetector struct {
	started     chan string
	release     chan struct{}
	releaseOnce sync.Once
}

func (d *blockingDetector) Detect(_ context.Context, documentID, _ string) ([]document.RuntimeDetection, error) {
	d.started <- documentID
	<-d.release
	return nil, nil
}

func (d *blockingDetector) Release() {
	d.releaseOnce.Do(func() {
		close(d.release)
	})
}

func waitForDocumentStatus(t *testing.T, store *store.Store, documentID, wantStatus string) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		document, ok := store.DocumentByID(documentID)
		if ok && document.Status == wantStatus {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	document, _ := store.DocumentByID(documentID)
	t.Fatalf("expected document %s to reach %s, got %+v", documentID, wantStatus, document)
}
