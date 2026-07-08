package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type UploadFixtureFile struct {
	Name string
	Body string
}

func WriteDatasetFixture(t *testing.T, documentCount int, mutate func([]map[string]any, []map[string]any)) (string, string) {
	t.Helper()

	root := t.TempDir()
	documentsPath := filepath.Join(root, "documents_seed.jsonl")
	redactionsPath := filepath.Join(root, "mock_redactions.jsonl")

	documents := make([]map[string]any, 0, documentCount)
	for i := 1; i <= documentCount; i++ {
		id := FormatDocID(i)
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
		if i == 5 {
			status = "FAILED"
			risk = "LOW"
			failureHint = "SIMULATED_QUEUE_RETRY"
		}

		text := "Notice contact Person Alpha email person.alpha@example.test phone +91 98765 43210."
		documents = append(documents, map[string]any{
			"id":                     id,
			"title":                  "Document " + id,
			"source":                 "CUAD_v1",
			"source_file":            id + ".txt",
			"text":                   text,
			"char_count":             len([]rune(text)),
			"synthetic_pii_injected": i == 1 || i == 5,
			"workflow_hint":          status,
			"risk_level_hint":        risk,
			"failure_hint":           failureHint,
		})
	}

	baseText := documents[0]["text"].(string)
	emailStart := strings.Index(baseText, "person.alpha@example.test")
	phoneStart := strings.Index(baseText, "+91 98765 43210")
	noticeStart := strings.Index(baseText, "Notice")
	personStart := strings.Index(baseText, "Person Alpha")

	redactions := []map[string]any{
		{
			"id":               "red_000001",
			"document_id":      "doc_0001",
			"start":            RuneIndex(baseText, emailStart),
			"end":              RuneIndex(baseText, emailStart) + len([]rune("person.alpha@example.test")),
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
			"start":            RuneIndex(baseText, phoneStart),
			"end":              RuneIndex(baseText, phoneStart) + len([]rune("+91 98765 43210")),
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
			"start":            RuneIndex(baseText, noticeStart),
			"end":              RuneIndex(baseText, noticeStart) + len([]rune("Notice")),
			"text":             "Notice",
			"type":             "ORGANIZATION_CONTACT",
			"confidence":       0.28,
			"reason":           "Controlled false positive",
			"source":           "controlled_false_positive",
			"suggested_status": "REVIEW",
			"is_ground_truth":  false,
		},
		{
			"id":               "red_000004",
			"document_id":      "doc_0003",
			"start":            RuneIndex(baseText, personStart),
			"end":              RuneIndex(baseText, personStart) + len([]rune("Person Alpha")),
			"text":             "Person Alpha",
			"type":             "PERSON",
			"confidence":       0.22,
			"reason":           "Controlled missed PII warning",
			"source":           "controlled_missed_pii",
			"suggested_status": "REVIEW",
			"is_ground_truth":  false,
		},
		{
			"id":               "red_000005",
			"document_id":      "doc_0005",
			"start":            RuneIndex(baseText, emailStart),
			"end":              RuneIndex(baseText, emailStart) + len([]rune("person.alpha@example.test")),
			"text":             "person.alpha@example.test",
			"type":             "EMAIL",
			"confidence":       0.95,
			"reason":           "Synthetic email",
			"source":           "synthetic_injection",
			"suggested_status": "ACCEPTED",
			"is_ground_truth":  true,
		},
	}

	if mutate != nil {
		mutate(documents, redactions)
	}

	writeJSONLFixture(t, documentsPath, documents)
	writeJSONLFixture(t, redactionsPath, redactions)
	return documentsPath, redactionsPath
}

func MultipartUploadBody(t *testing.T, mode string, files []UploadFixtureFile) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("mode", mode); err != nil {
		t.Fatalf("write mode field: %v", err)
	}
	for _, file := range files {
		part, err := writer.CreateFormFile("files", file.Name)
		if err != nil {
			t.Fatalf("create form file %s: %v", file.Name, err)
		}
		if _, err := io.WriteString(part, file.Body); err != nil {
			t.Fatalf("write form file %s: %v", file.Name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body, writer.FormDataContentType()
}

func DiscardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func FormatDocID(i int) string {
	return fmt.Sprintf("doc_%04d", i)
}

func RuneIndex(text string, byteIndex int) int {
	return len([]rune(text[:byteIndex]))
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
