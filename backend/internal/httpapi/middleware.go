package httpapi

import (
	"backend/internal/store"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *server) requestLogger() gin.HandlerFunc {
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

func (s *server) recovery() gin.HandlerFunc {
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

func (s *server) writeMutationError(c *gin.Context, err error) {
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
