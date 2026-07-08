package detector

import (
	"backend/internal/document"
	"context"
	"log/slog"
	"sort"
)

type Detector interface {
	Detect(ctx context.Context, documentID, text string) ([]document.RuntimeDetection, error)
}

type Config struct {
	GLiNEREnabled        bool
	GLiNERURL            string
	GLiNERTimeoutMS      int
	GLiNERMaxConcurrency int
}

type runtimeDetector struct {
	logger       *slog.Logger
	glinerClient *glinerClient
}

func NewRuntimeDetector(logger *slog.Logger, cfg Config) Detector {
	detector := &runtimeDetector{
		logger: logger,
	}
	if cfg.GLiNEREnabled {
		detector.glinerClient = newGLiNERClient(
			logger,
			cfg.GLiNERURL,
			cfg.GLiNERTimeoutMS,
			cfg.GLiNERMaxConcurrency,
		)
	}
	return detector
}

func (d *runtimeDetector) Detect(ctx context.Context, documentID, text string) ([]document.RuntimeDetection, error) {
	if d.glinerClient == nil {
		return document.DetectRuntimeRedactions(text), nil
	}

	glinerDetections, err := d.glinerClient.Detect(ctx, documentID, text)
	if err != nil {
		d.logger.Warn("gliner_detect_failed_fallback",
			slog.String("document_id", documentID),
			slog.String("error", err.Error()),
		)
		return document.DetectRuntimeRedactions(text), nil
	}

	regexDetections := detectRuntimeRedactionsRegexOwned(text)
	return MergeRuntimeDetections(glinerDetections, regexDetections), nil
}

func detectRuntimeRedactionsRegexOwned(text string) []document.RuntimeDetection {
	return document.DetectRuntimeRedactionsWithOptions(text, document.RuntimeDetectionOptions{
		IncludeGLiNEROwned: false,
		IncludeRegexOwned:  true,
	})
}

func MergeRuntimeDetections(primary []document.RuntimeDetection, secondary []document.RuntimeDetection) []document.RuntimeDetection {
	combined := make([]document.RuntimeDetection, 0, len(primary)+len(secondary))
	combined = append(combined, primary...)
	combined = append(combined, secondary...)

	sort.Slice(combined, func(i, j int) bool {
		if combined[i].Start == combined[j].Start {
			if combined[i].End == combined[j].End {
				return compareRuntimeDetections(combined[i], combined[j]) < 0
			}
			return combined[i].End > combined[j].End
		}
		return combined[i].Start < combined[j].Start
	})

	filtered := make([]document.RuntimeDetection, 0, len(combined))
	for _, candidate := range combined {
		if candidate.Start >= candidate.End {
			continue
		}

		replaced := false
		discarded := false
		for i := range filtered {
			existing := filtered[i]
			if !sameSemanticCategory(existing.Type, candidate.Type) {
				continue
			}
			if !overlapsHeavily(existing, candidate) {
				continue
			}

			if compareRuntimeDetections(candidate, existing) < 0 {
				filtered[i] = candidate
				replaced = true
			} else {
				discarded = true
			}
			break
		}
		if replaced || discarded {
			continue
		}

		filtered = append(filtered, candidate)
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Start == filtered[j].Start {
			return filtered[i].End > filtered[j].End
		}
		return filtered[i].Start < filtered[j].Start
	})

	return filtered
}

func compareRuntimeDetections(left, right document.RuntimeDetection) int {
	leftPriority := runtimeDetectionPriority(left)
	rightPriority := runtimeDetectionPriority(right)
	if leftPriority != rightPriority {
		return rightPriority - leftPriority
	}
	leftConfidence := runtimeDetectionConfidenceValue(left.Confidence)
	rightConfidence := runtimeDetectionConfidenceValue(right.Confidence)
	if leftConfidence != rightConfidence {
		if leftConfidence > rightConfidence {
			return -1
		}
		return 1
	}
	leftLength := left.End - left.Start
	rightLength := right.End - right.Start
	if leftLength != rightLength {
		if leftLength > rightLength {
			return -1
		}
		return 1
	}
	return 0
}

func runtimeDetectionConfidenceValue(value *float64) float64 {
	if value == nil {
		return -1
	}
	return *value
}

func runtimeDetectionPriority(detection document.RuntimeDetection) int {
	switch {
	case isRegexOwnedLabel(detection.Type):
		if detection.Source == "runtime_regex" {
			return 4
		}
		return 3
	case isGLiNEROwnedLabel(detection.Type):
		if detection.Source == "gliner_local" {
			return 2
		}
		return 1
	default:
		return 0
	}
}

func sameSemanticCategory(left, right string) bool {
	return normalizeSemanticLabel(left) == normalizeSemanticLabel(right)
}

func normalizeSemanticLabel(label string) string {
	switch label {
	case "US_PHONE":
		return "PHONE"
	default:
		return label
	}
}

func overlapsHeavily(left, right document.RuntimeDetection) bool {
	start := max(left.Start, right.Start)
	end := min(left.End, right.End)
	if start >= end {
		return false
	}

	intersection := end - start
	shorter := min(left.End-left.Start, right.End-right.Start)
	return float64(intersection)/float64(shorter) >= 0.5
}

func isRegexOwnedLabel(label string) bool {
	switch label {
	case "SSN", "EIN", "ITIN", "PAN_LIKE_ID", "AADHAAR", "MRN", "PATIENT_ID",
		"API_KEY", "IBAN", "SWIFT_BIC", "ROUTING_NUMBER", "BANK_ACCOUNT",
		"CASE_ID", "CLIENT_ID", "PASSPORT", "US_DRIVER_LICENSE", "NPI", "DEA",
		"MEDICAL_LICENSE", "CREDIT_CARD", "DOB", "IP_ADDRESS", "MAC_ADDRESS":
		return true
	default:
		return false
	}
}

func isGLiNEROwnedLabel(label string) bool {
	switch label {
	case "PERSON", "ADDRESS", "EMAIL", "PHONE", "ORGANIZATION_CONTACT":
		return true
	default:
		return false
	}
}
