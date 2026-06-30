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
	maxUploadFiles      = 600
	maxUploadFileBytes  = 10 * 1024 * 1024
	maxUploadTotalBytes = 40 * 1024 * 1024
)

var (
	workerPool *WorkerPool
)

var (
	emailPattern          = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	phoneIntlPattern      = regexp.MustCompile(`\+91[\s-]?[6-9]\d{4}[\s-]?\d{5}\b`)
	phonePattern          = regexp.MustCompile(`\b[6-9]\d{4}[\s-]?\d{5}\b`)
	phoneUKPattern        = regexp.MustCompile(`(?:^|[^0-9A-Za-z])(\+44[\s-]?(?:\d[\s-]?){9,10}\d)(?:$|[^0-9])`)
	panPattern            = regexp.MustCompile(`\b[A-Z]{5}[0-9]{4}[A-Z]\b`)
	casePattern           = regexp.MustCompile(`\bCASE-\d{4}-[A-Z]{2,8}-\d{3,6}\b`)
	clientPattern         = regexp.MustCompile(`\bCLIENT-\d{4}-\d{3,6}\b`)
	accountPattern        = regexp.MustCompile(`(?i)\b(?:Account(?: Number)?|Acct|Bank Account|IOLTA Acct)\s*(?:No\.?|Number|#|:)?\s*([0-9][0-9 -]{5,20}[0-9])\b`)
	routingPattern        = regexp.MustCompile(`(?i)\b(?:Routing(?: Transit)?(?: Number)?|RTN)\s*(?:No\.?|Number|#|:|\()?[\s#:-]*([0-9]{9})\b`)
	addressPattern        = regexp.MustCompile(`(?i)\b\d{1,5}\s+(?:[A-Z0-9][A-Za-z0-9.'\-]*\s){1,6}(?:Street|St\.?|Avenue|Ave\.?|Road|Rd\.?|Lane|Ln\.?|Drive|Dr\.?|Boulevard|Blvd\.?|Court|Ct\.?|Circle|Cir\.?|Way|Parkway|Pkwy\.?|Place|Pl\.?|Terrace|Ter\.?)(?:,|\s)+(?:((?:Apt|Apartment|Suite|Ste|Unit|Floor|Fl)\.?\s*[A-Za-z0-9-]+)(?:,|\s)+)?(?:[A-Za-z]+(?:\s+[A-Za-z]+){0,2})\,\s+(?:[A-Z]{2}|[A-Za-z]+(?:\s+[A-Za-z]+){0,2})\s+\d{5}(?:-\d{4})?\b`)
	ssnPattern            = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	einPattern            = regexp.MustCompile(`\b\d{2}-\d{7}\b`)
	itinPattern           = regexp.MustCompile(`\b9\d{2}-[78]\d{1}-\d{4}\b`)
	creditCardPattern     = regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4,7}\b`)
	usPhonePattern        = regexp.MustCompile(`(?:^|[^0-9A-Za-z])((?:\+?1[-.\s]?)?\(?[2-9]\d{2}\)?[-.\s]?\d{3}[-.\s]?\d{4})(?:$|[^0-9])`)
	macPattern            = regexp.MustCompile(`\b(?:[0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}\b`)
	ipv4Pattern           = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`)
	ibanPattern           = regexp.MustCompile(`(?i)\b[A-Z]{2}\d{2}(?:[ -]?[A-Z0-9]{4}){2,6}(?:[ -]?[A-Z0-9]{1,3})?\b`)
	swiftPattern          = regexp.MustCompile(`\b[A-Z]{4}[A-Z]{2}(?:\d[A-Z0-9]|[A-Z0-9]\d)(?:XXX)?\b`)
	aadhaarPattern        = regexp.MustCompile(`\b\d{4}\s\d{4}\s\d{4}\b`)
	mrnPattern            = regexp.MustCompile(`\bMRN[-:]\s?\d{6,12}\b`)
	patientIDPattern      = regexp.MustCompile(`\bPAT[-:]\d{4}[-:]\d{6,10}\b`)
	awsKeyPattern         = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	githubTokenPattern    = regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,255}\b`)
	stripeKeyPattern      = regexp.MustCompile(`\bsk_(?:live|test)_[A-Za-z0-9]{24,}\b`)
	dobPattern            = regexp.MustCompile(`(?i)\b(?:DOB|Date of Birth)\s*(?:No\.?|Number|#|:)?\s*((?:\d{1,2}[/-]\d{1,2}[/-]\d{2,4})|(?:\d{4}[/-]\d{2}[/-]\d{2}))\b`)
	passportPattern       = regexp.MustCompile(`(?i)\bPassport(?:\s*(?:No\.?|Number|#|:))?\s*([A-Z0-9]{7,9})\b`)
	driverLicensePattern  = regexp.MustCompile(`(?i)\b(?:DL|Driver'?s License|Driver License)\s*(?:No\.?|Number|#|:)?\s*([A-Z0-9-]{6,20})\b`)
	npiPattern            = regexp.MustCompile(`(?i)\bNPI\s*(?:No\.?|Number|#|:)?\s*(\d{10})\b`)
	deaPattern            = regexp.MustCompile(`(?i)\bDEA\s*(?:No\.?|Number|#|:)?\s*([A-Z]{2}\d{7})\b`)
	medicalLicensePattern = regexp.MustCompile(`(?i)\b(?:Med(?:ical)? Lic(?:ense)?|Medical License)\s*(?:No\.?|Number|#|:)?\s*([A-Z0-9-]{5,20})\b`)
)

