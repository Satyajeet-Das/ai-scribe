package session

import (
	"time"

	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Status string

const (
	StatusPending    Status = "PENDING"
	StatusNotStarted Status = "NOT_STARTED"
	StatusInProgress Status = "IN_PROGRESS"
	StatusSubmitted  Status = "SUBMITTED"
	StatusExpired    Status = "EXPIRED"
)

type Session struct {
	model.Base
	AssignmentID      uuid.UUID  `json:"assignmentId" db:"assignment_id"`
	ExamID            uuid.UUID  `json:"examId" db:"exam_id"`
	StudentID         uuid.UUID  `json:"studentId" db:"student_id"`
	Status            Status     `json:"status" db:"status"`
	CurrentQuestionID uuid.UUID  `json:"currentQuestionId,omitempty" db:"current_question_id"`
	StartedAt         time.Time  `json:"startedAt" db:"started_at"`
	SubmittedAt       *time.Time `json:"submittedAt,omitempty" db:"submitted_at"`

	// Runtime tracking field for Redis / ephemeral state
	LastActivityAt time.Time `json:"lastActivityAt,omitempty" db:"-"`
}
