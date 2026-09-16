package assignment

import (
	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Status string

const (
	StatusAssigned  Status = "assigned"
	StatusStarted   Status = "started"
	StatusSubmitted Status = "submitted"
	StatusGraded    Status = "graded"
)

type Assignment struct {
	model.Base
	ExamID      uuid.UUID `json:"examId" db:"exam_id"`
	CandidateID uuid.UUID `json:"candidateId" db:"candidate_id"`
	Status      Status    `json:"status" db:"status"`
}
