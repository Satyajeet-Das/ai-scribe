package answer

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var validate = validator.New()

type SubmitAnswerRequest struct {
	SelectedOptionID *uuid.UUID `json:"selectedOptionId,omitempty"`
	// TextAnswer is limited to 50,000 characters: far above any exam answer,
	// so that a request cannot store an unbounded value.
	TextAnswer string `json:"textAnswer" validate:"max=50000"`
	// ExpectedRevisionNo is the revisionNo the client last saw for this
	// answer (0 if it had none). When given, the save is refused with 409
	// ANSWER_CHANGED if the answer has changed since. Omit it to overwrite.
	ExpectedRevisionNo *int `json:"expectedRevisionNo,omitempty" validate:"omitempty,min=0"`

	// Revision is filled in by trusted server code (the voice module). It is
	// not part of the JSON contract, so a client cannot claim a source.
	Revision *RevisionInput `json:"-"`
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
	// RevisionNo changes whenever the answer gets a new revision. Send it back
	// as expectedRevisionNo so a save cannot overwrite a newer answer.
	RevisionNo int       `json:"revisionNo"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func ToAnswerResponse(a *Answer) AnswerResponse {
	return AnswerResponse{
		ID:               a.ID,
		SessionID:        a.SessionID,
		QuestionID:       a.QuestionID,
		SelectedOptionID: a.SelectedOptionID,
		TextAnswer:       a.TextAnswer,
		RevisionNo:       a.RevisionNo,
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
