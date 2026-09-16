package answer

import (
	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Answer struct {
	model.Base
	SessionID    uuid.UUID `json:"sessionId" db:"session_id"`
	QuestionID   uuid.UUID `json:"questionId" db:"question_id"`
	CandidateID  uuid.UUID `json:"candidateId" db:"candidate_id"`
	ResponseText string    `json:"responseText" db:"response_text"`
	AudioURL     string    `json:"audioUrl,omitempty" db:"audio_url"`
	Score        *float64  `json:"score,omitempty" db:"score"`
}
