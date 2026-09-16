package question

import (
	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Type string

const (
	TypeMultipleChoice Type = "multiple_choice"
	TypeShortAnswer    Type = "short_answer"
	TypeEssay          Type = "essay"
	TypeTrueFalse      Type = "true_false"
)

type Question struct {
	model.Base
	ExamID        uuid.UUID `json:"examId" db:"exam_id"`
	SequenceOrder int       `json:"sequenceOrder" db:"sequence_order"`
	Type          Type      `json:"type" db:"type"`
	Prompt        string    `json:"prompt" db:"prompt"`
	AudioPrompt   string    `json:"audioPrompt,omitempty" db:"audio_prompt"`
	Points        int       `json:"points" db:"points"`
}
