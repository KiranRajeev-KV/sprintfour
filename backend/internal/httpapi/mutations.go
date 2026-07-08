package httpapi

import (
	"backend/internal/store"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *server) addManualRedaction(c *gin.Context) {
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

func (s *server) acceptRedaction(c *gin.Context) {
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

func (s *server) rejectRedaction(c *gin.Context) {
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

func (s *server) bulkAcceptRedactions(c *gin.Context) {
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

func (s *server) bulkRejectRedactions(c *gin.Context) {
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

func (s *server) approveDocument(c *gin.Context) {
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

func (s *server) bulkApproveDocuments(c *gin.Context) {
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

func (s *server) retryDocument(c *gin.Context) {
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

func (s *server) bulkRetryDocuments(c *gin.Context) {
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

func (s *server) exportApprovedDocuments(c *gin.Context) {
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

func (s *server) latestExport(c *gin.Context) {
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
