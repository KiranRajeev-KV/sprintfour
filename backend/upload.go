package main

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	maxUploadFiles      = 300
	maxUploadFileBytes  = 2 * 1024 * 1024
	maxUploadTotalBytes = 20 * 1024 * 1024
)

var (
	emailPattern     = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	phoneIntlPattern = regexp.MustCompile(`\+91[\s-]?[6-9]\d{4}[\s-]?\d{5}\b`)
	phonePattern     = regexp.MustCompile(`\b[6-9]\d{4}[\s-]?\d{5}\b`)
	panPattern       = regexp.MustCompile(`\b[A-Z]{5}[0-9]{4}[A-Z]\b`)
	casePattern      = regexp.MustCompile(`\bCASE-\d{4}-[A-Z]{2,8}-\d{3,6}\b`)
	clientPattern    = regexp.MustCompile(`\bCLIENT-\d{4}-\d{3,6}\b`)
	accountPattern   = regexp.MustCompile(`\bACCT-[A-Z0-9]{2,8}-[A-Z0-9]{3,8}\b`)
	addressPattern   = regexp.MustCompile(`(?i)\b\d{1,4}\s+[A-Z0-9][A-Za-z0-9.\- ]{0,64}\s(?:Road|Rd|Street|St|Lane|Ln|Avenue|Ave|Nagar|Layout|Main|Cross|Phase),?\s+(?:Coimbatore|Chennai|Bengaluru|Bangalore|Hyderabad|Mumbai|Delhi|Pune|Tamil Nadu|Karnataka|Kerala|Telangana)(?:\s+\d{6})?\b`)
)

type runtimeDetection struct {
	Start           int
	End             int
	Text            string
	Type            string
	Confidence      float64
	Reason          string
	Source          string
	SuggestedStatus string
}

func normalizeUploadMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "replace"
	}
	return value
}

func titleFromFilename(filename string) string {
	base := path.Base(strings.ReplaceAll(filename, "\\", "/"))
	trimmed := strings.TrimSuffix(base, path.Ext(base))
	if trimmed == "" {
		return base
	}
	return trimmed
}

func nullableString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func formatBatchID(sequence int) string {
	return fmt.Sprintf("batch_%06d", sequence)
}

func formatUploadDocumentID(sequence int) string {
	return fmt.Sprintf("upload_%06d", sequence)
}

func formatGeneratedRedactionID(sequence int) string {
	return fmt.Sprintf("runtime_red_%06d", sequence)
}

func parseUploadDocumentSequence(id string) (int, bool) {
	return parseNumericSuffix(id, "upload_")
}

func parseGeneratedRedactionSequence(id string) (int, bool) {
	return parseNumericSuffix(id, "runtime_red_")
}

func parseManualRedactionSequence(id string) (int, bool) {
	return parseNumericSuffix(id, "user_red_")
}

func parseNumericSuffix(value, prefix string) (int, bool) {
	if !strings.HasPrefix(value, prefix) {
		return 0, false
	}
	parsed, err := strconv.Atoi(strings.TrimPrefix(value, prefix))
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func normalizeUploadedText(text string) string {
	text = strings.TrimPrefix(text, "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}

func classifyUploadedDocument(redactions []*Redaction) (string, string) {
	if len(redactions) == 0 {
		return "CLEAN", "LOW"
	}

	hasPending := false
	hasSensitive := false
	for _, redaction := range redactions {
		if initialReviewState(redaction) == "PENDING" {
			hasPending = true
		}
		switch redaction.Type {
		case "PAN_LIKE_ID", "BANK_ACCOUNT", "CLIENT_ID", "CASE_ID":
			hasSensitive = true
		}
	}

	if hasPending {
		if len(redactions) >= 3 || hasSensitive {
			return "NEEDS_REVIEW", "HIGH"
		}
		return "NEEDS_REVIEW", "MEDIUM"
	}

	if len(redactions) >= 4 || hasSensitive {
		return "READY", "MEDIUM"
	}
	return "READY", "LOW"
}

func detectRuntimeRedactions(text string) []runtimeDetection {
	candidates := make([]runtimeDetection, 0, 8)
	candidates = append(candidates,
		findRuntimeDetections(text, emailPattern, "EMAIL", 0.99, "runtime_regex", "ACCEPTED", "Detected email-like token with a strong regex pattern")...,
	)
	candidates = append(candidates,
		findRuntimeDetections(text, phoneIntlPattern, "PHONE", 0.96, "runtime_regex", "ACCEPTED", "Detected phone-like token with a strong regex pattern")...,
	)
	candidates = append(candidates,
		findRuntimeDetections(text, phonePattern, "PHONE", 0.96, "runtime_regex", "ACCEPTED", "Detected phone-like token with a strong regex pattern")...,
	)
	candidates = append(candidates,
		findRuntimeDetections(text, panPattern, "PAN_LIKE_ID", 0.98, "runtime_regex", "ACCEPTED", "Detected PAN-like identifier pattern")...,
	)
	candidates = append(candidates,
		findRuntimeDetections(text, casePattern, "CASE_ID", 0.95, "runtime_regex", "ACCEPTED", "Detected case identifier pattern")...,
	)
	candidates = append(candidates,
		findRuntimeDetections(text, clientPattern, "CLIENT_ID", 0.95, "runtime_regex", "ACCEPTED", "Detected client identifier pattern")...,
	)
	candidates = append(candidates,
		findRuntimeDetections(text, accountPattern, "BANK_ACCOUNT", 0.94, "runtime_regex", "ACCEPTED", "Detected account-like identifier pattern")...,
	)
	candidates = append(candidates,
		findRuntimeDetections(text, addressPattern, "ADDRESS", 0.64, "runtime_regex", "REVIEW", "Detected address-like phrase with numeric street context")...,
	)

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Start == candidates[j].Start {
			return candidates[i].End > candidates[j].End
		}
		return candidates[i].Start < candidates[j].Start
	})

	filtered := make([]runtimeDetection, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Text) == "" {
			continue
		}
		if len(filtered) == 0 {
			filtered = append(filtered, candidate)
			continue
		}
		last := filtered[len(filtered)-1]
		if candidate.Start < last.End {
			if candidate.End-candidate.Start > last.End-last.Start {
				filtered[len(filtered)-1] = candidate
			}
			continue
		}
		filtered = append(filtered, candidate)
	}

	return filtered
}

func findRuntimeDetections(text string, pattern *regexp.Regexp, piiType string, confidence float64, source, suggestedStatus, reason string) []runtimeDetection {
	matches := pattern.FindAllStringIndex(text, -1)
	detections := make([]runtimeDetection, 0, len(matches))
	for _, match := range matches {
		start := byteIndexToRuneIndex(text, match[0])
		end := byteIndexToRuneIndex(text, match[1])
		spanText, err := substringByRuneIndex(text, start, end)
		if err != nil {
			continue
		}
		detections = append(detections, runtimeDetection{
			Start:           start,
			End:             end,
			Text:            spanText,
			Type:            piiType,
			Confidence:      confidence,
			Reason:          reason,
			Source:          source,
			SuggestedStatus: suggestedStatus,
		})
	}
	return detections
}

func byteIndexToRuneIndex(text string, byteIndex int) int {
	if byteIndex <= 0 {
		return 0
	}
	if byteIndex >= len(text) {
		return len([]rune(text))
	}
	return len([]rune(text[:byteIndex]))
}
