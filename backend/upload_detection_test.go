package main

import (
	"os"
	"strings"
	"testing"
)

func TestDetectRuntimeRedactionsCapturesLabeledStructuredPII(t *testing.T) {
	textBytes, err := os.ReadFile("../dataset/raw/manual_synthetic_txt/dense_pii.txt")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}

	detections := detectRuntimeRedactions(string(textBytes))

	assertDetection(t, detections, "DOB", "01/15/1980")
	assertDetection(t, detections, "ADDRESS", "742 Willow Creek Drive, Apt 3B, Springfield, IL 62704")
	assertDetection(t, detections, "BANK_ACCOUNT", "100092837465")
	assertDetection(t, detections, "ROUTING_NUMBER", "021000021")
	assertDetection(t, detections, "PASSPORT", "561234567")
	assertDetection(t, detections, "US_DRIVER_LICENSE", "IL-D876-5432-9876")
	assertDetection(t, detections, "NPI", "1928473625")
	assertDetection(t, detections, "DEA", "MF8372914")
	assertDetection(t, detections, "PHONE", "+44 20 7946 0958")
	assertDetection(t, detections, "API_KEY", "ghp_xyzABCD1234567890abcdef")
}

func TestDetectRuntimeRedactionsCapturesBusinessContractPII(t *testing.T) {
	textBytes, err := os.ReadFile("../dataset/raw/manual_synthetic_txt/business_contract.txt")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}

	detections := detectRuntimeRedactions(string(textBytes))

	assertDetection(t, detections, "ADDRESS", "1800 Technology Parkway, Suite 200 San Jose, CA 95110")
	assertDetection(t, detections, "ADDRESS", "100 Innovation Drive, Suite 400 San Francisco, CA 94105")
	assertDetection(t, detections, "BANK_ACCOUNT", "10029384756")
	assertDetection(t, detections, "ROUTING_NUMBER", "121140399")
	assertDetection(t, detections, "US_PHONE", "(408) 555-0199")
}

func TestDetectRuntimeRedactionsUSPhoneIncludesOpeningParenthesis(t *testing.T) {
	text := "Call me at (408) 555-0199 tomorrow."
	detections := detectRuntimeRedactions(text)

	assertDetection(t, detections, "US_PHONE", "(408) 555-0199")
}

func assertDetection(t *testing.T, detections []runtimeDetection, wantType, wantText string) {
	t.Helper()

	wantText = normalizeWhitespace(wantText)
	for _, detection := range detections {
		if detection.Type == wantType && normalizeWhitespace(detection.Text) == wantText {
			return
		}
	}

	items := make([]string, 0, len(detections))
	for _, detection := range detections {
		items = append(items, detection.Type+":"+normalizeWhitespace(detection.Text))
	}
	t.Fatalf("missing detection %s:%q in %v", wantType, wantText, items)
}

func normalizeWhitespace(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
