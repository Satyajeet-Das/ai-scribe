package assignment

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var validate = validator.New()

type CreateAssignmentRequest struct {
	ExamID      uuid.UUID `json:"examId" validate:"required"`
	CandidateID uuid.UUID `json:"candidateId" validate:"required"`
}

func (r *CreateAssignmentRequest) Validate() error {
	return validate.Struct(r)
}

type AssignmentResponse struct {
	ID          uuid.UUID `json:"id"`
	ExamID      uuid.UUID `json:"examId"`
	CandidateID uuid.UUID `json:"candidateId"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
}

func ToAssignmentResponse(a *Assignment) AssignmentResponse {
	return AssignmentResponse{
		ID:          a.ID,
		ExamID:      a.ExamID,
		CandidateID: a.CandidateID,
		Status:      a.Status,
		CreatedAt:   a.CreatedAt,
	}
}
