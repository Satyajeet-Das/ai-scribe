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

	// Revision describes where this value came from. It is set by the server,
	// never read from a request, and is optional: without it the value is
	// recorded as typed text or a selected option.
	Revision *RevisionInput `json:"-" db:"-"`

	// Replayed is set by the repository when this save repeated a confirmation
	// that had already been applied. Nothing was written, and the other fields
	// then describe the answer as it currently is, not as the replay asked.
	Replayed bool `json:"-" db:"-"`
}

// Revision sources. STT_CONFIRMED and STUDENT_CORRECTION revisions are never
// modified once written.
const (
	SourceOption            = "OPTION"
	SourceTyped             = "TYPED"
	SourceSTTConfirmed      = "STT_CONFIRMED"
	SourceStudentCorrection = "STUDENT_CORRECTION"
)

// RevisionInput is the provenance of a saved answer value.
type RevisionInput struct {
	Source string
	// TranscriptionID is the machine transcription the text came from.
	TranscriptionID *uuid.UUID
	// ConfirmationID makes a repeated confirmation add no second revision.
	ConfirmationID string
	// RecordingIDs are the audio segments that support this value.
	RecordingIDs []uuid.UUID
}
