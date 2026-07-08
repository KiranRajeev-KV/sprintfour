package httpapi

import (
	doc "backend/internal/document"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *server) uploadDocuments(c *gin.Context) {
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
