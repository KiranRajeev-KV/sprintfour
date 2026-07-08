package httpapi

import (
	doc "backend/internal/document"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *server) listDocuments(c *gin.Context) {
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

func (s *server) getDocument(c *gin.Context) {
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

func (s *server) listDocumentRedactions(c *gin.Context) {
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

func (s *server) getDocumentReviewSummary(c *gin.Context) {
	summary, err := s.store.ReviewSummary(c.Param("id"))
	if err != nil {
		s.writeMutationError(c, err)
		return
	}
	c.JSON(http.StatusOK, summary)
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
