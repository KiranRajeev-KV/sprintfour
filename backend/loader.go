package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const minimumDocumentCount = 200

type rawDocument struct {
	ID                   *string `json:"id"`
	Title                *string `json:"title"`
	Source               *string `json:"source"`
	SourceFile           *string `json:"source_file"`
	Text                 *string `json:"text"`
	CharCount            *int    `json:"char_count"`
	SyntheticPIIInjected *bool   `json:"synthetic_pii_injected"`
	WorkflowHint         *string `json:"workflow_hint"`
	RiskLevelHint        *string `json:"risk_level_hint"`
	FailureHint          *string `json:"failure_hint"`
}

type rawRedaction struct {
	ID              *string  `json:"id"`
	DocumentID      *string  `json:"document_id"`
	Start           *int     `json:"start"`
	End             *int     `json:"end"`
	Text            *string  `json:"text"`
	Type            *string  `json:"type"`
	Confidence      *float64 `json:"confidence"`
	Reason          *string  `json:"reason"`
	Source          *string  `json:"source"`
	SuggestedStatus *string  `json:"suggested_status"`
	IsGroundTruth   *bool    `json:"is_ground_truth"`
}

func datasetPaths() (documentsPath string, redactionsPath string) {
	return filepath.Clean("../dataset/processed/documents_seed.jsonl"), filepath.Clean("../dataset/processed/mock_redactions.jsonl")
}

func LoadDataset(documentsPath, redactionsPath string) ([]*Document, []*Redaction, error) {
	documents, err := loadDocuments(documentsPath)
	if err != nil {
		return nil, nil, err
	}

	redactions, err := loadRedactions(redactionsPath, documents)
	if err != nil {
		return nil, nil, err
	}

	return documents, redactions, nil
}

func loadDocuments(path string) ([]*Document, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("documents seed file missing: %s", path)
		}
		return nil, fmt.Errorf("open documents seed file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)

	documents := make([]*Document, 0, minimumDocumentCount)
	seenIDs := make(map[string]struct{})
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var raw rawDocument
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			return nil, fmt.Errorf("invalid documents JSONL at line %d: %w", lineNumber, err)
		}

		document, err := raw.toDocument(lineNumber)
		if err != nil {
			return nil, err
		}

		if _, exists := seenIDs[document.ID]; exists {
			return nil, fmt.Errorf("duplicate document id %q at line %d", document.ID, lineNumber)
		}
		seenIDs[document.ID] = struct{}{}
		documents = append(documents, document)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read documents seed file: %w", err)
	}

	if len(documents) < minimumDocumentCount {
		return nil, fmt.Errorf("expected at least %d documents, found %d", minimumDocumentCount, len(documents))
	}

	return documents, nil
}

func loadRedactions(path string, documents []*Document) ([]*Redaction, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("mock redactions file missing: %s", path)
		}
		return nil, fmt.Errorf("open mock redactions file: %w", err)
	}
	defer file.Close()

	documentsByID := make(map[string]*Document, len(documents))
	for _, document := range documents {
		documentsByID[document.ID] = document
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	redactions := make([]*Redaction, 0, 256)
	seenIDs := make(map[string]struct{})
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var raw rawRedaction
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			return nil, fmt.Errorf("invalid redactions JSONL at line %d: %w", lineNumber, err)
		}

		redaction, err := raw.toRedaction(lineNumber)
		if err != nil {
			return nil, err
		}

		if _, exists := seenIDs[redaction.ID]; exists {
			return nil, fmt.Errorf("duplicate redaction id %q at line %d", redaction.ID, lineNumber)
		}
		seenIDs[redaction.ID] = struct{}{}

		document, exists := documentsByID[redaction.DocumentID]
		if !exists {
			return nil, fmt.Errorf("redaction %q references unknown document %q", redaction.ID, redaction.DocumentID)
		}
		if redaction.Start < 0 || redaction.End <= redaction.Start {
			return nil, fmt.Errorf("redaction %q has out-of-bounds span [%d,%d) for document %q", redaction.ID, redaction.Start, redaction.End, redaction.DocumentID)
		}
		spanText, err := substringByRuneIndex(document.Text, redaction.Start, redaction.End)
		if err != nil {
			return nil, fmt.Errorf("redaction %q has out-of-bounds span [%d,%d) for document %q", redaction.ID, redaction.Start, redaction.End, redaction.DocumentID)
		}
		if spanText != redaction.Text {
			return nil, fmt.Errorf("redaction %q span text mismatch for document %q", redaction.ID, redaction.DocumentID)
		}

		redactions = append(redactions, redaction)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read mock redactions file: %w", err)
	}

	return redactions, nil
}

