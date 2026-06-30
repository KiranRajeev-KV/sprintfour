package main

import "testing"

func TestDetectRuntimeRedactionsRegexOwnedExcludesGLiNEROwnedLabels(t *testing.T) {
	text := "Jane Smith lives at 742 Willow Creek Drive, Apt 3B, Springfield, IL 62704. Email jane@example.com. Routing Number 021000021."

	detections := detectRuntimeRedactionsRegexOwned(text)

	assertNoDetectionType(t, detections, "EMAIL")
	assertNoDetectionType(t, detections, "ADDRESS")
	assertNoDetectionType(t, detections, "PHONE")
	assertDetection(t, detections, "ROUTING_NUMBER", "021000021")
}

func TestMergeRuntimeDetectionsPrefersRegexOwnedStructuredLabels(t *testing.T) {
	merged := mergeRuntimeDetections(
		[]runtimeDetection{{
			Start:      10,
			End:        19,
			Text:       "021000021",
			Type:       "ROUTING_NUMBER",
			Confidence: float64Pointer(0.61),
			Source:     "gliner_local",
		}},
		[]runtimeDetection{{
			Start:      10,
			End:        19,
			Text:       "021000021",
			Type:       "ROUTING_NUMBER",
			Confidence: nil,
			Source:     "runtime_regex",
		}},
	)

	if len(merged) != 1 {
		t.Fatalf("expected 1 merged detection, got %d", len(merged))
	}
	if merged[0].Source != "runtime_regex" {
		t.Fatalf("expected regex detection to win, got %+v", merged[0])
	}
}

func TestMergeRuntimeDetectionsPrefersGLiNERPhoneOverRegexPhoneOverlap(t *testing.T) {
	merged := mergeRuntimeDetections(
		[]runtimeDetection{{
			Start:      5,
			End:        19,
			Text:       "(408) 555-0199",
			Type:       "PHONE",
			Confidence: float64Pointer(0.91),
			Source:     "gliner_local",
		}},
		[]runtimeDetection{{
			Start:      5,
			End:        19,
			Text:       "(408) 555-0199",
			Type:       "US_PHONE",
			Confidence: nil,
			Source:     "runtime_regex",
		}},
	)

	if len(merged) != 1 {
		t.Fatalf("expected 1 merged detection, got %d", len(merged))
	}
	if merged[0].Type != "PHONE" || merged[0].Source != "gliner_local" {
		t.Fatalf("expected GLiNER phone detection to win, got %+v", merged[0])
	}
}

func assertNoDetectionType(t *testing.T, detections []runtimeDetection, blockedType string) {
	t.Helper()

	for _, detection := range detections {
		if detection.Type == blockedType {
			t.Fatalf("unexpected detection type %s in %+v", blockedType, detection)
		}
	}
}
