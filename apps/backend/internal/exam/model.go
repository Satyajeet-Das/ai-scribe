package exam

import (
	"time"

	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Status string

const (
	StatusDraft     Status = "DRAFT"
	StatusPublished Status = "PUBLISHED"
	StatusArchived  Status = "ARCHIVED"
)

type Exam struct {
	model.Base
	Title        string     `json:"title" db:"title"`
	Subject      string     `json:"subject" db:"subject"`
	Description  string     `json:"description" db:"description"`
	DurationMins int        `json:"durationMins" db:"duration_mins"`
	Status       Status     `json:"status" db:"status"`
	CreatedBy    uuid.UUID  `json:"createdBy" db:"created_by"`
	PublishedAt  *time.Time `json:"publishedAt,omitempty" db:"published_at"`
}
