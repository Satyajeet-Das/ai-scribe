package question

import (
	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Type string

const (
	TypeMCQ   Type = "MCQ"
	TypeEssay Type = "ESSAY"
	TypeVoice Type = "VOICE"
)

type QuestionOption struct {
	model.Base
	QuestionID   uuid.UUID `json:"questionId" db:"question_id"`
	OptionKey    string    `json:"optionKey" db:"option_key"`
	OptionText   string    `json:"optionText" db:"option_text"`
	DisplayOrder int       `json:"displayOrder" db:"display_order"`
	IsCorrect    bool      `json:"isCorrect" db:"is_correct"`
}

type Question struct {
	model.Base
	ExamID         uuid.UUID `json:"examId" db:"exam_id"`
	QuestionNumber int       `json:"questionNumber" db:"question_number"`
	Text           string    `json:"text" db:"text"`
	Type           Type      `json:"type" db:"type"`
	Points         int       `json:"points" db:"points"`
	// SourceDocumentID and SourceDraftQuestionID say which uploaded document and
	// draft question an approved question came from; nil for questions typed in.
	SourceDocumentID      *uuid.UUID       `json:"sourceDocumentId,omitempty" db:"source_document_id"`
	SourceDraftQuestionID *uuid.UUID       `json:"sourceDraftQuestionId,omitempty" db:"source_draft_question_id"`
	Options               []QuestionOption `json:"options,omitempty"`
}
