package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoadDatasetValid(t *testing.T) {
	documentsPath, redactionsPath := writeDatasetFixture(t, 200, nil)

	documents, redactions, err := LoadDataset(documentsPath, redactionsPath)
	if err != nil {
		t.Fatalf("LoadDataset returned error: %v", err)
	}
	if got := len(documents); got != 200 {
		t.Fatalf("expected 200 documents, got %d", got)
	}
	if got := len(redactions); got != 3 {
		t.Fatalf("expected 3 redactions, got %d", got)
	}
}

func TestLoadDatasetRejectsDuplicateDocumentID(t *testing.T) {
	documentsPath, redactionsPath := writeDatasetFixture(t, 200, func(documents []map[string]any, _ []map[string]any) {
		documents[1]["id"] = documents[0]["id"]
	})

	_, _, err := LoadDataset(documentsPath, redactionsPath)
	if err == nil || !strings.Contains(err.Error(), "duplicate document id") {
		t.Fatalf("expected duplicate document id error, got %v", err)
	}
}

func TestLoadDatasetRejectsUnknownRedactionDocumentID(t *testing.T) {
	documentsPath, redactionsPath := writeDatasetFixture(t, 200, func(_ []map[string]any, redactions []map[string]any) {
		redactions[0]["document_id"] = "missing_doc"
	})

	_, _, err := LoadDataset(documentsPath, redactionsPath)
	if err == nil || !strings.Contains(err.Error(), "references unknown document") {
		t.Fatalf("expected unknown document error, got %v", err)
	}
}

func TestLoadDatasetRejectsRedactionSpanMismatch(t *testing.T) {
	documentsPath, redactionsPath := writeDatasetFixture(t, 200, func(_ []map[string]any, redactions []map[string]any) {
		redactions[0]["text"] = "wrong"
	})

	_, _, err := LoadDataset(documentsPath, redactionsPath)
	if err == nil || !strings.Contains(err.Error(), "span text mismatch") {
		t.Fatalf("expected span mismatch error, got %v", err)
	}
}

func TestSummaryCounts(t *testing.T) {
	documentsPath, redactionsPath := writeDatasetFixture(t, 200, nil)

	documents, redactions, err := LoadDataset(documentsPath, redactionsPath)
	if err != nil {
		t.Fatalf("LoadDataset returned error: %v", err)
	}

	store := NewStore(documents, redactions)
	summary := store.Summary()
	if summary.TotalDocuments != 200 {
		t.Fatalf("expected 200 total documents, got %d", summary.TotalDocuments)
	}
	if summary.Ready != 197 || summary.NeedsReview != 1 || summary.Failed != 1 || summary.Clean != 1 {
		t.Fatalf("unexpected status counts: %+v", summary)
	}
	if summary.TotalRedactions != 3 || summary.SyntheticRedactions != 1 || summary.RegexCandidates != 1 || summary.ControlledFalsePositives != 1 {
		t.Fatalf("unexpected redaction counts: %+v", summary)
	}
}

func TestListDocumentsFiltersByStatus(t *testing.T) {
	store := testStore(t)
	documents, total := store.Documents("needs_review", "", "", 50, 0)
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(documents) != 1 || documents[0].ID != "doc_0002" {
		t.Fatalf("unexpected filtered documents: %+v", documents)
	}
}

func TestGetDocumentReturns404ForMissingID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := testStore(t)
	router := NewRouter(discardLogger(), store)

	request := httptest.NewRequest(http.MethodGet, "/api/documents/missing_doc", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}

	var payload APIErrorEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error.Code != "not_found" {
		t.Fatalf("expected not_found error code, got %q", payload.Error.Code)
	}
}

func writeDatasetFixture(t *testing.T, documentCount int, mutate func([]map[string]any, []map[string]any)) (string, string) {
	t.Helper()

	root := t.TempDir()
	documentsPath := filepath.Join(root, "documents_seed.jsonl")
	redactionsPath := filepath.Join(root, "mock_redactions.jsonl")

	documents := make([]map[string]any, 0, documentCount)
	for i := 1; i <= documentCount; i++ {
		id := formatDocID(i)
		status := "READY"
		risk := "LOW"
		var failureHint any
		if i == 2 {
			status = "NEEDS_REVIEW"
			risk = "MEDIUM"
		}
		if i == 3 {
			status = "FAILED"
			risk = "HIGH"
			failureHint = "SIMULATED_DETECTION_TIMEOUT"
		}
		if i == 4 {
			status = "CLEAN"
			risk = "LOW"
		}

		text := "Notice contact Person Alpha email person.alpha@example.test phone +91 98765 43210."
		documents = append(documents, map[string]any{
			"id":                     id,
			"title":                  "Document " + id,
			"source":                 "CUAD_v1",
			"source_file":            id + ".txt",
			"text":                   text,
			"char_count":             len(text),
			"synthetic_pii_injected": i == 1,
			"workflow_hint":          status,
			"risk_level_hint":        risk,
			"failure_hint":           failureHint,
		})
	}

	baseText := documents[0]["text"].(string)
	emailStart := strings.Index(baseText, "person.alpha@example.test")
	phoneStart := strings.Index(baseText, "+91 98765 43210")
	noticeStart := strings.Index(baseText, "Notice")

	redactions := []map[string]any{
		{
			"id":               "red_000001",
			"document_id":      "doc_0001",
			"start":            emailStart,
			"end":              emailStart + len("person.alpha@example.test"),
			"text":             "person.alpha@example.test",
			"type":             "EMAIL",
			"confidence":       0.98,
			"reason":           "Synthetic email",
			"source":           "synthetic_injection",
			"suggested_status": "ACCEPTED",
			"is_ground_truth":  true,
		},
		{
			"id":               "red_000002",
			"document_id":      "doc_0002",
			"start":            phoneStart,
			"end":              phoneStart + len("+91 98765 43210"),
			"text":             "+91 98765 43210",
			"type":             "PHONE",
			"confidence":       0.57,
			"reason":           "Regex phone",
			"source":           "regex_candidate",
			"suggested_status": "REVIEW",
			"is_ground_truth":  false,
		},
		{
			"id":               "red_000003",
			"document_id":      "doc_0002",
			"start":            noticeStart,
			"end":              noticeStart + len("Notice"),
			"text":             "Notice",
			"type":             "ORGANIZATION_CONTACT",
			"confidence":       0.28,
			"reason":           "Controlled false positive",
			"source":           "controlled_false_positive",
			"suggested_status": "REVIEW",
			"is_ground_truth":  false,
		},
	}

	if mutate != nil {
		mutate(documents, redactions)
	}

	writeJSONLFixture(t, documentsPath, documents)
	writeJSONLFixture(t, redactionsPath, redactions)
	return documentsPath, redactionsPath
}

func writeJSONLFixture(t *testing.T, path string, records []map[string]any) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture %s: %v", path, err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatalf("encode fixture %s: %v", path, err)
		}
	}
}

func testStore(t *testing.T) *Store {
	t.Helper()
	documentsPath, redactionsPath := writeDatasetFixture(t, 200, nil)
	documents, redactions, err := LoadDataset(documentsPath, redactionsPath)
	if err != nil {
		t.Fatalf("LoadDataset returned error: %v", err)
	}
	return NewStore(documents, redactions)
}

func formatDocID(i int) string {
	return fmt.Sprintf("doc_%04d", i)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}
