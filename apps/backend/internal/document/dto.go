package document

import (
	"time"

	"github.com/google/uuid"
)

// ---- responses ----

// ProgressResponse says how far a PROCESSING document has got.
type ProgressResponse struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// PageInfoResponse is what was found about one page of a document.
type PageInfoResponse struct {
	Number        int      `json:"number"`
	Kind          string   `json:"kind" enums:"TEXT,IMAGE_ONLY,MIXED"`
	Chars         int      `json:"chars"`
	Columns       int      `json:"columns"`
	Images        int      `json:"images"`
	ImagesSkipped int      `json:"imagesSkipped"`
	RemovedLines  []string `json:"removedLines,omitempty"`
	Note          string   `json:"note,omitempty"`
}

// DocumentResponse is an uploaded exam document and where its processing stands.
type DocumentResponse struct {
	ID             uuid.UUID          `json:"id"`
	ExamID         uuid.UUID          `json:"examId"`
	Filename       string             `json:"filename"`
	SizeBytes      int64              `json:"sizeBytes"`
	PageCount      int                `json:"pageCount"`
	Status         Status             `json:"status" enums:"QUEUED,PROCESSING,REVIEW_REQUIRED,COMPLETED,FAILED,CANCELLED"`
	Stage          Stage              `json:"stage,omitempty" enums:"READING_PAGES,EXTRACTING,VALIDATING,SAVING"`
	Progress       ProgressResponse   `json:"progress"`
	Attempt        int                `json:"attempt"`
	FailureCode    string             `json:"failureCode,omitempty"`
	FailureMessage string             `json:"failureMessage,omitempty"`
	Provider       string             `json:"provider,omitempty"`
	DraftVersion   int                `json:"draftVersion"`
	UploadedBy     uuid.UUID          `json:"uploadedBy"`
	ApprovedBy     *uuid.UUID         `json:"approvedBy,omitempty"`
	ApprovedAt     *time.Time         `json:"approvedAt,omitempty"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
	Pages          []PageInfoResponse `json:"pages,omitempty"`
}

// DocumentListResponse lists an exam's documents, newest first.
type DocumentListResponse struct {
	Documents []DocumentResponse `json:"documents"`
	Total     int                `json:"total"`
}

// OptionResponse is one answer choice of a draft question, with the label the
// source printed. IsCorrect is only ever set by a teacher.
type OptionResponse struct {
	Label     string `json:"label"`
	Text      string `json:"text"`
	IsCorrect bool   `json:"isCorrect"`
}

// TableResponse is a table kept as Markdown, with its source page.
type TableResponse struct {
	Page     int    `json:"page"`
	Markdown string `json:"markdown"`
}

// DiagramResponse is a figure a question refers to. The description is a
// reading of it that a teacher must confirm before the draft can be approved.
type DiagramResponse struct {
	Page              int    `json:"page"`
	Region            string `json:"region"`
	Description       string `json:"description"`
	DescriptionSource string `json:"descriptionSource,omitempty" enums:"MODEL,TEACHER"`
	Confirmed         bool   `json:"confirmed"`
}

// WarningResponse is a note about a draft question. A blocking warning must be
// acknowledged by the teacher before the draft can be approved.
type WarningResponse struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	Blocking     bool   `json:"blocking"`
	Acknowledged bool   `json:"acknowledged"`
}

// DraftQuestionResponse is one question awaiting review.
type DraftQuestionResponse struct {
	ID           uuid.UUID         `json:"id"`
	Position     int               `json:"position"`
	NumberLabel  string            `json:"numberLabel"`
	Type         string            `json:"type" enums:"MCQ,ESSAY"`
	Instructions string            `json:"instructions"`
	Text         string            `json:"text"`
	Points       *int              `json:"points"`
	Options      []OptionResponse  `json:"options"`
	Tables       []TableResponse   `json:"tables"`
	Diagrams     []DiagramResponse `json:"diagrams"`
	SourcePages  []int             `json:"sourcePages"`
	Confidence   string            `json:"confidence" enums:"HIGH,MEDIUM,LOW"`
	Warnings     []WarningResponse `json:"warnings"`
	Origin       string            `json:"origin" enums:"EXTRACTED,TEACHER"`
	Edited       bool              `json:"edited"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

// UnassignedResponse is page content that could not be attached to a question.
type UnassignedResponse struct {
	Page   int    `json:"page"`
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

// DraftResponse is a document's extracted draft.
type DraftResponse struct {
	Document             DocumentResponse        `json:"document"`
	DraftVersion         int                     `json:"draftVersion"`
	DocumentInstructions string                  `json:"documentInstructions"`
	Unassigned           []UnassignedResponse    `json:"unassigned"`
	Questions            []DraftQuestionResponse `json:"questions"`
}

// DraftQuestionResultResponse is one question after an add or an edit.
type DraftQuestionResultResponse struct {
	Question     DraftQuestionResponse `json:"question"`
	DraftVersion int                   `json:"draftVersion"`
}

// ApprovedQuestionResponse is a question created in the exam by an approval.
type ApprovedQuestionResponse struct {
	ID             uuid.UUID `json:"id"`
	QuestionNumber int       `json:"questionNumber"`
	Type           string    `json:"type"`
	Points         int       `json:"points"`
	Text           string    `json:"text"`
}

// ApproveResponse is the document after approval and the questions it created.
type ApproveResponse struct {
	Document  DocumentResponse           `json:"document"`
	Questions []ApprovedQuestionResponse `json:"questions"`
}

// ---- requests ----

// OptionRequest is an answer choice written by a teacher.
type OptionRequest struct {
	Label     string `json:"label"`
	Text      string `json:"text"`
	IsCorrect bool   `json:"isCorrect"`
}

// TableRequest is a table written by a teacher.
type TableRequest struct {
	Page     int    `json:"page"`
	Markdown string `json:"markdown"`
}

// DiagramRequest is a diagram description written by a teacher.
type DiagramRequest struct {
	Page        int    `json:"page"`
	Region      string `json:"region"`
	Description string `json:"description"`
	// Confirmed says the teacher has checked the description against the figure.
	Confirmed bool `json:"confirmed"`
}

// DraftQuestionRequest is the complete state a teacher wants for one draft
// question: a replacement, not a patch.
type DraftQuestionRequest struct {
	NumberLabel  string           `json:"numberLabel"`
	Type         string           `json:"type" enums:"MCQ,ESSAY"`
	Instructions string           `json:"instructions"`
	Text         string           `json:"text"`
	Points       *int             `json:"points"`
	Options      []OptionRequest  `json:"options"`
	Tables       []TableRequest   `json:"tables"`
	Diagrams     []DiagramRequest `json:"diagrams"`
	// AcknowledgedWarnings are the codes of blocking warnings the teacher has
	// checked. Only warnings the system raised can be acknowledged.
	AcknowledgedWarnings []string `json:"acknowledgedWarnings"`
}

// AddDraftQuestionRequest adds a question the extraction missed.
type AddDraftQuestionRequest struct {
	ExpectedDraftVersion *int `json:"expectedDraftVersion"`
	// Position puts the question there (1 is first); omitted or past the end puts
	// it last.
	Position *int                 `json:"position"`
	Question DraftQuestionRequest `json:"question"`
}

// ReplaceDraftQuestionRequest replaces a draft question.
type ReplaceDraftQuestionRequest struct {
	ExpectedDraftVersion *int                 `json:"expectedDraftVersion"`
	Question             DraftQuestionRequest `json:"question"`
}

// ReorderDraftRequest gives every draft question in its new order.
type ReorderDraftRequest struct {
	ExpectedDraftVersion *int        `json:"expectedDraftVersion"`
	QuestionIDs          []uuid.UUID `json:"questionIds"`
}

// RetryDocumentRequest runs a document again.
type RetryDocumentRequest struct {
	// DiscardDraft allows re-running a document that has a draft waiting for
	// review, throwing the draft away. ExpectedDraftVersion must then match.
	DiscardDraft         bool `json:"discardDraft"`
	ExpectedDraftVersion int  `json:"expectedDraftVersion"`
}

// ApproveDraftRequest approves the draft as the teacher last saw it.
type ApproveDraftRequest struct {
	ExpectedDraftVersion *int `json:"expectedDraftVersion"`
}

// ---- mapping ----

func toDocumentResponse(d *Document, withPages bool) DocumentResponse {
	out := DocumentResponse{
		ID: d.ID, ExamID: d.ExamID, Filename: d.OriginalFilename, SizeBytes: d.SizeBytes, PageCount: d.PageCount,
		Status: d.Status, Stage: d.Stage, Progress: ProgressResponse{Done: d.ProgressDone, Total: d.ProgressTotal},
		Attempt: d.Attempt, FailureCode: d.FailureCode, FailureMessage: d.FailureMessage, Provider: d.Provider,
		DraftVersion: d.DraftVersion, UploadedBy: d.UploadedBy, ApprovedBy: d.ApprovedBy, ApprovedAt: d.ApprovedAt,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
	if withPages {
		out.Pages = make([]PageInfoResponse, 0, len(d.PageSummary))
		for _, p := range d.PageSummary {
			out.Pages = append(out.Pages, PageInfoResponse(p))
		}
	}
	return out
}

func toDraftQuestionResponse(q DraftQuestion) DraftQuestionResponse {
	out := DraftQuestionResponse{
		ID: q.ID, Position: q.Position, NumberLabel: q.NumberLabel, Type: q.Type, Instructions: q.Instructions, Text: q.Text,
		Points: q.Points, Options: make([]OptionResponse, 0, len(q.Options)), Tables: make([]TableResponse, 0, len(q.Tables)),
		Diagrams: make([]DiagramResponse, 0, len(q.Diagrams)), SourcePages: append([]int{}, q.SourcePages...),
		Confidence: q.Confidence, Warnings: make([]WarningResponse, 0, len(q.Warnings)), Origin: q.Origin, Edited: q.Edited,
		CreatedAt: q.CreatedAt, UpdatedAt: q.UpdatedAt,
	}
	for _, o := range q.Options {
		out.Options = append(out.Options, OptionResponse(o))
	}
	for _, t := range q.Tables {
		out.Tables = append(out.Tables, TableResponse(t))
	}
	for _, d := range q.Diagrams {
		out.Diagrams = append(out.Diagrams, DiagramResponse(d))
	}
	for _, w := range q.Warnings {
		out.Warnings = append(out.Warnings, WarningResponse(w))
	}
	return out
}

func toDraftResponse(d *Draft) DraftResponse {
	out := DraftResponse{
		Document: toDocumentResponse(d.Document, true), DraftVersion: d.Document.DraftVersion,
		DocumentInstructions: d.Document.DocumentInstructions,
		Unassigned:           make([]UnassignedResponse, 0, len(d.Document.Unassigned)),
		Questions:            make([]DraftQuestionResponse, 0, len(d.Questions)),
	}
	for _, u := range d.Document.Unassigned {
		out.Unassigned = append(out.Unassigned, UnassignedResponse(u))
	}
	for _, q := range d.Questions {
		out.Questions = append(out.Questions, toDraftQuestionResponse(q))
	}
	return out
}

func (r DraftQuestionRequest) toInput() QuestionInput {
	in := QuestionInput{
		NumberLabel: r.NumberLabel, Type: r.Type, Instructions: r.Instructions, Text: r.Text, Points: r.Points,
		AcknowledgedWarnings: r.AcknowledgedWarnings,
	}
	for _, o := range r.Options {
		in.Options = append(in.Options, OptionInput(o))
	}
	for _, t := range r.Tables {
		in.Tables = append(in.Tables, TableInput(t))
	}
	for _, d := range r.Diagrams {
		in.Diagrams = append(in.Diagrams, DiagramInput(d))
	}
	return in
}
