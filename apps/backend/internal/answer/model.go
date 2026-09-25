package answer

import (
	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Answer struct {
	model.Base
	SessionID        uuid.UUID  `json:"sessionId" db:"session_id"`
	QuestionID       uuid.UUID  `json:"questionId" db:"question_id"`
	SelectedOptionID *uuid.UUID `json:"selectedOptionId,omitempty" db:"selected_option_id"`
	TextAnswer       string     `json:"textAnswer" db:"text_answer"`
}
