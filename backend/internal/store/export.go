package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var exportOutputDir = defaultExportOutputDir()

func ExportOutputDir() string {
	return exportOutputDir
}

func SetExportOutputDir(dir string) {
	exportOutputDir = dir
}

func defaultExportOutputDir() string {
	workingDir, err := os.Getwd()
	if err != nil {
		return filepath.Clean("../exported")
	}

	for dir := workingDir; ; dir = filepath.Dir(dir) {
		if isRepoRoot(dir) {
			return filepath.Join(dir, "exported")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	if filepath.Base(workingDir) == "backend" {
		return filepath.Join(filepath.Dir(workingDir), "exported")
	}
	return filepath.Join(workingDir, "exported")
}

func isRepoRoot(dir string) bool {
	if info, err := os.Stat(filepath.Join(dir, "backend")); err != nil || !info.IsDir() {
		return false
	}
	if info, err := os.Stat(filepath.Join(dir, "frontend")); err != nil || !info.IsDir() {
		return false
	}
	return true
}

func writeExportedDocuments(documents []ExportedDocument) error {
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
