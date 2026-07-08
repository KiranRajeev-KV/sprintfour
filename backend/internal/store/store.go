package store

import (
	doc "backend/internal/document"
	"errors"
	"sync"
)

const lowConfidenceThreshold = doc.LowConfidenceThreshold

type (
	Document                      = doc.Document
	DocumentRuntimeState          = doc.DocumentRuntimeState
	DocumentSnapshot              = doc.DocumentSnapshot
	Redaction                     = doc.Redaction
	RedactionRuntimeState         = doc.RedactionRuntimeState
	RedactionSnapshot             = doc.RedactionSnapshot
	BatchSummary                  = doc.BatchSummary
	DocumentMutationResult        = doc.DocumentMutationResult
	RedactionMutationResult       = doc.RedactionMutationResult
	BulkRedactionMutationResponse = doc.BulkRedactionMutationResponse
	BulkMutationResponse          = doc.BulkMutationResponse
	ExportedDocument              = doc.ExportedDocument
	ExportSummary                 = doc.ExportSummary
	ReviewSummary                 = doc.ReviewSummary
	UploadedDocumentInput         = doc.UploadedDocumentInput
	UploadItemResult              = doc.UploadItemResult
	UploadBatchResult             = doc.UploadBatchResult
	RuntimeDetection              = doc.RuntimeDetection
)

var (
	ErrDocumentNotFound  = errors.New("document not found")
	ErrRedactionNotFound = errors.New("redaction not found")
)

type StateConflictError struct {
	Code    string
	Message string
}

func (e *StateConflictError) Error() string {
	return e.Message
}

type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

type ManualRedactionInput struct {
	Start        int
	End          int
	Type         string
	Reason       string
	SelectedText string
}

type redactionCounts struct {
	total                    int
	pending                  int
	accepted                 int
	rejected                 int
	added                    int
	lowConfidence            int
	regexCandidates          int
	controlledFalsePositives int
	controlledMissedPII      int
	blockingReviewItems      int
}

type exportStats struct {
	applied         int
	skippedRejected int
	skippedPending  int
	skippedOverlap  int
}

type Store struct {
	mu                    sync.RWMutex
	submitter             JobSubmitter
	documents             []*Document
	documentsByID         map[string]*Document
	redactionsByDoc       map[string][]*Redaction
	redactionsByID        map[string]*Redaction
	documentRuneLengths   map[string]int
	runtimeByDocID        map[string]DocumentRuntimeState
	redactionRuntimeByID  map[string]RedactionRuntimeState
	latestExport          *ExportSummary
	exportSequence        int
	manualRedactionSeq    int
	uploadDocumentSeq     int
	generatedRedactionSeq int
	uploadBatchSeq        int
}

type JobSubmitter interface {
	Submit(documentID, text string)
}

func NewStore(documents []*Document, redactions []*Redaction) *Store {
	store := &Store{}
	store.rebuildLocked(documents, redactions)
	return store
}

func (s *Store) SetJobSubmitter(submitter JobSubmitter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.submitter = submitter
}

type queuedDocument struct {
	documentID string
	text       string
}

type exportPlan struct {
	documents               []exportPlanDocument
	approvedBlockedByReview int
}

type exportPlanDocument struct {
	DocumentID string
	Title      string
	SourceFile string
	Text       string
	redactions []RedactionSnapshot
	stats      exportStats
}
