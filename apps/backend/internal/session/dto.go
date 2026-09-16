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
	ID           uuid.UUID  `json:"id"`
	AssignmentID uuid.UUID  `json:"assignmentId"`
	CandidateID  uuid.UUID  `json:"candidateId"`
	ExamID       uuid.UUID  `json:"examId"`
	Status       Status     `json:"status"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	EndedAt      *time.Time `json:"endedAt,omitempty"`
	CurrentIndex int        `json:"currentIndex"`
}

func ToSessionResponse(s *Session) SessionResponse {
	return SessionResponse{
		ID:           s.ID,
		AssignmentID: s.AssignmentID,
		CandidateID:  s.CandidateID,
		ExamID:       s.ExamID,
		Status:       s.Status,
		StartedAt:    s.StartedAt,
		EndedAt:      s.EndedAt,
		CurrentIndex: s.CurrentIndex,
	}
}
