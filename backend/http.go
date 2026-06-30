package main

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

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
		responseItems = append(responseItems, gin.H{
			"id":                   document.ID,
			"title":                document.Title,
			"source":               document.Source,
			"source_file":          document.SourceFile,
			"status":               normalizeStatus(document.Status),
			"risk_level":           normalizeRisk(document.RiskLevel),
			"char_count":           document.CharCount,
			"pii_count":            document.PIICount,
			"low_confidence_count": document.LowConfidenceCount,
			"failure_hint":         document.FailureHint,
		})
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
		"id":                   document.ID,
		"title":                document.Title,
		"source":               document.Source,
		"source_file":          document.SourceFile,
		"text":                 document.Text,
		"char_count":           document.CharCount,
		"status":               normalizeStatus(document.Status),
		"risk_level":           normalizeRisk(document.RiskLevel),
		"failure_hint":         document.FailureHint,
		"redaction_count":      document.RedactionCount,
		"low_confidence_count": document.LowConfidenceCount,
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
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"document_id": documentID,
		"items":       items,
		"total":       len(items),
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
	case "READY", "NEEDS_REVIEW", "FAILED", "CLEAN":
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
