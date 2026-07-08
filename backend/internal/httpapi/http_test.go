package httpapi

import (
	"backend/internal/document"
	"backend/internal/seed"
	"backend/internal/store"
	"backend/internal/testutil"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestGetDocumentReturns404ForMissingID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(testutil.DiscardLogger(), testStore(t, nil))

	request := httptest.NewRequest(http.MethodGet, "/api/documents/missing_doc", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}

	var payload document.APIErrorEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error.Code != "not_found" {
		t.Fatalf("expected not_found error code, got %q", payload.Error.Code)
	}
}

func TestReviewSummaryEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(testutil.DiscardLogger(), testStore(t, nil))

	request := httptest.NewRequest(http.MethodGet, "/api/documents/doc_0002/review-summary", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var payload document.ReviewSummary
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.BlockingReviewItems != 2 || payload.CanApprove {
		t.Fatalf("unexpected review summary: %+v", payload)
	}
}

func TestExportLatestEndpointReflectsLatestRun(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := testStore(t, nil)
	router := NewRouter(testutil.DiscardLogger(), store)

	if _, err := store.ApproveDocument("doc_0001"); err != nil {
		t.Fatalf("ApproveDocument returned error: %v", err)
	}
	if _, _, err := store.ExportApprovedDocuments(time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ExportApprovedDocuments returned error: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/export/latest", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["has_export"] != true {
		t.Fatalf("expected has_export=true, got %+v", payload)
	}
	if payload["exported_documents"].(float64) != 1 {
		t.Fatalf("expected exported_documents=1, got %+v", payload)
	}
}

func TestBackendStartsEmptyWithoutDataset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(testutil.DiscardLogger(), store.NewStore(nil, nil))

	request := httptest.NewRequest(http.MethodGet, "/api/batch/summary", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
}

func TestUploadRejectsNonTXT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(testutil.DiscardLogger(), store.NewStore(nil, nil))
	body, contentType := testutil.MultipartUploadBody(t, "replace", []testutil.UploadFixtureFile{
		{Name: "scan.pdf", Body: "not supported"},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/uploads/documents", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var payload document.UploadBatchResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Accepted != 0 || payload.Rejected != 1 || payload.DocumentsCreated != 0 {
		t.Fatalf("unexpected upload result: %+v", payload)
	}
	if payload.Items[0].Reason != "only_txt_supported" {
		t.Fatalf("unexpected rejection reason: %+v", payload.Items[0])
	}
}

func TestUploadRejectsEmptyTXT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(testutil.DiscardLogger(), store.NewStore(nil, nil))
	body, contentType := testutil.MultipartUploadBody(t, "replace", []testutil.UploadFixtureFile{
		{Name: "blank.txt", Body: "   \n"},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/uploads/documents", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var payload document.UploadBatchResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Accepted != 0 || payload.Rejected != 1 {
		t.Fatalf("unexpected upload result: %+v", payload)
	}
	if payload.Items[0].Reason != "empty_file" {
		t.Fatalf("unexpected rejection reason: %+v", payload.Items[0])
	}
}

func TestUploadAcceptsTXTAndCreatesRegexRedactions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := store.NewStore(nil, nil)
	router := NewRouter(testutil.DiscardLogger(), store)
	text := "Client email ananya.raman@example.test phone +91 98765 43210 PAN ABCDE1234F"
	body, contentType := testutil.MultipartUploadBody(t, "replace", []testutil.UploadFixtureFile{
		{Name: "case-note.txt", Body: text},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/uploads/documents", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var payload document.UploadBatchResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Accepted != 1 || payload.DocumentsCreated != 1 {
		t.Fatalf("unexpected upload result: %+v", payload)
	}
	if payload.Items[0].Status == nil || *payload.Items[0].Status != "QUEUED" {
		t.Fatalf("expected QUEUED upload item, got %+v", payload.Items[0])
	}

	processUploadSync(t, store)

	documents, total := store.Documents("", "", "", 50, 0)
	if total != 1 || len(documents) != 1 {
		t.Fatalf("expected one stored document, got total=%d documents=%+v", total, documents)
	}
	redactions := store.RedactionsByDocumentID(documents[0].ID)
	if len(redactions) < 3 {
		t.Fatalf("expected regex redactions, got %+v", redactions)
	}
}

func processUploadSync(t *testing.T, store *store.Store) {
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

func testStore(t *testing.T, mutate func([]map[string]any, []map[string]any)) *store.Store {
	t.Helper()

	documentsPath, redactionsPath := testutil.WriteDatasetFixture(t, 200, mutate)
	documents, redactions, err := seed.LoadDataset(documentsPath, redactionsPath)
	if err != nil {
		t.Fatalf("LoadDataset returned error: %v", err)
	}
	return store.NewStore(documents, redactions)
}
