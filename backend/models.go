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

type DocumentRuntimeState struct {
	Status      string
	RetryCount  int
	FailureHint *string
}

type DocumentSnapshot struct {
	ID                     string
	Title                  string
	Source                 string
	SourceFile             string
	Text                   string
	CharCount              int
	SyntheticPIIInjected   bool
	Status                 string
	RiskLevel              string
	FailureHint            *string
	PIICount               int
	LowConfidenceCount     int
	RedactionCount         int
	RetryCount             int
	PendingRedactionCount  int
	AcceptedRedactionCount int
	RejectedRedactionCount int
	AddedRedactionCount    int
	BlockingReviewCount    int
	CanApprove             bool
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

type RedactionRuntimeState struct {
	ReviewState string
	ReviewedAt  *string
	IsUserAdded bool
	CreatedAt   *string
}

type RedactionSnapshot struct {
	ID              string  `json:"id"`
	DocumentID      string  `json:"document_id"`
	Start           int     `json:"start"`
	End             int     `json:"end"`
	Text            string  `json:"text"`
	Type            string  `json:"type"`
	Confidence      float64 `json:"confidence"`
	Reason          string  `json:"reason"`
	Source          string  `json:"source"`
	SuggestedStatus string  `json:"suggested_status"`
	IsGroundTruth   bool    `json:"is_ground_truth"`
	ReviewState     string  `json:"review_state"`
	ReviewedAt      *string `json:"reviewed_at"`
	ReviewedBy      *string `json:"reviewed_by"`
	IsUserAdded     bool    `json:"is_user_added"`
	CreatedAt       *string `json:"created_at"`
}

type BatchSummary struct {
	TotalDocuments           int `json:"total_documents"`
	Queued                   int `json:"queued"`
	Processing               int `json:"processing"`
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
	PendingRedactions        int `json:"pending_redactions"`
	AcceptedRedactions       int `json:"accepted_redactions"`
	RejectedRedactions       int `json:"rejected_redactions"`
	AddedRedactions          int `json:"added_redactions"`
	BlockingReviewDocuments  int `json:"blocking_review_documents"`
}

type APIErrorEnvelope struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type DocumentMutationResult struct {
	DocumentID     string `json:"document_id"`
	PreviousStatus string `json:"previous_status"`
	Status         string `json:"status"`
	Changed        bool   `json:"changed"`
	RetryCount     int    `json:"retry_count,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

type RedactionMutationResult struct {
	RedactionID   string `json:"redaction_id"`
	DocumentID    string `json:"document_id"`
	PreviousState string `json:"previous_state"`
	ReviewState   string `json:"review_state"`
	Changed       bool   `json:"changed"`
}

type BulkMutationResponse struct {
	Requested int                      `json:"requested"`
	Approved  int                      `json:"approved,omitempty"`
	Retried   int                      `json:"retried,omitempty"`
	Skipped   int                      `json:"skipped"`
	Items     []DocumentMutationResult `json:"items"`
}

type ExportedDocument struct {
	DocumentID            string
	Title                 string
	SourceFile            string
	OutputFilename        string
	RedactedText          string
	AppliedRedactionCount int
}

type ExportSummary struct {
	ExportID                string             `json:"export_id"`
	ExportedDocuments       int                `json:"exported_documents"`
	SkippedDocuments        int                `json:"skipped_documents"`
	NeedsReview             int                `json:"needs_review"`
	Failed                  int                `json:"failed"`
	Ready                   int                `json:"ready"`
	ApprovedBlockedByReview int                `json:"approved_blocked_by_review"`
	AppliedRedactions       int                `json:"applied_redactions"`
	SkippedRejected         int                `json:"skipped_rejected_redactions"`
	SkippedPending          int                `json:"skipped_pending_redactions"`
	SkippedOverlap          int                `json:"skipped_overlap_redactions"`
	OutputDir               string             `json:"output_dir"`
	Files                   []string           `json:"files"`
	CreatedAt               string             `json:"created_at"`
	Documents               []ExportedDocument `json:"-"`
}

type ReviewSummary struct {
	DocumentID               string `json:"document_id"`
	Status                   string `json:"status"`
	RiskLevel                string `json:"risk_level"`
	TotalRedactions          int    `json:"total_redactions"`
	Pending                  int    `json:"pending"`
	Accepted                 int    `json:"accepted"`
	Rejected                 int    `json:"rejected"`
	Added                    int    `json:"added"`
	LowConfidence            int    `json:"low_confidence"`
	RegexCandidates          int    `json:"regex_candidates"`
	ControlledFalsePositives int    `json:"controlled_false_positives"`
	ControlledMissedPII      int    `json:"controlled_missed_pii"`
	BlockingReviewItems      int    `json:"blocking_review_items"`
	CanApprove               bool   `json:"can_approve"`
}

type UploadedDocumentInput struct {
	Filename     string
	RelativePath string
	Text         string
}

type UploadItemResult struct {
	Filename       string  `json:"filename"`
	RelativePath   *string `json:"relative_path"`
	DocumentID     *string `json:"document_id"`
	Status         *string `json:"status"`
	RiskLevel      *string `json:"risk_level"`
	RedactionCount int     `json:"redaction_count"`
	Accepted       bool    `json:"accepted"`
	Reason         string  `json:"reason"`
}

type UploadBatchResult struct {
	BatchID           string             `json:"batch_id"`
	Mode              string             `json:"mode"`
	Uploaded          int                `json:"uploaded"`
	Accepted          int                `json:"accepted"`
	Rejected          int                `json:"rejected"`
	DocumentsCreated  int                `json:"documents_created"`
	RedactionsCreated int                `json:"redactions_created"`
	Items             []UploadItemResult `json:"items"`
}
