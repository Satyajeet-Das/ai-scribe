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

func (s Status) String() string {
	return string(s)
}

func (s Status) IsValid() bool {
	switch s {
	case StatusDraft, StatusPublished, StatusArchived:
		return true
	default:
		return false
	}
}

type Exam struct {
	model.Base
	Title        string     `json:"title" db:"title"`
	Subject      string     `json:"subject" db:"subject"`
	Description  string     `json:"description" db:"description"`
	DurationMins int        `json:"durationMins" db:"duration_mins"`
	Status       Status     `json:"status" db:"status"`
	CreatedBy    uuid.UUID  `json:"createdBy" db:"created_by"`
	PublishedAt  *time.Time `json:"publishedAt,omitempty" db:"published_at"`
	DeletedAt    *time.Time `json:"deletedAt,omitempty" db:"deleted_at"`
}

func (e *Exam) IsDraft() bool {
	return e.Status == StatusDraft
}

func (e *Exam) IsPublished() bool {
	return e.Status == StatusPublished
}

func (e *Exam) IsArchived() bool {
	return e.Status == StatusArchived
}

func (e *Exam) IsDeleted() bool {
	return e.DeletedAt != nil
}
