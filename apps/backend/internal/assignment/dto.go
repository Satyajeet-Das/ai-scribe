package assignment

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var validate = validator.New()

type CreateAssignmentRequest struct {
	ExamID    uuid.UUID `json:"examId" validate:"required"`
	StudentID uuid.UUID `json:"studentId" validate:"required"`
}

func (r *CreateAssignmentRequest) Validate() error {
	return validate.Struct(r)
}

type AssignmentResponse struct {
	ID            uuid.UUID `json:"id"`
	ExamID        uuid.UUID `json:"examId"`
	StudentID     uuid.UUID `json:"studentId"`
	StudentName   string    `json:"studentName,omitempty"`
	StudentRollNo string    `json:"studentRollNo,omitempty"`
	AssignedAt    time.Time `json:"assignedAt"`
	Status        Status    `json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func ToAssignmentResponse(a *Assignment) AssignmentResponse {
	return AssignmentResponse{
		ID:            a.ID,
		ExamID:        a.ExamID,
		StudentID:     a.StudentID,
		StudentName:   a.StudentName,
		StudentRollNo: a.StudentRollNo,
		AssignedAt:    a.AssignedAt,
		Status:        a.Status,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
	}
}

func ToAssignmentResponseList(assignments []Assignment) []AssignmentResponse {
	res := make([]AssignmentResponse, len(assignments))
	for i := range assignments {
		res[i] = ToAssignmentResponse(&assignments[i])
	}
	return res
}
