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
	ExamID        uuid.UUID `json:"examId" db:"exam_id"`
	StudentID     uuid.UUID `json:"studentId" db:"student_id"`
	StudentName   string    `json:"studentName,omitempty" db:"student_name"`
	StudentRollNo string    `json:"studentRollNo,omitempty" db:"student_roll_no"`
	AssignedAt    time.Time `json:"assignedAt" db:"assigned_at"`
	Status        Status    `json:"status" db:"status"`
}

type StudentAssignedExam struct {
	AssignmentID uuid.UUID `json:"assignmentId" db:"assignment_id"`
	ExamID       uuid.UUID `json:"examId" db:"exam_id"`
	Title        string    `json:"title" db:"title"`
	Subject      string    `json:"subject" db:"subject"`
	Description  string    `json:"description" db:"description"`
	DurationMins int       `json:"durationMins" db:"duration_mins"`
	ExamStatus   string    `json:"examStatus" db:"exam_status"`
	AssignedAt   time.Time `json:"assignedAt" db:"assigned_at"`
	Status       Status    `json:"status" db:"status"`
}
