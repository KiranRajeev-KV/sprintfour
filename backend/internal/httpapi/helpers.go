package httpapi

import (
	doc "backend/internal/document"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

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
