package session

import (
	"errors"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var validate = validator.New()

type StartSessionRequest struct {
	AssignmentID *uuid.UUID `json:"assignmentId,omitempty"`
	ExamID       *uuid.UUID `json:"examId,omitempty"`
}

func (r *StartSessionRequest) Validate() error {
	if err := validate.Struct(r); err != nil {
		return err
	}
	hasAssignment := r.AssignmentID != nil && *r.AssignmentID != uuid.Nil
	hasExam := r.ExamID != nil && *r.ExamID != uuid.Nil
	if !hasAssignment && !hasExam {
		return errors.New("either assignmentId or examId is required")
	}
	return nil
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

type PaginatedSessionsResponse struct {
	Sessions []SessionResponse `json:"sessions"`
	Total    int               `json:"total"`
	Limit    int               `json:"limit"`
	Offset   int               `json:"offset"`
}