type runtimeDetection struct {
	Start           int
	End             int
	Text            string
	Type            string
	Confidence      *float64
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
		case "PERSON", "PAN_LIKE_ID", "BANK_ACCOUNT", "CLIENT_ID", "CASE_ID",
			"SSN", "EIN", "ITIN", "CREDIT_CARD", "US_PHONE", "IP_ADDRESS",
			"AADHAAR", "PATIENT_ID", "API_KEY", "IBAN", "SWIFT_BIC",
			"US_DRIVER_LICENSE", "MEDICAL_LICENSE", "ROUTING_NUMBER",
			"PASSPORT", "DEA", "NPI", "DOB":
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
	return detectRuntimeRedactionsWithOptions(text, runtimeDetectionOptions{
		includeGLiNEROwned: true,
		includeRegexOwned:  true,
	})
}

func detectRuntimeRedactionsWithOptions(text string, options runtimeDetectionOptions) []runtimeDetection {
	candidates := make([]runtimeDetection, 0, 24)
	if options.includeGLiNEROwned {
		candidates = append(candidates,
			findRuntimeDetections(text, emailPattern, "EMAIL", nil, "runtime_regex", "ACCEPTED", "Detected email-like token with a strong regex pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, phoneIntlPattern, "PHONE", nil, "runtime_regex", "ACCEPTED", "Detected phone-like token with a strong regex pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, phonePattern, "PHONE", nil, "runtime_regex", "ACCEPTED", "Detected phone-like token with a strong regex pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetectionsWithSubmatch(text, phoneUKPattern, 1, "PHONE", nil, "runtime_regex", "ACCEPTED", "Detected international phone-like token with a strong regex pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, addressPattern, "ADDRESS", nil, "runtime_regex", "ACCEPTED", "Detected address-like phrase with numeric street context")...,
		)
		candidates = append(candidates,
			findRuntimeDetectionsWithSubmatch(text, usPhonePattern, 1, "US_PHONE", nil, "runtime_regex", "ACCEPTED", "Detected US/NA phone number pattern")...,
		)
	}
	if options.includeRegexOwned {
		candidates = append(candidates,
			findRuntimeDetections(text, panPattern, "PAN_LIKE_ID", nil, "runtime_regex", "ACCEPTED", "Detected PAN-like identifier pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, casePattern, "CASE_ID", nil, "runtime_regex", "ACCEPTED", "Detected case identifier pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, clientPattern, "CLIENT_ID", nil, "runtime_regex", "ACCEPTED", "Detected client identifier pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetectionsWithSubmatch(text, accountPattern, 1, "BANK_ACCOUNT", nil, "runtime_regex", "ACCEPTED", "Detected account-like identifier pattern in labeled financial text")...,
		)
		candidates = append(candidates,
			findRuntimeDetectionsWithSubmatch(text, routingPattern, 1, "ROUTING_NUMBER", nil, "runtime_regex", "ACCEPTED", "Detected routing-transit style number in labeled financial text")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, ssnPattern, "SSN", nil, "runtime_regex", "ACCEPTED", "Detected US Social Security Number pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, einPattern, "EIN", nil, "runtime_regex", "ACCEPTED", "Detected US Employer ID Number pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, itinPattern, "ITIN", nil, "runtime_regex", "ACCEPTED", "Detected US Individual Taxpayer ID pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, creditCardPattern, "CREDIT_CARD", nil, "runtime_regex", "ACCEPTED", "Detected credit card number pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, macPattern, "MAC_ADDRESS", nil, "runtime_regex", "ACCEPTED", "Detected MAC address pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, ipv4Pattern, "IP_ADDRESS", nil, "runtime_regex", "ACCEPTED", "Detected IPv4 address pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, ibanPattern, "IBAN", nil, "runtime_regex", "ACCEPTED", "Detected IBAN code pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, swiftPattern, "SWIFT_BIC", nil, "runtime_regex", "ACCEPTED", "Detected SWIFT/BIC code pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, aadhaarPattern, "AADHAAR", nil, "runtime_regex", "ACCEPTED", "Detected Indian Aadhaar number pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, mrnPattern, "MRN", nil, "runtime_regex", "ACCEPTED", "Detected medical record number pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, patientIDPattern, "PATIENT_ID", nil, "runtime_regex", "ACCEPTED", "Detected patient identifier pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, awsKeyPattern, "API_KEY", nil, "runtime_regex", "ACCEPTED", "Detected AWS access key pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, githubTokenPattern, "API_KEY", nil, "runtime_regex", "ACCEPTED", "Detected GitHub token pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetections(text, stripeKeyPattern, "API_KEY", nil, "runtime_regex", "ACCEPTED", "Detected Stripe API key pattern")...,
		)
		candidates = append(candidates,
			findRuntimeDetectionsWithSubmatch(text, dobPattern, 1, "DOB", nil, "runtime_regex", "ACCEPTED", "Detected date-of-birth value in a labeled context")...,
		)
		candidates = append(candidates,
			findRuntimeDetectionsWithSubmatch(text, passportPattern, 1, "PASSPORT", nil, "runtime_regex", "ACCEPTED", "Detected passport identifier in a labeled context")...,
		)
		candidates = append(candidates,
			findRuntimeDetectionsWithSubmatch(text, driverLicensePattern, 1, "US_DRIVER_LICENSE", nil, "runtime_regex", "ACCEPTED", "Detected driver license identifier in a labeled context")...,
		)
		candidates = append(candidates,
			findRuntimeDetectionsWithSubmatch(text, npiPattern, 1, "NPI", nil, "runtime_regex", "ACCEPTED", "Detected healthcare provider identifier in a labeled context")...,
		)
		candidates = append(candidates,
			findRuntimeDetectionsWithSubmatch(text, deaPattern, 1, "DEA", nil, "runtime_regex", "ACCEPTED", "Detected DEA identifier in a labeled context")...,
		)
		candidates = append(candidates,
			findRuntimeDetectionsWithSubmatch(text, medicalLicensePattern, 1, "MEDICAL_LICENSE", nil, "runtime_regex", "ACCEPTED", "Detected medical license identifier in a labeled context")...,
		)
	}

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

func findRuntimeDetections(text string, pattern *regexp.Regexp, piiType string, confidence *float64, source, suggestedStatus, reason string) []runtimeDetection {
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

func findRuntimeDetectionsWithSubmatch(text string, pattern *regexp.Regexp, group int, piiType string, confidence *float64, source, suggestedStatus, reason string) []runtimeDetection {
	matches := pattern.FindAllStringSubmatchIndex(text, -1)
	detections := make([]runtimeDetection, 0, len(matches))
	for _, match := range matches {
		startIndex := group * 2
		if len(match) <= startIndex+1 {
			continue
		}
		startByte := match[startIndex]
		endByte := match[startIndex+1]
		if startByte < 0 || endByte < 0 {
			continue
		}

		start := byteIndexToRuneIndex(text, startByte)
		end := byteIndexToRuneIndex(text, endByte)
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
