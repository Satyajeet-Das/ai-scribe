// Package review lets an authorized teacher inspect the answers students have
// saved for an exam. It is strictly read-only: nothing in this package writes
// to the database, and it never changes a session, an answer or an exam.
package review

import (
	"time"

	"github.com/google/uuid"
)

// Status is a student's exam status as persisted in the sessions table, plus
// StatusNotStarted for assigned students who have no session at all.
type Status string

const (
	StatusNotStarted Status = "NOT_STARTED"
	StatusInProgress Status = "IN_PROGRESS"
	StatusSubmitted  Status = "SUBMITTED"
	StatusExpired    Status = "EXPIRED"
)

func (s Status) Valid() bool {
	switch s {
	case StatusNotStarted, StatusInProgress, StatusSubmitted, StatusExpired:
		return true
	}
	return false
}

// SortField and SortOrder select the roster order. Every order ends in the
// student ID, so pages never overlap or skip rows.
type SortField string

const (
	SortName        SortField = "name"
	SortRollNo      SortField = "rollNo"
	SortSubmittedAt SortField = "submittedAt"
)

type SortOrder string

const (
	OrderAsc  SortOrder = "asc"
	OrderDesc SortOrder = "desc"
)

// ListParams are validated roster filters, produced by ListQuery.Normalize.
type ListParams struct {
	ExamID uuid.UUID
	Search string
	Status *Status
	Sort   SortField
	Order  SortOrder
	Limit  int
	Offset int
}

// StudentRow is one assigned student and the state of their exam.
type StudentRow struct {
	StudentID   uuid.UUID
	RollNo      string
	Name        string
	Status      Status
	SessionID   *uuid.UUID
	StartedAt   *time.Time
	SubmittedAt *time.Time
	// AnsweredCount is nil when the student has no session.
	AnsweredCount *int
}

// StatusCounts covers every student assigned to the exam, ignoring any
// search or status filter on the list.
type StatusCounts struct {
	Assigned   int
	NotStarted int
	InProgress int
	Submitted  int
	Expired    int
}

type StudentPage struct {
	Rows           []StudentRow
	Total          int // rows matching the filters
	Counts         StatusCounts
	TotalQuestions int
}

type SessionInfo struct {
	ID          uuid.UUID
	Status      Status
	StartedAt   time.Time
	SubmittedAt *time.Time
}

type OptionRecord struct {
	ID           uuid.UUID
	Key          string
	Text         string
	DisplayOrder int
	IsCorrect    bool
}

type AnswerRecord struct {
	SelectedOptionID *uuid.UUID
	TextAnswer       string
	UpdatedAt        time.Time
	// Source says where the current value came from: OPTION, TYPED,
	// STT_CONFIRMED or STUDENT_CORRECTION. Empty for answers saved before
	// revisions were recorded.
	Source string
}

// Recording statuses as shown to a reviewer.
const (
	RecordingAvailable = "AVAILABLE"
	// RecordingRemoved means the audio was deleted (for example when its
	// retention period ended). Its transcription is kept.
	RecordingRemoved = "REMOVED"
)

// RecordingRecord is the metadata of one retained audio segment and the
// machine transcription made from it. It never holds the audio or its storage
// location.
type RecordingRecord struct {
	ID             uuid.UUID
	SegmentIndex   int
	Status         string
	DurationMillis int
	RecordedAt     time.Time
	// Transcript is what the recognizer produced, unedited.
	Transcript string
	// Confirmed is true when the student confirmed an answer that used this
	// recording, so its audio supports the saved answer.
	Confirmed bool
}

type QuestionRecord struct {
	ID             uuid.UUID
	QuestionNumber int
	Text           string
	Type           string
	Points         int
	Options        []OptionRecord
	// Answer is nil when no answer row exists.
	Answer *AnswerRecord
	// Recordings are the audio segments kept for this question, oldest first.
	// Empty when none were kept, which is the normal case.
	Recordings []RecordingRecord
}

// Responses is everything the repository reads for one student's review.
type Responses struct {
	StudentID      uuid.UUID
	RollNo         string
	Name           string
	Session        *SessionInfo // nil when the student never started
	TotalQuestions int
	// Loaded reports whether Questions was read. It is false when there is no
	// session, or the session is in progress and the policy forbids viewing it.
	Loaded    bool
	Questions []QuestionRecord
}

// IsAnswered is the single definition of "answered" used for counts and for
// the per-question status: an option was chosen, or non-blank text was saved.
func IsAnswered(a *AnswerRecord) bool {
	if a == nil {
		return false
	}
	if a.SelectedOptionID != nil {
		return true
	}
	for _, r := range a.TextAnswer {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return true
		}
	}
	return false
}

// RecordingFile is what is needed to play one recording. StorageKey is internal
// and never leaves the service.
type RecordingFile struct {
	ID            uuid.UUID
	SessionID     uuid.UUID
	SessionStatus Status
	StorageKey    string
	ContentType   string
	ByteSize      int64
	Removed       bool
}
