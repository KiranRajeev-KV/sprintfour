package main

import "testing"

func TestShouldAutoAcceptGLiNERDetectionUsesLoweredThresholds(t *testing.T) {
	testCases := []struct {
		label string
		score float64
		want  bool
	}{
		{label: "EMAIL", score: 0.75, want: true},
		{label: "EMAIL", score: 0.74, want: false},
		{label: "PHONE", score: 0.72, want: true},
		{label: "PHONE", score: 0.71, want: false},
		{label: "ADDRESS", score: 0.68, want: true},
		{label: "ADDRESS", score: 0.67, want: false},
		{label: "PERSON", score: 0.72, want: true},
		{label: "PERSON", score: 0.71, want: false},
		{label: "ORGANIZATION_CONTACT", score: 0.78, want: true},
		{label: "ORGANIZATION_CONTACT", score: 0.77, want: false},
	}

	for _, testCase := range testCases {
		got := shouldAutoAcceptGLiNERDetection(testCase.label, testCase.score)
		if got != testCase.want {
			t.Fatalf("shouldAutoAcceptGLiNERDetection(%q, %.2f) = %v, want %v", testCase.label, testCase.score, got, testCase.want)
		}
	}
}
