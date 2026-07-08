package worker

import (
	"backend/internal/document"
	"backend/internal/store"
	"backend/internal/testutil"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type alwaysFailingDetector struct{}

func (alwaysFailingDetector) Detect(context.Context, string, string) ([]document.RuntimeDetection, error) {
	return nil, errors.New("forced detector failure")
}

func TestJobStatusDuringRetries(t *testing.T) {
	doc := &document.Document{
		ID:         "doc1",
		Title:      "t",
		Source:     "s",
		SourceFile: "f.txt",
		Text:       "plain text",
		CharCount:  len([]rune("plain text")),
		Status:     "QUEUED",
		RiskLevel:  "UNKNOWN",
	}
	store := store.NewStore([]*document.Document{doc}, nil)
	pool := NewWorkerPool(testutil.DiscardLogger(), store, alwaysFailingDetector{}, 1, 16)
	pool.Start()
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := pool.Shutdown(shutdownCtx); err != nil {
			t.Fatalf("shutdown worker pool: %v", err)
		}
	})

	for i := 0; i < 16; i++ {
		pool.Submit(doc.ID, doc.Text)
	}

	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		for id := 1; id <= 16; id++ {
			pool.JobStatus(jobID(id))
		}
	}
}

func jobID(sequence int) string {
	return fmt.Sprintf("job_%06d", sequence)
}
