package exam

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var validate = validator.New()

type CreateExamRequest struct {
	Title           string     `json:"title" validate:"required,min=3,max=200"`
	Description     string     `json:"description" validate:"max=2000"`
	Subject         string     `json:"subject" validate:"required,min=2,max=100"`
	DurationMinutes int        `json:"durationMinutes" validate:"required,min=5,max=600"`
	ScheduledStart  *time.Time `json:"scheduledStart,omitempty"`
	ScheduledEnd    *time.Time `json:"scheduledEnd,omitempty"`
}

func (r *CreateExamRequest) Validate() error {
	return validate.Struct(r)
}

type ExamResponse struct {
	ID              uuid.UUID  `json:"id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	Subject         string     `json:"subject"`
	DurationMinutes int        `json:"durationMinutes"`
	Status          Status     `json:"status"`
	ScheduledStart  *time.Time `json:"scheduledStart,omitempty"`
	ScheduledEnd    *time.Time `json:"scheduledEnd,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

func ToExamResponse(e *Exam) ExamResponse {
	return ExamResponse{
		ID:              e.ID,
		Title:           e.Title,
		Description:     e.Description,
		Subject:         e.Subject,
		DurationMinutes: e.DurationMinutes,
		Status:          e.Status,
		ScheduledStart:  e.ScheduledStart,
		ScheduledEnd:    e.ScheduledEnd,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
	}
}
