package answer

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var validate = validator.New()

type SubmitAnswerRequest struct {
	SelectedOptionID *uuid.UUID `json:"selectedOptionId,omitempty"`
	TextAnswer       string     `json:"textAnswer"`
}

func (r *SubmitAnswerRequest) Validate() error {
	if r.SelectedOptionID == nil && r.TextAnswer == "" {
		return ErrInvalidAnswer
	}
	return validate.Struct(r)
}

type AnswerResponse struct {
	ID               uuid.UUID  `json:"id"`
	SessionID        uuid.UUID  `json:"sessionId"`
	QuestionID       uuid.UUID  `json:"questionId"`
	SelectedOptionID *uuid.UUID `json:"selectedOptionId,omitempty"`
	TextAnswer       string     `json:"textAnswer"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

func ToAnswerResponse(a *Answer) AnswerResponse {
	return AnswerResponse{
		ID:               a.ID,
		SessionID:        a.SessionID,
		QuestionID:       a.QuestionID,
		SelectedOptionID: a.SelectedOptionID,
		TextAnswer:       a.TextAnswer,
		CreatedAt:        a.CreatedAt,
		UpdatedAt:        a.UpdatedAt,
	}
}

func ToAnswerResponseList(answers []Answer) []AnswerResponse {
	res := make([]AnswerResponse, len(answers))
	for i := range answers {
		res[i] = ToAnswerResponse(&answers[i])
	}
	return res
}
