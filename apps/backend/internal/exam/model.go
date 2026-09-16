package exam

import (
	"time"

	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusScheduled Status = "scheduled"
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
	StatusArchived  Status = "archived"
)

type Exam struct {
	model.Base
	Title           string     `json:"title" db:"title"`
	Description     string     `json:"description" db:"description"`
	Subject         string     `json:"subject" db:"subject"`
	DurationMinutes int        `json:"durationMinutes" db:"duration_minutes"`
	Status          Status     `json:"status" db:"status"`
	ScheduledStart  *time.Time `json:"scheduledStart,omitempty" db:"scheduled_start"`
	ScheduledEnd    *time.Time `json:"scheduledEnd,omitempty" db:"scheduled_end"`
	CreatedBy       uuid.UUID  `json:"createdBy" db:"created_by"`
}
