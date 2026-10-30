package document

import (
	"time"

	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
)

// Status is where a document is in its life.
//
//	QUEUED ──> PROCESSING ──> REVIEW_REQUIRED ──> COMPLETED
//	   ^            │               │
//	   │            ├──> FAILED     └──(retry, discarding the draft)──> QUEUED
//	   └─(retry)────┴──> CANCELLED
//
// A document is uploaded straight into QUEUED: the row is only created once the
// file is stored and valid, so there is no separate UPLOADED state to be stuck in.
type Status string

const (
	StatusQueued         Status = "QUEUED"
	StatusProcessing     Status = "PROCESSING"
	StatusReviewRequired Status = "REVIEW_REQUIRED"
	StatusCompleted      Status = "COMPLETED"
	StatusFailed         Status = "FAILED"
	StatusCancelled      Status = "CANCELLED"
)

// Live reports whether the document still counts towards an exam's limit and
// blocks uploading the same file again.
func (s Status) Live() bool { return s != StatusFailed && s != StatusCancelled }

// Stage says what a PROCESSING document is doing.
type Stage string

const (
	StageReadingPages Stage = "READING_PAGES"
	StageExtracting   Stage = "EXTRACTING"
	StageValidating   Stage = "VALIDATING"
	StageSaving       Stage = "SAVING"
)

// Origin says who wrote a draft question.
const (
	OriginExtracted = "EXTRACTED"
	OriginTeacher   = "TEACHER"
)

// PageInfo is what the pipeline found about one page.
type PageInfo struct {
	Number  int    `json:"number"`
	Kind    string `json:"kind"`
	Chars   int    `json:"chars"`
	Columns int    `json:"columns"`
	// Images is how many pictures were sent to a vision model; ImagesSkipped is
	// how many could not be (unsupported format or too large).
	Images        int      `json:"images"`
	ImagesSkipped int      `json:"imagesSkipped"`
	RemovedLines  []string `json:"removedLines,omitempty"`
	Note          string   `json:"note,omitempty"`
}

// Document is an uploaded exam paper and the state of its processing. The PDF
// itself is in object storage under StorageKey.
type Document struct {
	ID               uuid.UUID
	ExamID           uuid.UUID
	UploadedBy       uuid.UUID
	OriginalFilename string
	StorageKey       string
	SizeBytes        int64
	SHA256           string
	PageCount        int

	Status        Status
	Stage         Stage
	ProgressDone  int
	ProgressTotal int
	Attempt       int
	LeaseExpires  *time.Time

	Provider       string
	PromptVersion  string
	FailureCode    string
	FailureMessage string

	PageSummary          []PageInfo
	DocumentInstructions string
	Unassigned           []extract.Unassigned

	DraftVersion int
	ApprovedBy   *uuid.UUID
	ApprovedAt   *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// DraftQuestion is one extracted (or teacher-added) question awaiting review.
type DraftQuestion struct {
	ID         uuid.UUID
	DocumentID uuid.UUID
	Position   int

	NumberLabel  string
	Type         string
	Instructions string
	Text         string
	Points       *int
	Options      []extract.Option
	Tables       []extract.Table
	Diagrams     []extract.Diagram
	SourcePages  []int
	Confidence   string
	Warnings     []extract.Warning

	Origin    string
	Edited    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// DraftData is a complete extraction result, saved in one step.
type DraftData struct {
	Provider             string
	PromptVersion        string
	PageSummary          []PageInfo
	DocumentInstructions string
	Unassigned           []extract.Unassigned
	Questions            []DraftQuestion
}

// Caller is the authenticated identity, taken from the verified token and never
// from the request.
type Caller struct {
	ID   uuid.UUID
	Role platformauth.Role
}
