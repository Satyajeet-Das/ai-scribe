package document

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var (
	// ErrNotFound is returned for a missing document or exam, and also for one the
	// caller does not own: the two look the same, so ids cannot be probed.
	ErrNotFound = errors.New("document not found")
	// ErrForbiddenRole means the caller is not a teacher or administrator.
	ErrForbiddenRole = errors.New("only teachers and administrators can manage exam documents")
	// ErrExamNotDraft means the exam is published or archived: questions can only
	// be added to a draft exam.
	ErrExamNotDraft = errors.New("the exam is not a draft")
	// ErrExtractionUnavailable means no extraction provider is configured.
	ErrExtractionUnavailable = errors.New("document extraction is not available")
	// ErrFileTooLarge means the upload is over the size limit.
	ErrFileTooLarge = errors.New("file too large")
	// ErrUnsupportedType means the upload is not a PDF.
	ErrUnsupportedType = errors.New("unsupported file type")
	// ErrTooManyDocuments means the exam already has the most live documents.
	ErrTooManyDocuments = errors.New("too many documents for this exam")
	// ErrInvalidState means the action does not apply to the document's status.
	ErrInvalidState = errors.New("the document is not in a state that allows this")
	// ErrStaleVersion means the draft changed since the caller read it.
	ErrStaleVersion = errors.New("the draft was changed elsewhere")
	// ErrAlreadyApproved means the draft was already approved into the exam.
	ErrAlreadyApproved = errors.New("the draft was already approved")
	// ErrQueueUnavailable means the background queue could not take the job.
	ErrQueueUnavailable = errors.New("processing queue unavailable")
	// ErrStorageUnavailable means the file store could not be used.
	ErrStorageUnavailable = errors.New("document storage unavailable")
	// ErrConcurrentChange means the exam's questions changed while approving.
	ErrConcurrentChange = errors.New("the exam's questions changed while the draft was being approved")
	// ErrLeaseHeld means another worker still owns the document. The job is
	// retried later, by which time the lease has been renewed or has expired.
	ErrLeaseHeld = errors.New("document is being processed by another worker")
	// ErrDraftTooLarge means the draft already has the most questions.
	ErrDraftTooLarge = errors.New("the draft has too many questions")
	// ErrBusy means the server is parsing as many documents as it allows.
	ErrBusy = errors.New("the server is busy; try again shortly")
)

// InvalidPDFError says why an upload was refused as a PDF. Reason is one of a
// fixed set of codes and never quotes the file.
type InvalidPDFError struct{ Reason string }

func (e *InvalidPDFError) Error() string { return "invalid PDF: " + e.Reason }

// Reasons an upload is not a usable PDF.
const (
	ReasonNotPDF       = "NOT_A_PDF"
	ReasonMalformed    = "MALFORMED"
	ReasonEncrypted    = "ENCRYPTED"
	ReasonTooManyPages = "TOO_MANY_PAGES"
	ReasonNoPages      = "NO_PAGES"
)

// DuplicateError means this exact file is already uploaded to the exam.
type DuplicateError struct{ ExistingID uuid.UUID }

func (e *DuplicateError) Error() string { return "this file was already uploaded to this exam" }

// ValidationError is a problem with one field of a request.
type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return e.Field + " " + e.Message }

func invalid(field, format string, args ...any) *ValidationError {
	return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// Problem is one thing that stops a draft being approved.
type Problem struct {
	QuestionID uuid.UUID `json:"questionId,omitempty"`
	Position   int       `json:"position,omitempty"`
	Field      string    `json:"field"`
	Message    string    `json:"message"`
}

// IncompleteError lists everything that stops a draft being approved, so the
// teacher can fix it in one pass.
type IncompleteError struct{ Problems []Problem }

func (e *IncompleteError) Error() string {
	return fmt.Sprintf("the draft is not ready to approve (%d problems)", len(e.Problems))
}
