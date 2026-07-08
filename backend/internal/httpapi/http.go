package httpapi

import (
	doc "backend/internal/document"
	"backend/internal/store"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

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

type Server struct {
	logger *slog.Logger
	store  *store.Store
}

func NewRouter(logger *slog.Logger, store *store.Store) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	server := &Server{
		logger: logger,
		store:  store,
	}

	router := gin.New()
	router.Use(server.requestLogger())
	router.Use(server.recovery())

	router.GET("/healthz", server.healthz)
	router.GET("/api/batch/summary", server.batchSummary)
	router.GET("/api/documents", server.listDocuments)
	router.GET("/api/documents/:id", server.getDocument)
	router.GET("/api/documents/:id/redactions", server.listDocumentRedactions)
	router.GET("/api/documents/:id/review-summary", server.getDocumentReviewSummary)
	router.POST("/api/uploads/documents", server.uploadDocuments)
	router.POST("/api/documents/:id/redactions", server.addManualRedaction)
	router.POST("/api/redactions/:id/accept", server.acceptRedaction)
	router.POST("/api/redactions/:id/reject", server.rejectRedaction)
	router.POST("/api/redactions/bulk-accept", server.bulkAcceptRedactions)
	router.POST("/api/redactions/bulk-reject", server.bulkRejectRedactions)
	router.POST("/api/documents/:id/approve", server.approveDocument)
	router.POST("/api/documents/bulk-approve", server.bulkApproveDocuments)
	router.POST("/api/documents/:id/retry", server.retryDocument)
	router.POST("/api/documents/bulk-retry", server.bulkRetryDocuments)
	router.POST("/api/export", server.exportApprovedDocuments)
	router.GET("/api/export/latest", server.latestExport)

	return router
}

func (s *Server) healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) batchSummary(c *gin.Context) {
	c.JSON(http.StatusOK, s.store.Summary())
}

func (s *Server) listDocuments(c *gin.Context) {
	limit, err := parsePositiveInt(c.Query("limit"), 50, 200)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_query", err.Error())
		return
	}
	offset, err := parsePositiveInt(c.Query("offset"), 0, 1_000_000)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_query", err.Error())
		return
	}

	status := c.Query("status")
	if status != "" && !isAllowedStatusFilter(status) {
		writeError(c, http.StatusBadRequest, "invalid_query", "invalid status filter")
		return
	}
	risk := c.Query("risk")
	if risk != "" && !isAllowedRiskFilter(risk) {
		writeError(c, http.StatusBadRequest, "invalid_query", "invalid risk filter")
		return
	}

	items, total := s.store.Documents(status, risk, c.Query("q"), limit, offset)
	responseItems := make([]gin.H, 0, len(items))
	for _, document := range items {
		responseItems = append(responseItems, documentListItem(document))
	}

	c.JSON(http.StatusOK, gin.H{
		"items":  responseItems,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) getDocument(c *gin.Context) {
	documentID := c.Param("id")
	document, ok := s.store.DocumentByID(documentID)
	if !ok {
		writeError(c, http.StatusNotFound, "not_found", "document not found")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":                       document.ID,
		"title":                    document.Title,
		"source":                   document.Source,
		"source_file":              document.SourceFile,
		"text":                     document.Text,
		"char_count":               document.CharCount,
		"status":                   doc.NormalizeStatus(document.Status),
		"risk_level":               doc.NormalizeRisk(document.RiskLevel),
		"failure_hint":             document.FailureHint,
		"redaction_count":          document.RedactionCount,
		"low_confidence_count":     document.LowConfidenceCount,
		"retry_count":              document.RetryCount,
		"pending_redaction_count":  document.PendingRedactionCount,
		"accepted_redaction_count": document.AcceptedRedactionCount,
		"rejected_redaction_count": document.RejectedRedactionCount,
		"added_redaction_count":    document.AddedRedactionCount,
		"blocking_review_count":    document.BlockingReviewCount,
		"can_approve":              document.CanApprove,
	})
}

