// Package recording keeps the audio a student dictates, together with the
// machine transcription of it, so a teacher can check a doubtful transcript
// against what was actually said.
//
// It is deliberately separate from the answer and exam-session logic: storing
// audio never decides, delays or blocks whether an answer is saved. The audio
// object is stored first and the metadata row second, so a row always points
// at audio that exists.
package recording

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrInvalidAudio means the audio handed to Save cannot be a valid recording.
	ErrInvalidAudio = errors.New("recording: invalid audio")
	// ErrStreamReused means the stream ID already belongs to a recording of a
	// different question or student, or of different audio. A client that
	// reuses a stream ID would otherwise have its new audio silently replaced
	// by the old.
	ErrStreamReused = errors.New("recording: stream id already used for another recording")
	// ErrSessionEnded means the session was submitted or expired before the
	// recording could be written. Nothing is kept.
	ErrSessionEnded = errors.New("recording: session is no longer in progress")
	// ErrStorage means the audio could not be stored. The caller should carry
	// on without a recording rather than fail the student's answer.
	ErrStorage = errors.New("recording: storage failed")
)

// Recording is the metadata of one retained audio segment.
type Recording struct {
	ID             uuid.UUID
	SessionID      uuid.UUID
	QuestionID     uuid.UUID
	StudentID      uuid.UUID
	StreamID       uuid.UUID
	SegmentIndex   int
	ContentType    string
	SampleRate     int
	DurationMillis int
	ByteSize       int64
	ChunkCount     int
	SHA256         []byte
	StorageKey     string
	RecordedAt     time.Time
	RetentionUntil *time.Time
}

// Transcription is what the recognizer produced for a recording. It is never
// edited; a student's correction is a separate answer revision.
type Transcription struct {
	ID          uuid.UUID
	RecordingID uuid.UUID
	Text        string
	Provider    string
	Language    string
	CreatedAt   time.Time
}

// Saved is the outcome of Save.
type Saved struct {
	RecordingID     uuid.UUID
	TranscriptionID uuid.UUID
	DurationMillis  int
	// Duplicate is true when this stream had already been saved, so nothing new
	// was stored.
	Duplicate bool
}
