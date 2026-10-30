// Package extract turns the pages of an exam paper into draft questions. It is
// provider independent: a Model (Groq today) only turns one batch of pages into
// JSON text, and everything else (batching, parsing, validating, checking the
// output against the page, joining questions that run across a batch) happens
// here, where it can be tested without any model.
//
// The model is an untrusted source. Its reply is parsed and validated like any
// other input; what it returns is never published, only offered to a teacher.
// It is not asked to mark, grade or choose correct answers, and the output has
// nowhere to put one.
package extract

import (
	"context"
	"errors"
	"fmt"
)

// Page kinds, as the PDF layer reports them.
const (
	KindText      = "TEXT"
	KindImageOnly = "IMAGE_ONLY"
	KindMixed     = "MIXED"
)

// Question types extraction may propose. VOICE questions are a classroom choice
// and are never proposed.
const (
	TypeMCQ   = "MCQ"
	TypeEssay = "ESSAY"
)

// Confidence levels.
const (
	ConfidenceHigh   = "HIGH"
	ConfidenceMedium = "MEDIUM"
	ConfidenceLow    = "LOW"
)

// Warning codes. A warning never removes content; it tells the teacher where to
// look.
const (
	WarnUnverifiedText     = "UNVERIFIED_TEXT"      // wording not found on the page
	WarnPageMismatch       = "PAGE_MISMATCH"        // found, but on another page than claimed
	WarnReadFromImage      = "READ_FROM_IMAGE"      // read by a vision model; check against the page
	WarnImageNotRead       = "IMAGE_NOT_READ"       // a picture page nothing could read
	WarnTypeUncertain      = "TYPE_UNCERTAIN"       // question type chosen from the shape of the question
	WarnFewOptions         = "FEW_OPTIONS"          // a multiple-choice question with fewer than 2 options
	WarnDuplicateOption    = "DUPLICATE_OPTION"     // two options had the same label
	WarnDuplicateNumber    = "DUPLICATE_NUMBER"     // a question number was used twice
	WarnNumberingGap       = "NUMBERING_GAP"        // numbers skip: a question may be missing
	WarnPageUnknown        = "PAGE_UNKNOWN"         // source page missing or invalid
	WarnContinuationOrphan = "CONTINUATION_ORPHAN"  // said to continue a question that does not exist
	WarnDiagramNeedsText   = "DIAGRAM_NEEDS_TEXT"   // a diagram with no description
	WarnModelUncertain     = "MODEL_UNCERTAIN"      // the model said it was unsure
	WarnMarksNotOnPage     = "MARKS_NOT_ON_PAGE"    // marks reported that the page does not show
	WarnTextTooLong        = "TEXT_TOO_LONG"        // a field was cut to the allowed length
	WarnEmptyQuestionMoved = "EMPTY_QUESTION_MOVED" // a question with no wording, kept under unassigned
)

// Warning is a note about one question.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Blocking warnings must be acknowledged by the teacher before approval.
	Blocking     bool `json:"blocking"`
	Acknowledged bool `json:"acknowledged"`
}

// Option is one answer choice with the label printed in the source.
type Option struct {
	Label string `json:"label"`
	Text  string `json:"text"`
	// IsCorrect is only ever set by a teacher; extraction never sets it.
	IsCorrect bool `json:"isCorrect"`
}

// Table is a table kept as Markdown text.
type Table struct {
	Page     int    `json:"page"`
	Markdown string `json:"markdown"`
}

// Diagram is a figure the question refers to. The description is the model's
// reading of it and must be confirmed by a teacher before approval.
type Diagram struct {
	Page              int    `json:"page"`
	Region            string `json:"region"`
	Description       string `json:"description"`
	DescriptionSource string `json:"descriptionSource"` // MODEL or TEACHER
	Confirmed         bool   `json:"confirmed"`
}

