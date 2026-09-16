package question

import (
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var validate = validator.New()

type CreateQuestionRequest struct {
	ExamID        uuid.UUID `json:"examId" validate:"required"`
	SequenceOrder int       `json:"sequenceOrder" validate:"min=1"`
	Type          Type      `json:"type" validate:"required"`
	Prompt        string    `json:"prompt" validate:"required,min=3"`
	Points        int       `json:"points" validate:"min=0"`
}

func (r *CreateQuestionRequest) Validate() error {
	return validate.Struct(r)
}

type QuestionResponse struct {
	ID            uuid.UUID `json:"id"`
	ExamID        uuid.UUID `json:"examId"`
	SequenceOrder int       `json:"sequenceOrder"`
	Type          Type      `json:"type"`
	Prompt        string    `json:"prompt"`
	Points        int       `json:"points"`
}

func ToQuestionResponse(q *Question) QuestionResponse {
	return QuestionResponse{
		ID:            q.ID,
		ExamID:        q.ExamID,
		SequenceOrder: q.SequenceOrder,
		Type:          q.Type,
		Prompt:        q.Prompt,
		Points:        q.Points,
	}
}
