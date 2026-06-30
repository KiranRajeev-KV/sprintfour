package main

type Document struct {
	ID                   string
	Title                string
	Source               string
	SourceFile           string
	Text                 string
	CharCount            int
	SyntheticPIIInjected bool
	Status               string
	RiskLevel            string
	FailureHint          *string
	PIICount             int
	LowConfidenceCount   int
	RedactionCount       int
}

type Redaction struct {
	ID              string
	DocumentID      string
	Start           int
	End             int
	Text            string
	Type            string
	Confidence      float64
	Reason          string
	Source          string
	SuggestedStatus string
	IsGroundTruth   bool
}

type BatchSummary struct {
	TotalDocuments           int `json:"total_documents"`
	Ready                    int `json:"ready"`
	NeedsReview              int `json:"needs_review"`
	Failed                   int `json:"failed"`
	Clean                    int `json:"clean"`
	Approved                 int `json:"approved"`
	Exported                 int `json:"exported"`
	TotalRedactions          int `json:"total_redactions"`
	SyntheticRedactions      int `json:"synthetic_redactions"`
	RegexCandidates          int `json:"regex_candidates"`
	ControlledFalsePositives int `json:"controlled_false_positives"`
	ControlledMissedPII      int `json:"controlled_missed_pii"`
}

type APIErrorEnvelope struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