func (s *Server) listDocumentRedactions(c *gin.Context) {
	documentID := c.Param("id")
	if _, ok := s.store.DocumentByID(documentID); !ok {
		writeError(c, http.StatusNotFound, "not_found", "document not found")
		return
	}

	redactions := s.store.RedactionsByDocumentID(documentID)
	items := make([]gin.H, 0, len(redactions))
	for _, redaction := range redactions {
		items = append(items, gin.H{
			"id":               redaction.ID,
			"document_id":      redaction.DocumentID,
			"start":            redaction.Start,
			"end":              redaction.End,
			"text":             redaction.Text,
			"type":             redaction.Type,
			"confidence":       redaction.Confidence,
			"reason":           redaction.Reason,
			"source":           redaction.Source,
			"suggested_status": redaction.SuggestedStatus,
			"is_ground_truth":  redaction.IsGroundTruth,
			"review_state":     redaction.ReviewState,
			"reviewed_at":      redaction.ReviewedAt,
			"reviewed_by":      redaction.ReviewedBy,
			"is_user_added":    redaction.IsUserAdded,
			"created_at":       redaction.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"document_id": documentID,
		"items":       items,
		"total":       len(items),
	})
}

func (s *Server) getDocumentReviewSummary(c *gin.Context) {
	summary, err := s.store.ReviewSummary(c.Param("id"))
	if err != nil {
		s.writeMutationError(c, err)
		return
	}
	c.JSON(http.StatusOK, summary)
}

func (s *Server) uploadDocuments(c *gin.Context) {
	startedAt := time.Now()
	requestID := c.GetHeader("X-Request-Id")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, doc.MaxUploadTotalBytes+(512*1024))
	s.logger.Info("upload_request_started",
		slog.String("request_id", requestID),
		slog.String("method", c.Request.Method),
		slog.String("path", c.FullPath()),
		slog.Int64("max_upload_total_bytes", doc.MaxUploadTotalBytes),
		slog.Int("max_upload_files", doc.MaxUploadFiles),
	)
	reader, err := c.Request.MultipartReader()
	if err != nil {
		s.logger.Error("upload_multipart_reader_failed",
			slog.String("request_id", requestID),
			slog.String("error", err.Error()),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
		writeError(c, http.StatusBadRequest, "invalid_body", "multipart form data is required")
		return
	}

	mode := "replace"
	totalBytes := int64(0)
	uploadedCount := 0
	rejectedCount := 0
	acceptedInputs := make([]doc.UploadedDocumentInput, 0, 64)
	items := make([]doc.UploadItemResult, 0, 64)

	for {
		partStartedAt := time.Now()
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			s.logger.Info("upload_parts_complete",
				slog.String("request_id", requestID),
				slog.String("mode", mode),
				slog.Int("uploaded", uploadedCount),
				slog.Int("accepted", len(acceptedInputs)),
				slog.Int("rejected", rejectedCount),
				slog.Int64("total_bytes", totalBytes),
				slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
			)
			break
		}
		if err != nil {
			s.logger.Error("upload_next_part_failed",
				slog.String("request_id", requestID),
				slog.Int("uploaded", uploadedCount),
				slog.Int64("total_bytes", totalBytes),
				slog.String("error", err.Error()),
				slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
			)
			writeError(c, http.StatusBadRequest, "upload_too_large", "upload exceeds the maximum allowed size")
			return
		}
		if part.FormName() == "mode" {
			content, readErr := io.ReadAll(io.LimitReader(part, 64))
			part.Close()
			if readErr != nil {
				s.logger.Error("upload_mode_read_failed",
					slog.String("request_id", requestID),
					slog.String("error", readErr.Error()),
					slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
				)
				writeError(c, http.StatusBadRequest, "invalid_mode", "upload mode must be replace or append")
				return
			}
			mode = doc.NormalizeUploadMode(string(content))
			s.logger.Info("upload_mode_read",
				slog.String("request_id", requestID),
				slog.String("mode", mode),
				slog.Int64("part_duration_ms", time.Since(partStartedAt).Milliseconds()),
			)
			continue
		}
		if part.FormName() != "files" {
			part.Close()
			continue
		}

		uploadedCount++
		if uploadedCount > doc.MaxUploadFiles {
			part.Close()
			s.logger.Error("upload_file_limit_exceeded",
				slog.String("request_id", requestID),
				slog.Int("uploaded", uploadedCount),
				slog.Int("max_upload_files", doc.MaxUploadFiles),
				slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
			)
			writeError(c, http.StatusBadRequest, "too_many_files", "too many files uploaded")
			return
		}

		input, item, size := readUploadedTXTPart(part)
		totalBytes += size
		part.Close()
		if totalBytes > doc.MaxUploadTotalBytes {
			s.logger.Error("upload_total_bytes_exceeded",
				slog.String("request_id", requestID),
				slog.Int("uploaded", uploadedCount),
				slog.Int64("total_bytes", totalBytes),
				slog.Int64("max_upload_total_bytes", doc.MaxUploadTotalBytes),
				slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
			)
			writeError(c, http.StatusBadRequest, "upload_too_large", "upload exceeds the maximum allowed size")
			return
		}
		items = append(items, item)
		if item.Accepted {
			acceptedInputs = append(acceptedInputs, input)
		} else {
			rejectedCount++
		}
		if uploadedCount <= 5 || uploadedCount%25 == 0 || !item.Accepted {
			s.logger.Info("upload_file_processed",
				slog.String("request_id", requestID),
				slog.Int("uploaded", uploadedCount),
				slog.String("filename", item.Filename),
				slog.Bool("accepted", item.Accepted),
				slog.String("reason", item.Reason),
				slog.Int64("file_bytes", size),
				slog.Int64("total_bytes", totalBytes),
				slog.Int64("part_duration_ms", time.Since(partStartedAt).Milliseconds()),
				slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
			)
		}
	}

	if uploadedCount == 0 {
		writeError(c, http.StatusBadRequest, "invalid_body", "at least one file is required")
		return
	}
	if mode != "replace" && mode != "append" {
		s.logger.Error("upload_invalid_mode",
			slog.String("request_id", requestID),
			slog.String("mode", mode),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
		writeError(c, http.StatusBadRequest, "invalid_mode", "upload mode must be replace or append")
		return
	}

	storeStartedAt := time.Now()
	result, err := s.store.UploadDocuments(mode, acceptedInputs, time.Now())
	if err != nil {
		s.logger.Error("upload_store_failed",
			slog.String("request_id", requestID),
			slog.String("mode", mode),
			slog.Int("accepted", len(acceptedInputs)),
			slog.String("error", err.Error()),
			slog.Int64("store_duration_ms", time.Since(storeStartedAt).Milliseconds()),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
		s.writeMutationError(c, err)
		return
	}

	result.Uploaded = uploadedCount
	result.Accepted = len(acceptedInputs)
	result.Rejected = uploadedCount - len(acceptedInputs)
	result.Items = mergeUploadItemResults(items, result.Items)

	s.logger.Info("documents_uploaded",
		slog.String("request_id", requestID),
		slog.String("batch_id", result.BatchID),
		slog.String("mode", result.Mode),
		slog.Int("uploaded", result.Uploaded),
		slog.Int("accepted", result.Accepted),
		slog.Int("rejected", result.Rejected),
		slog.Int("documents_created", result.DocumentsCreated),
		slog.Int("redactions_created", result.RedactionsCreated),
		slog.Int64("total_bytes", totalBytes),
		slog.Int64("store_duration_ms", time.Since(storeStartedAt).Milliseconds()),
		slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
	)
	c.JSON(http.StatusOK, result)
}

func (s *Server) addManualRedaction(c *gin.Context) {
	var request manualRedactionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_body", "invalid request body")
		return
	}
	if request.Start == nil || request.End == nil || request.Type == nil {
		writeError(c, http.StatusBadRequest, "invalid_body", "start, end, and type are required")
		return
	}

	result, err := s.store.AddManualRedaction(c.Param("id"), store.ManualRedactionInput{
		Start:        *request.Start,
		End:          *request.End,
		Type:         valueOrEmpty(request.Type),
		Reason:       valueOrEmpty(request.Reason),
		SelectedText: valueOrEmpty(request.SelectedText),
	}, time.Now())
	if err != nil {
		s.writeMutationError(c, err)
		return
	}

	s.logger.Info("redaction_added",
		slog.String("document_id", result.DocumentID),
		slog.String("redaction_id", result.ID),
		slog.String("type", result.Type),
		slog.Int("start", result.Start),
		slog.Int("end", result.End),
	)
	c.JSON(http.StatusOK, result)
}

func (s *Server) acceptRedaction(c *gin.Context) {
	result, err := s.store.AcceptRedaction(c.Param("id"), time.Now())
	if err != nil {
		s.writeMutationError(c, err)
		return
	}
	s.logger.Info("redaction_accepted",
		slog.String("redaction_id", result.RedactionID),
		slog.String("document_id", result.DocumentID),
		slog.String("previous_state", result.PreviousState),
		slog.String("review_state", result.ReviewState),
		slog.Bool("changed", result.Changed),
	)
	c.JSON(http.StatusOK, result)
}

func (s *Server) rejectRedaction(c *gin.Context) {
	result, err := s.store.RejectRedaction(c.Param("id"), time.Now())
	if err != nil {
		s.writeMutationError(c, err)
		return
	}
	s.logger.Info("redaction_rejected",
		slog.String("redaction_id", result.RedactionID),
		slog.String("document_id", result.DocumentID),
		slog.String("previous_state", result.PreviousState),
		slog.String("review_state", result.ReviewState),
		slog.Bool("changed", result.Changed),
	)
	c.JSON(http.StatusOK, result)
}

func (s *Server) bulkAcceptRedactions(c *gin.Context) {
	request, ok := decodeBulkRedactionsRequest(c)
	if !ok {
		return
	}

	result, err := s.store.BulkAcceptRedactions(request.RedactionIDs, time.Now())
	if err != nil {
		s.writeMutationError(c, err)
		return
	}

	s.logger.Info("redactions_bulk_accepted",
		slog.Int("requested", result.Requested),
		slog.Int("accepted", result.Accepted),
		slog.Int("skipped", result.Skipped),
	)
	c.JSON(http.StatusOK, result)
}

func (s *Server) bulkRejectRedactions(c *gin.Context) {
	request, ok := decodeBulkRedactionsRequest(c)
	if !ok {
		return
	}

	result, err := s.store.BulkRejectRedactions(request.RedactionIDs, time.Now())
	if err != nil {
		s.writeMutationError(c, err)
		return
	}

	s.logger.Info("redactions_bulk_rejected",
		slog.Int("requested", result.Requested),
		slog.Int("rejected", result.Rejected),
		slog.Int("skipped", result.Skipped),
	)
	c.JSON(http.StatusOK, result)
}

func (s *Server) approveDocument(c *gin.Context) {
	result, err := s.store.ApproveDocument(c.Param("id"))
	if err != nil {
		s.writeMutationError(c, err)
		return
	}

	s.logger.Info("document_approved",
		slog.String("document_id", result.DocumentID),
		slog.String("previous_status", result.PreviousStatus),
		slog.String("status", result.Status),
		slog.Bool("changed", result.Changed),
	)
	c.JSON(http.StatusOK, result)
}

func (s *Server) bulkApproveDocuments(c *gin.Context) {
	request, ok := decodeBulkDocumentsRequest(c)
	if !ok {
		return
	}

	result := s.store.BulkApprove(request.DocumentIDs)
	s.logger.Info("documents_bulk_approved",
		slog.Int("requested", result.Requested),
		slog.Int("approved", result.Approved),
		slog.Int("skipped", result.Skipped),
	)
	c.JSON(http.StatusOK, result)
}

func (s *Server) retryDocument(c *gin.Context) {
	result, err := s.store.RetryDocument(c.Param("id"))
	if err != nil {
		s.writeMutationError(c, err)
		return
	}

	s.logger.Info("document_retried",
		slog.String("document_id", result.DocumentID),
		slog.String("previous_status", result.PreviousStatus),
		slog.String("status", result.Status),
		slog.Bool("changed", result.Changed),
		slog.Int("retry_count", result.RetryCount),
	)
	c.JSON(http.StatusOK, result)
}

func (s *Server) bulkRetryDocuments(c *gin.Context) {
	request, ok := decodeBulkDocumentsRequest(c)
	if !ok {
		return
	}

	result := s.store.BulkRetry(request.DocumentIDs)
	s.logger.Info("documents_bulk_retried",
		slog.Int("requested", result.Requested),
		slog.Int("retried", result.Retried),
		slog.Int("skipped", result.Skipped),
	)
	c.JSON(http.StatusOK, result)
}

func (s *Server) exportApprovedDocuments(c *gin.Context) {
	summary, changed, err := s.store.ExportApprovedDocuments(time.Now())
	if err != nil {
		s.logger.Error("export_failed", slog.String("error", err.Error()))
		writeError(c, http.StatusInternalServerError, "internal_error", "export failed")
		return
	}

	s.logger.Info("documents_exported",
		slog.String("export_id", summary.ExportID),
		slog.Bool("changed", changed),
		slog.Int("exported_documents", summary.ExportedDocuments),
		slog.Int("skipped_documents", summary.SkippedDocuments),
		slog.Int("needs_review", summary.NeedsReview),
		slog.Int("failed", summary.Failed),
		slog.Int("ready", summary.Ready),
		slog.Int("approved_blocked_by_review", summary.ApprovedBlockedByReview),
		slog.Int("applied_redactions", summary.AppliedRedactions),
		slog.Int("skipped_pending_redactions", summary.SkippedPending),
		slog.Int("skipped_rejected_redactions", summary.SkippedRejected),
		slog.Int("skipped_overlap_redactions", summary.SkippedOverlap),
	)
	c.JSON(http.StatusOK, summary)
}

func (s *Server) latestExport(c *gin.Context) {
	summary, ok := s.store.LatestExport()
	if !ok {
		c.JSON(http.StatusOK, gin.H{"has_export": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"has_export":                  true,
		"export_id":                   summary.ExportID,
		"exported_documents":          summary.ExportedDocuments,
		"skipped_documents":           summary.SkippedDocuments,
		"needs_review":                summary.NeedsReview,
		"failed":                      summary.Failed,
		"ready":                       summary.Ready,
		"approved_blocked_by_review":  summary.ApprovedBlockedByReview,
		"applied_redactions":          summary.AppliedRedactions,
		"skipped_pending_redactions":  summary.SkippedPending,
		"skipped_rejected_redactions": summary.SkippedRejected,
		"skipped_overlap_redactions":  summary.SkippedOverlap,
		"output_dir":                  summary.OutputDir,
		"files":                       summary.Files,
		"created_at":                  summary.CreatedAt,
	})
}

func (s *Server) requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)

		requestID := c.GetHeader("X-Request-Id")
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		s.logger.Info("http_request",
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Int("status", c.Writer.Status()),
			slog.Int64("duration_ms", duration.Milliseconds()),
			slog.String("request_id", requestID),
		)
	}
}

