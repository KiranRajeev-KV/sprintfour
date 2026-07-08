package httpapi

import (
	"backend/internal/store"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

type bulkDocumentsRequest struct {
	DocumentIDs []string `json:"document_ids"`
}

type bulkRedactionsRequest struct {
	RedactionIDs []string `json:"redaction_ids"`
}

type manualRedactionRequest struct {
	Start        *int    `json:"start"`
	End          *int    `json:"end"`
	Type         *string `json:"type"`
	Reason       *string `json:"reason"`
	SelectedText *string `json:"selected_text"`
}

type server struct {
	logger *slog.Logger
	store  *store.Store
}

// NewRouter builds the HTTP API router for the backend service.
func NewRouter(logger *slog.Logger, store *store.Store) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	srv := &server{
		logger: logger,
		store:  store,
	}

	router := gin.New()
	router.Use(srv.requestLogger())
	router.Use(srv.recovery())

	router.GET("/healthz", srv.healthz)
	router.GET("/api/batch/summary", srv.batchSummary)
	router.GET("/api/documents", srv.listDocuments)
	router.GET("/api/documents/:id", srv.getDocument)
	router.GET("/api/documents/:id/redactions", srv.listDocumentRedactions)
	router.GET("/api/documents/:id/review-summary", srv.getDocumentReviewSummary)
	router.POST("/api/uploads/documents", srv.uploadDocuments)
	router.POST("/api/documents/:id/redactions", srv.addManualRedaction)
	router.POST("/api/redactions/:id/accept", srv.acceptRedaction)
	router.POST("/api/redactions/:id/reject", srv.rejectRedaction)
	router.POST("/api/redactions/bulk-accept", srv.bulkAcceptRedactions)
	router.POST("/api/redactions/bulk-reject", srv.bulkRejectRedactions)
	router.POST("/api/documents/:id/approve", srv.approveDocument)
	router.POST("/api/documents/bulk-approve", srv.bulkApproveDocuments)
	router.POST("/api/documents/:id/retry", srv.retryDocument)
	router.POST("/api/documents/bulk-retry", srv.bulkRetryDocuments)
	router.POST("/api/export", srv.exportApprovedDocuments)
	router.GET("/api/export/latest", srv.latestExport)

	return router
}

func (s *server) healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *server) batchSummary(c *gin.Context) {
	c.JSON(http.StatusOK, s.store.Summary())
}