// Question is one draft question.
type Question struct {
	NumberLabel  string    `json:"numberLabel"`
	Type         string    `json:"type"`
	Instructions string    `json:"instructions"`
	Text         string    `json:"text"`
	Points       *int      `json:"points"`
	Options      []Option  `json:"options"`
	Tables       []Table   `json:"tables"`
	Diagrams     []Diagram `json:"diagrams"`
	SourcePages  []int     `json:"sourcePages"`
	Confidence   string    `json:"confidence"`
	Warnings     []Warning `json:"warnings"`
}

// Unassigned is page content the model could not attach to a question. It is
// shown to the teacher, never dropped.
type Unassigned struct {
	Page   int    `json:"page"`
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

// Result is a validated extraction.
type Result struct {
	DocumentInstructions string       `json:"documentInstructions"`
	Questions            []Question   `json:"questions"`
	Unassigned           []Unassigned `json:"unassigned"`
}

// Image is a picture of (part of) a page, for a vision model.
type Image struct {
	Format string // "jpg" or "png"
	Data   []byte
}

// Page is one page of the paper as the extractor receives it.
type Page struct {
	Number int
	Kind   string
	// Text is the page's text layer in reading order, with running headers and
	// footers removed. Empty for a picture-only page.
	Text   string
	Images []Image
}

// Input is a whole paper.
type Input struct {
	Pages []Page
}

// Request is one batch for a model: either pages with text, or one picture page.
type Request struct {
	Pages []Page
	// Vision is true when the batch is a picture page and Pages[0].Images carry
	// the content to read.
	Vision bool
}

// Model turns one batch into the JSON text of a raw extraction. It does no
// parsing or validation of what the model said.
type Model interface {
	// Name identifies the provider and model in logs and in the document record.
	Name() string
	// PromptVersion labels the prompt in use.
	PromptVersion() string
	// CanReadImages is false when no vision model is configured.
	CanReadImages() bool
	Complete(ctx context.Context, req Request) (string, error)
}

// Error classes. A transient failure may succeed if tried again later; a
// permanent one will not.
var (
	ErrTransient = errors.New("extraction: transient failure")
	ErrPermanent = errors.New("extraction: permanent failure")
)

// Failure codes stored on a failed document. They are fixed strings: what the
// provider said is never kept.
const (
	CodeRateLimited    = "PROVIDER_RATE_LIMITED"
	CodeUnavailable    = "PROVIDER_UNAVAILABLE"
	CodeTimeout        = "PROVIDER_TIMEOUT"
	CodeAuth           = "PROVIDER_AUTH"
	CodeRejected       = "PROVIDER_REJECTED"
	CodeModelNotFound  = "MODEL_NOT_FOUND"
	CodeTruncated      = "OUTPUT_TRUNCATED"
	CodeInvalidOutput  = "INVALID_PROVIDER_OUTPUT"
	CodeNothingFound   = "NO_QUESTIONS_FOUND"
	CodeInvalidPDF     = "INVALID_PDF"
	CodeQueueDown      = "QUEUE_UNAVAILABLE"
	CodeStorageDown    = "STORAGE_UNAVAILABLE"
	CodeInternal       = "INTERNAL_ERROR"
	CodeTooManyBatches = "DOCUMENT_TOO_LARGE"
)

// Error is a classified failure.
type Error struct {
	Code      string
	Transient bool
	// Detail is safe text: it never contains provider output or page content.
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return e.Code
	}
	return e.Code + ": " + e.Detail
}

// Is makes errors.Is(err, ErrTransient) / ErrPermanent work.
func (e *Error) Is(target error) bool {
	if e.Transient {
		return target == ErrTransient
	}
	return target == ErrPermanent
}

// NewError builds a classified failure.
func NewError(code string, transient bool, format string, args ...any) *Error {
	return &Error{Code: code, Transient: transient, Detail: fmt.Sprintf(format, args...)}
}

// CodeOf returns the failure code of err, or CodeInternal for anything else,
// and whether the failure is worth retrying.
func CodeOf(err error) (code string, transient bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Code, e.Transient
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return CodeTimeout, true
	}
	return CodeInternal, false
}