func (s *Server) recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		s.logger.Error("panic_recovered",
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Any("panic", recovered),
		)
		writeError(c, http.StatusInternalServerError, "internal_error", "internal server error")
	})
}

func (s *Server) writeMutationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, store.ErrDocumentNotFound):
		writeError(c, http.StatusNotFound, "not_found", "document not found")
	case errors.Is(err, store.ErrRedactionNotFound):
		writeError(c, http.StatusNotFound, "not_found", "redaction not found")
	default:
		var validationErr *store.ValidationError
		if errors.As(err, &validationErr) {
			writeError(c, http.StatusBadRequest, validationErr.Code, validationErr.Message)
			return
		}
		var conflictErr *store.StateConflictError
		if errors.As(err, &conflictErr) {
			code := conflictErr.Code
			if code == "" {
				code = "invalid_state"
			}
			writeError(c, http.StatusConflict, code, conflictErr.Error())
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func documentListItem(document doc.DocumentSnapshot) gin.H {
	return gin.H{
		"id":                      document.ID,
		"title":                   document.Title,
		"source":                  document.Source,
		"source_file":             document.SourceFile,
		"status":                  doc.NormalizeStatus(document.Status),
		"risk_level":              doc.NormalizeRisk(document.RiskLevel),
		"char_count":              document.CharCount,
		"pii_count":               document.PIICount,
		"low_confidence_count":    document.LowConfidenceCount,
		"failure_hint":            document.FailureHint,
		"retry_count":             document.RetryCount,
		"pending_redaction_count": document.PendingRedactionCount,
		"blocking_review_count":   document.BlockingReviewCount,
		"can_approve":             document.CanApprove,
	}
}

func decodeBulkDocumentsRequest(c *gin.Context) (bulkDocumentsRequest, bool) {
	var request bulkDocumentsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_body", "invalid request body")
		return bulkDocumentsRequest{}, false
	}

	cleanedIDs := make([]string, 0, len(request.DocumentIDs))
	for _, documentID := range request.DocumentIDs {
		documentID = strings.TrimSpace(documentID)
		if documentID == "" {
			writeError(c, http.StatusBadRequest, "invalid_body", "document_ids must not contain empty values")
			return bulkDocumentsRequest{}, false
		}
		cleanedIDs = append(cleanedIDs, documentID)
	}
	request.DocumentIDs = cleanedIDs
	return request, true
}

