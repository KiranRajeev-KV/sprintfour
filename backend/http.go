package main

import (
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

type manualRedactionRequest struct {
	Start        *int    `json:"start"`
	End          *int    `json:"end"`
	Type         *string `json:"type"`
	Reason       *string `json:"reason"`
	SelectedText *string `json:"selected_text"`
}

type Server struct {
	logger *slog.Logger
	store  *Store
}

func NewRouter(logger *slog.Logger, store *Store) *gin.Engine {
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
		"status":                   normalizeStatus(document.Status),
		"risk_level":               normalizeRisk(document.RiskLevel),
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
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadTotalBytes+(512*1024))
	if err := c.Request.ParseMultipartForm(maxUploadTotalBytes); err != nil {
		writeError(c, http.StatusBadRequest, "upload_too_large", "upload exceeds the maximum allowed size")
		return
	}

	form := c.Request.MultipartForm
	if form == nil || len(form.File["files"]) == 0 {
		writeError(c, http.StatusBadRequest, "invalid_body", "at least one file is required")
		return
	}

	mode := normalizeUploadMode(c.PostForm("mode"))
	if mode != "replace" && mode != "append" {
		writeError(c, http.StatusBadRequest, "invalid_mode", "upload mode must be replace or append")
		return
	}

	files := form.File["files"]
	if len(files) > maxUploadFiles {
		writeError(c, http.StatusBadRequest, "too_many_files", "too many files uploaded")
		return
	}

	totalBytes := int64(0)
	acceptedInputs := make([]UploadedDocumentInput, 0, len(files))
	items := make([]UploadItemResult, 0, len(files))

	for _, header := range files {
		totalBytes += header.Size
		if totalBytes > maxUploadTotalBytes {
			writeError(c, http.StatusBadRequest, "upload_too_large", "upload exceeds the maximum allowed size")
			return
		}

		input, item := readUploadedTXTFile(header)
		items = append(items, item)
		if item.Accepted {
			acceptedInputs = append(acceptedInputs, input)
		}
	}

	result, err := s.store.UploadDocuments(mode, acceptedInputs, time.Now())
	if err != nil {
		s.writeMutationError(c, err)
		return
	}

	result.Uploaded = len(files)
	result.Accepted = len(acceptedInputs)
	result.Rejected = len(files) - len(acceptedInputs)
	result.Items = mergeUploadItemResults(items, result.Items)

	s.logger.Info("documents_uploaded",
		slog.String("batch_id", result.BatchID),
		slog.String("mode", result.Mode),
		slog.Int("uploaded", result.Uploaded),
		slog.Int("accepted", result.Accepted),
		slog.Int("rejected", result.Rejected),
		slog.Int("documents_created", result.DocumentsCreated),
		slog.Int("redactions_created", result.RedactionsCreated),
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

	result, err := s.store.AddManualRedaction(c.Param("id"), manualRedactionInput{
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
	case errors.Is(err, ErrDocumentNotFound):
		writeError(c, http.StatusNotFound, "not_found", "document not found")
	case errors.Is(err, ErrRedactionNotFound):
		writeError(c, http.StatusNotFound, "not_found", "redaction not found")
	default:
		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			writeError(c, http.StatusBadRequest, validationErr.Code, validationErr.Message)
			return
		}
		var conflictErr *StateConflictError
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

func documentListItem(document DocumentSnapshot) gin.H {
	return gin.H{
		"id":                      document.ID,
		"title":                   document.Title,
		"source":                  document.Source,
		"source_file":             document.SourceFile,
		"status":                  normalizeStatus(document.Status),
		"risk_level":              normalizeRisk(document.RiskLevel),
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

func writeError(c *gin.Context, statusCode int, code, message string) {
	c.AbortWithStatusJSON(statusCode, APIErrorEnvelope{
		Error: APIError{
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
	case "READY", "NEEDS_REVIEW", "FAILED", "CLEAN", "APPROVED", "EXPORTED":
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

func readUploadedTXTFile(header *multipart.FileHeader) (UploadedDocumentInput, UploadItemResult) {
	filename, relativePath := uploadNames(header.Filename)
	item := UploadItemResult{
		Filename:     filename,
		RelativePath: nullableString(relativePath),
		Accepted:     false,
	}

	if !strings.EqualFold(path.Ext(filename), ".txt") {
		item.Reason = "only_txt_supported"
		return UploadedDocumentInput{}, item
	}
	if header.Size <= 0 {
		item.Reason = "empty_file"
		return UploadedDocumentInput{}, item
	}
	if header.Size > maxUploadFileBytes {
		item.Reason = "file_too_large"
		return UploadedDocumentInput{}, item
	}

	file, err := header.Open()
	if err != nil {
		item.Reason = "read_failed"
		return UploadedDocumentInput{}, item
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxUploadFileBytes+1))
	if err != nil {
		item.Reason = "read_failed"
		return UploadedDocumentInput{}, item
	}
	if len(content) == 0 {
		item.Reason = "empty_file"
		return UploadedDocumentInput{}, item
	}
	if len(content) > maxUploadFileBytes {
		item.Reason = "file_too_large"
		return UploadedDocumentInput{}, item
	}

	text := normalizeUploadedText(string(content))
	if strings.TrimSpace(text) == "" {
		item.Reason = "empty_file"
		return UploadedDocumentInput{}, item
	}

	item.Accepted = true
	item.Reason = "uploaded"
	return UploadedDocumentInput{
		Filename:     filename,
		RelativePath: relativePath,
		Text:         text,
	}, item
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

func mergeUploadItemResults(items []UploadItemResult, accepted []UploadItemResult) []UploadItemResult {
	acceptedByFilename := make(map[string][]UploadItemResult, len(accepted))
	for _, item := range accepted {
		key := item.Filename + "|" + valueOrDefault(item.RelativePath, "")
		acceptedByFilename[key] = append(acceptedByFilename[key], item)
	}

	merged := make([]UploadItemResult, 0, len(items))
	for _, item := range items {
		key := item.Filename + "|" + valueOrDefault(item.RelativePath, "")
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
