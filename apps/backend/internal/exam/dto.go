package exam

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var validate = validator.New()

type CreateExamRequest struct {
	Title        string `json:"title" validate:"required,min=3,max=255"`
	Subject      string `json:"subject" validate:"required,min=2,max=100"`
	Description  string `json:"description" validate:"max=2000"`
	DurationMins int    `json:"durationMins" validate:"required,min=1,max=600"`
}

func (r *CreateExamRequest) Validate() error {
	return validate.Struct(r)
}

type UpdateExamRequest struct {
	Title        *string `json:"title,omitempty" validate:"omitempty,min=3,max=255"`
	Subject      *string `json:"subject,omitempty" validate:"omitempty,min=2,max=100"`
	Description  *string `json:"description,omitempty" validate:"omitempty,max=2000"`
	DurationMins *int    `json:"durationMins,omitempty" validate:"omitempty,min=1,max=600"`
}

func (r *UpdateExamRequest) Validate() error {
	return validate.Struct(r)
}

type ExamResponse struct {
	ID           uuid.UUID  `json:"id"`
	Title        string     `json:"title"`
	Subject      string     `json:"subject"`
	Description  string     `json:"description"`
	DurationMins int        `json:"durationMins"`
	Status       Status     `json:"status"`
	CreatedBy    uuid.UUID  `json:"createdBy"`
	PublishedAt  *time.Time `json:"publishedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func ToExamResponse(e *Exam) ExamResponse {
	return ExamResponse{
		ID:           e.ID,
		Title:        e.Title,
		Subject:      e.Subject,
		Description:  e.Description,
		DurationMins: e.DurationMins,
		Status:       e.Status,
		CreatedBy:    e.CreatedBy,
		PublishedAt:  e.PublishedAt,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
	}
}

func ToExamResponseList(exams []Exam) []ExamResponse {
	resp := make([]ExamResponse, len(exams))
	for i := range exams {
		resp[i] = ToExamResponse(&exams[i])
	}
	return resp
}
