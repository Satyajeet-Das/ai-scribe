package session

import (
	"time"

	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Status string

const (
	StatusInitialized Status = "initialized"
	StatusInProgress  Status = "in_progress"
	StatusPaused      Status = "paused"
	StatusCompleted   Status = "completed"
	StatusTerminated  Status = "terminated"
)

type Session struct {
	model.Base
	AssignmentID uuid.UUID  `json:"assignmentId" db:"assignment_id"`
	CandidateID  uuid.UUID  `json:"candidateId" db:"candidate_id"`
	ExamID       uuid.UUID  `json:"examId" db:"exam_id"`
	Status       Status     `json:"status" db:"status"`
	StartedAt    *time.Time `json:"startedAt,omitempty" db:"started_at"`
	EndedAt      *time.Time `json:"endedAt,omitempty" db:"ended_at"`
	CurrentIndex int        `json:"currentIndex" db:"current_index"`
}
