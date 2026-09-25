package session

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var validate = validator.New()

type StartSessionRequest struct {
	AssignmentID uuid.UUID `json:"assignmentId" validate:"required"`
}

func (r *StartSessionRequest) Validate() error {
	return validate.Struct(r)
}

type SessionResponse struct {
	ID               uuid.UUID  `json:"id"`
	AssignmentID     uuid.UUID  `json:"assignmentId"`
	ExamID           uuid.UUID  `json:"examId"`
	StudentID        uuid.UUID  `json:"studentId"`
	Status           Status     `json:"status"`
	StartedAt        time.Time  `json:"startedAt"`
	SubmittedAt      *time.Time `json:"submittedAt,omitempty"`
	DurationMins     int        `json:"durationMins,omitempty"`
	RemainingSeconds *int64     `json:"remainingSeconds,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

func ToSessionResponse(s *Session, durationMins int) SessionResponse {
	var remainingSecs *int64
	if s.Status == StatusInProgress && durationMins > 0 {
		totalAllowed := time.Duration(durationMins) * time.Minute
		elapsed := time.Since(s.StartedAt)
		rem := int64((totalAllowed - elapsed).Seconds())
		if rem < 0 {
			rem = 0
		}
		remainingSecs = &rem
	}

	return SessionResponse{
		ID:               s.ID,
		AssignmentID:     s.AssignmentID,
		ExamID:           s.ExamID,
		StudentID:        s.StudentID,
		Status:           s.Status,
		StartedAt:        s.StartedAt,
		SubmittedAt:      s.SubmittedAt,
		DurationMins:     durationMins,
		RemainingSeconds: remainingSecs,
		CreatedAt:        s.CreatedAt,
		UpdatedAt:        s.UpdatedAt,
	}
}

type SubmitSessionResponse struct {
	ID          uuid.UUID `json:"id"`
	Status      Status    `json:"status"`
	SubmittedAt time.Time `json:"submittedAt"`
	Message     string    `json:"message"`
}
