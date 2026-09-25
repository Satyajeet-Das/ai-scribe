package session

import (
	"time"

	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Status string

const (
	StatusInProgress Status = "IN_PROGRESS"
	StatusSubmitted  Status = "SUBMITTED"
	StatusExpired    Status = "EXPIRED"
)

type Session struct {
	model.Base
	AssignmentID uuid.UUID  `json:"assignmentId" db:"assignment_id"`
	ExamID       uuid.UUID  `json:"examId" db:"exam_id"`
	StudentID    uuid.UUID  `json:"studentId" db:"student_id"`
	Status       Status     `json:"status" db:"status"`
	StartedAt    time.Time  `json:"startedAt" db:"started_at"`
	SubmittedAt  *time.Time `json:"submittedAt,omitempty" db:"submitted_at"`
}
