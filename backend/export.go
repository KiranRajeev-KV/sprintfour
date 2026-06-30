package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var exportOutputDir = filepath.Clean("../exported")

func writeExportedDocuments(documents []ExportedDocument) error {
	if err := os.RemoveAll(exportOutputDir); err != nil {
		return fmt.Errorf("reset export directory: %w", err)
	}
	if err := os.MkdirAll(exportOutputDir, 0o755); err != nil {
		return fmt.Errorf("create export directory: %w", err)
	}

	usedNames := make(map[string]struct{}, len(documents))
	for index, document := range documents {
		filename := buildExportFilename(document.SourceFile, document.DocumentID)
		if _, exists := usedNames[filename]; exists {
			filename = buildCollisionSafeExportFilename(document.SourceFile, document.DocumentID)
		}
		usedNames[filename] = struct{}{}
		documents[index].OutputFilename = filename

		outputPath := filepath.Join(exportOutputDir, filename)
		if err := os.WriteFile(outputPath, []byte(document.RedactedText), 0o644); err != nil {
			return fmt.Errorf("write export file %s: %w", filename, err)
		}
	}

	return nil
}

func buildExportFilename(sourceFile, documentID string) string {
	base := filepath.Base(strings.ReplaceAll(sourceFile, "\\", "/"))
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = documentID + ".txt"
	}
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if name == "" {
		name = documentID
	}
	if ext == "" {
		ext = ".txt"
	}
	return sanitizeFilename(name) + "_redacted" + ext
}

func buildCollisionSafeExportFilename(sourceFile, documentID string) string {
	base := filepath.Base(strings.ReplaceAll(sourceFile, "\\", "/"))
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if name == "" {
		name = documentID
	}
	if ext == "" {
		ext = ".txt"
	}
	return sanitizeFilename(name) + "_" + sanitizeFilename(documentID) + "_redacted" + ext
}

func sanitizeFilename(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "document"
	}
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	value = replacer.Replace(value)
	value = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		case r == ' ':
			return '_'
		default:
			return '_'
		}
	}, value)
	value = strings.Trim(value, "._")
	if value == "" {
		return "document"
	}
	return value
}