func (r rawDocument) toDocument(lineNumber int) (*Document, error) {
	missingField := func(name string) error {
		return fmt.Errorf("documents line %d missing required field %q", lineNumber, name)
	}

	switch {
	case r.ID == nil:
		return nil, missingField("id")
	case r.Title == nil:
		return nil, missingField("title")
	case r.Source == nil:
		return nil, missingField("source")
	case r.SourceFile == nil:
		return nil, missingField("source_file")
	case r.Text == nil:
		return nil, missingField("text")
	case r.CharCount == nil:
		return nil, missingField("char_count")
	}

	status := normalizeStatus(valueOrDefault(r.WorkflowHint, "READY"))
	riskLevel := normalizeRisk(valueOrDefault(r.RiskLevelHint, "UNKNOWN"))

	document := &Document{
		ID:                   strings.TrimSpace(*r.ID),
		Title:                *r.Title,
		Source:               *r.Source,
		SourceFile:           *r.SourceFile,
		Text:                 *r.Text,
		CharCount:            *r.CharCount,
		SyntheticPIIInjected: valueOrBoolDefault(r.SyntheticPIIInjected, false),
		Status:               status,
		RiskLevel:            riskLevel,
		FailureHint:          r.FailureHint,
	}

	if document.ID == "" {
		return nil, fmt.Errorf("documents line %d has empty document id", lineNumber)
	}

	return document, nil
}

func (r rawRedaction) toRedaction(lineNumber int) (*Redaction, error) {
	missingField := func(name string) error {
		return fmt.Errorf("redactions line %d missing required field %q", lineNumber, name)
	}

	switch {
	case r.ID == nil:
		return nil, missingField("id")
	case r.DocumentID == nil:
		return nil, missingField("document_id")
	case r.Start == nil:
		return nil, missingField("start")
	case r.End == nil:
		return nil, missingField("end")
	case r.Text == nil:
		return nil, missingField("text")
	case r.Type == nil:
		return nil, missingField("type")
	case r.Confidence == nil:
		return nil, missingField("confidence")
	case r.Reason == nil:
		return nil, missingField("reason")
	case r.Source == nil:
		return nil, missingField("source")
	case r.SuggestedStatus == nil:
		return nil, missingField("suggested_status")
	case r.IsGroundTruth == nil:
		return nil, missingField("is_ground_truth")
	}

	redaction := &Redaction{
		ID:              strings.TrimSpace(*r.ID),
		DocumentID:      strings.TrimSpace(*r.DocumentID),
		Start:           *r.Start,
		End:             *r.End,
		Text:            *r.Text,
		Type:            *r.Type,
		Confidence:      *r.Confidence,
		Reason:          *r.Reason,
		Source:          *r.Source,
		SuggestedStatus: *r.SuggestedStatus,
		IsGroundTruth:   *r.IsGroundTruth,
	}

	if redaction.ID == "" {
		return nil, fmt.Errorf("redactions line %d has empty redaction id", lineNumber)
	}
	if redaction.DocumentID == "" {
		return nil, fmt.Errorf("redactions line %d has empty document id", lineNumber)
	}

	return redaction, nil
}

func valueOrDefault(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}

func valueOrBoolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func normalizeStatus(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "READY", "NEEDS_REVIEW", "FAILED", "CLEAN":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return "READY"
	}
}

func normalizeRisk(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "LOW", "MEDIUM", "HIGH":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return "UNKNOWN"
	}
}
