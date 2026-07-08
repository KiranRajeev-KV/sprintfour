package store

import (
	doc "backend/internal/document"
	"strings"
	"time"
)

func (s *Store) UploadDocuments(mode string, inputs []UploadedDocumentInput, now time.Time) (UploadBatchResult, error) {
	s.mu.Lock()

	mode = doc.NormalizeUploadMode(mode)
	if mode != "replace" && mode != "append" {
		s.mu.Unlock()
		return UploadBatchResult{}, &ValidationError{
			Code:    "invalid_mode",
			Message: "upload mode must be replace or append",
		}
	}

	if mode == "replace" {
		s.resetForUploadLocked()
	}

	s.uploadBatchSeq++
	result := UploadBatchResult{
		BatchID:  doc.FormatBatchID(s.uploadBatchSeq),
		Mode:     mode,
		Uploaded: len(inputs),
		Items:    make([]UploadItemResult, 0, len(inputs)),
	}

	queuedDocs := make([]queuedDocument, 0, len(inputs))
	for _, input := range inputs {
		createdDoc := s.CreateUploadDocument(input)

		queuedDocs = append(queuedDocs, queuedDocument{
			documentID: createdDoc.ID,
			text:       createdDoc.Text,
		})
		result.Accepted++
		result.DocumentsCreated++

		status := "QUEUED"
		risk := "UNKNOWN"
		documentID := createdDoc.ID
		relativePath := doc.NullableString(input.RelativePath)
		result.Items = append(result.Items, UploadItemResult{
			Filename:       input.Filename,
			RelativePath:   relativePath,
			DocumentID:     &documentID,
			Status:         &status,
			RiskLevel:      &risk,
			RedactionCount: 0,
			Accepted:       true,
			Reason:         "uploaded",
		})
	}

	s.mu.Unlock()

	s.submitQueuedDocuments(queuedDocs)

	result.Rejected = result.Uploaded - result.Accepted
	return result, nil
}

func (s *Store) CreateUploadDocument(input UploadedDocumentInput) *Document {
	s.uploadDocumentSeq++
	documentID := doc.FormatUploadDocumentID(s.uploadDocumentSeq)
	sourceFile := input.Filename
	if strings.TrimSpace(input.RelativePath) != "" {
		sourceFile = input.RelativePath
	}
	createdDoc := &Document{
		ID:                   documentID,
		Title:                doc.TitleFromFilename(input.Filename),
		Source:               "USER_UPLOAD_TXT",
		SourceFile:           sourceFile,
		Text:                 input.Text,
		CharCount:            len([]rune(input.Text)),
		SyntheticPIIInjected: false,
		Status:               "QUEUED",
		RiskLevel:            "UNKNOWN",
	}
	s.documents = append(s.documents, createdDoc)
	s.documentsByID[createdDoc.ID] = createdDoc
	s.documentRuneLengths[createdDoc.ID] = len([]rune(createdDoc.Text))
	s.runtimeByDocID[createdDoc.ID] = DocumentRuntimeState{
		Status: "QUEUED",
	}
	return createdDoc
}

func (s *Store) submitQueuedDocuments(documents []queuedDocument) {
	s.mu.RLock()
	submitter := s.submitter
	s.mu.RUnlock()

	if submitter == nil {
		return
	}
	for _, document := range documents {
		submitter.Submit(document.documentID, document.text)
	}
}