func decodeBulkRedactionsRequest(c *gin.Context) (bulkRedactionsRequest, bool) {
	var request bulkRedactionsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_body", "invalid request body")
		return bulkRedactionsRequest{}, false
	}

	cleanedIDs := make([]string, 0, len(request.RedactionIDs))
	for _, redactionID := range request.RedactionIDs {
		redactionID = strings.TrimSpace(redactionID)
		if redactionID == "" {
			writeError(c, http.StatusBadRequest, "invalid_body", "redaction_ids must not contain empty values")
			return bulkRedactionsRequest{}, false
		}
		cleanedIDs = append(cleanedIDs, redactionID)
	}
	request.RedactionIDs = cleanedIDs
	return request, true
}

func writeError(c *gin.Context, statusCode int, code, message string) {
	c.AbortWithStatusJSON(statusCode, doc.APIErrorEnvelope{
		Error: doc.APIError{
			Code:    code,
			Message: message,
		},
	})
}

func parsePositiveInt(raw string, defaultValue, maxValue int) (int, error) {
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if value < 0 {
		return 0, strconv.ErrSyntax
	}
	if value > maxValue {
		return maxValue, nil
	}
	return value, nil
}

func isAllowedStatusFilter(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "READY", "NEEDS_REVIEW", "FAILED", "CLEAN", "APPROVED", "EXPORTED",
		"QUEUED", "PROCESSING":
		return true
	default:
		return false
	}
}

