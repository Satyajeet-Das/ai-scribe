package answer

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var validate = validator.New()

type SubmitAnswerRequest struct {
	SessionID    uuid.UUID `json:"sessionId" validate:"required"`
	QuestionID   uuid.UUID `json:"questionId" validate:"required"`
	ResponseText string    `json:"responseText" validate:"required"`
	AudioURL     string    `json:"audioUrl,omitempty"`
}

func (r *SubmitAnswerRequest) Validate() error {
	return validate.Struct(r)
}

type AnswerResponse struct {
	ID           uuid.UUID `json:"id"`
	SessionID    uuid.UUID `json:"sessionId"`
	QuestionID   uuid.UUID `json:"questionId"`
	CandidateID  uuid.UUID `json:"candidateId"`
	ResponseText string    `json:"responseText"`
	AudioURL     string    `json:"audioUrl,omitempty"`
	Score        *float64  `json:"score,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

func ToAnswerResponse(a *Answer) AnswerResponse {
	return AnswerResponse{
		ID:           a.ID,
		SessionID:    a.SessionID,
		QuestionID:   a.QuestionID,
		CandidateID:  a.CandidateID,
		ResponseText: a.ResponseText,
		AudioURL:     a.AudioURL,
		Score:        a.Score,
		CreatedAt:    a.CreatedAt,
	}
}
