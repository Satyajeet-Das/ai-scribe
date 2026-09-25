package assignment

import (
	"time"

	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Status string

const (
	StatusAssigned Status = "ASSIGNED"
	StatusRevoked  Status = "REVOKED"
)

type Assignment struct {
	model.Base
	ExamID     uuid.UUID `json:"examId" db:"exam_id"`
	StudentID  uuid.UUID `json:"studentId" db:"student_id"`
	AssignedAt time.Time `json:"assignedAt" db:"assigned_at"`
	Status     Status    `json:"status" db:"status"`
}