func isAllowedRiskFilter(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "LOW", "MEDIUM", "HIGH", "UNKNOWN":
		return true
	default:
		return false
	}
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func readUploadedTXTPart(part *multipart.Part) (doc.UploadedDocumentInput, doc.UploadItemResult, int64) {
	filename, relativePath := uploadNames(part.FileName())
	item := doc.UploadItemResult{
		Filename:     filename,
		RelativePath: doc.NullableString(relativePath),
		Accepted:     false,
	}

	if !strings.EqualFold(path.Ext(filename), ".txt") {
		item.Reason = "only_txt_supported"
		_, _ = io.Copy(io.Discard, part)
		return doc.UploadedDocumentInput{}, item, 0
	}

	content, err := io.ReadAll(io.LimitReader(part, doc.MaxUploadFileBytes+1))
	if err != nil {
		item.Reason = "read_failed"
		return doc.UploadedDocumentInput{}, item, 0
	}
	size := int64(len(content))
	if len(content) == 0 {
		item.Reason = "empty_file"
		return doc.UploadedDocumentInput{}, item, size
	}
	if len(content) > doc.MaxUploadFileBytes {
		item.Reason = "file_too_large"
		_, _ = io.Copy(io.Discard, part)
		return doc.UploadedDocumentInput{}, item, size
	}

	text := doc.NormalizeUploadedText(string(content))
	if strings.TrimSpace(text) == "" {
		item.Reason = "empty_file"
		return doc.UploadedDocumentInput{}, item, size
	}

	item.Accepted = true
	item.Reason = "uploaded"
	return doc.UploadedDocumentInput{
		Filename:     filename,
		RelativePath: relativePath,
		Text:         text,
	}, item, size
}

func uploadNames(raw string) (filename string, relativePath string) {
	normalized := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if normalized == "" {
		return "upload.txt", ""
	}
	base := path.Base(normalized)
	if base == "." || base == "/" || base == "" {
		base = "upload.txt"
	}
	if normalized == base {
		return base, ""
	}
	return base, normalized
}

func mergeUploadItemResults(items []doc.UploadItemResult, accepted []doc.UploadItemResult) []doc.UploadItemResult {
	acceptedByFilename := make(map[string][]doc.UploadItemResult, len(accepted))
	for _, item := range accepted {
		key := item.Filename + "|" + doc.ValueOrDefault(item.RelativePath, "")
		acceptedByFilename[key] = append(acceptedByFilename[key], item)
	}

	merged := make([]doc.UploadItemResult, 0, len(items))
	for _, item := range items {
		key := item.Filename + "|" + doc.ValueOrDefault(item.RelativePath, "")
		matches := acceptedByFilename[key]
		if len(matches) == 0 {
			merged = append(merged, item)
			continue
		}
		merged = append(merged, matches[0])
		if len(matches) == 1 {
			delete(acceptedByFilename, key)
		} else {
			acceptedByFilename[key] = matches[1:]
		}
	}
	return merged
}
