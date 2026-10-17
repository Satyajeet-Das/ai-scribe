package grading

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Status is where a paper is in marking.
type Status string

const (
	// StatusDraft marks can still be changed.
	StatusDraft Status = "DRAFT"
	// StatusFinal marks are frozen in a snapshot and cannot be changed until the
	// paper is reopened.
	StatusFinal Status = "FINAL"
)

// MarkSource says where a question's effective mark came from.
type MarkSource string

const (
	SourceTeacher MarkSource = "TEACHER"
	SourceAuto    MarkSource = "AUTO"
	// SourceNone means the question has no mark yet.
	SourceNone MarkSource = "NONE"
)

// Question types, as stored.
const (
	TypeMCQ   = "MCQ"
	TypeEssay = "ESSAY"
	TypeVoice = "VOICE"
)

var (
	// ErrInvalidMarks means a mark is not a multiple of 0.5 within 0 and the
	// question's maximum. It carries a message fit to show a teacher.
	ErrInvalidMarks = errors.New("invalid marks")
	// ErrStaleVersion means the mark or paper changed since the caller read it.
	ErrStaleVersion = errors.New("marks changed since they were read")
	// ErrPaperFinalised means the paper is final and cannot be changed.
	ErrPaperFinalised = errors.New("paper is finalised")
	// ErrSessionNotEnded means the student is still sitting the exam.
	ErrSessionNotEnded = errors.New("the exam has not ended for this student")
	// ErrSessionChanged means the attempt being marked is no longer the one the
	// review shows for this student (for example after a retake was submitted).
	ErrSessionChanged = errors.New("a different attempt is now shown for this student")
	// ErrPaperIncomplete means some questions have no mark yet. See IncompleteError.
	ErrPaperIncomplete = errors.New("some questions have no mark")
	// ErrExamNotFound covers a missing exam, one the caller does not own, and an
	// unknown student, attempt or question, so nothing reveals what exists.
	ErrExamNotFound = errors.New("exam not found")
	// ErrForbiddenRole means the caller's role may never mark papers.
	ErrForbiddenRole = errors.New("forbidden: only teachers and administrators can mark papers")
	// ErrNotFinal means there is nothing to reopen.
	ErrNotFinal = errors.New("paper is not finalised")
	// ErrInvalidRequest wraps a message about a malformed request.
	ErrInvalidRequest = errors.New("invalid request")
	// ErrAuditUnavailable means the change could not be recorded, so it was not made.
	ErrAuditUnavailable = errors.New("change could not be recorded")
)

// IncompleteError says which questions still need a mark. It matches
// ErrPaperIncomplete with errors.Is.
type IncompleteError struct {
	// Questions are question numbers, ascending.
	Questions []int
}

func (e *IncompleteError) Error() string {
	nums := make([]string, len(e.Questions))
	for i, n := range e.Questions {
		nums[i] = fmt.Sprint(n)
	}
	return "questions without a mark: " + strings.Join(nums, ", ")
}

func (e *IncompleteError) Is(target error) bool { return target == ErrPaperIncomplete }

// QuestionInput is what the rules need to know about one question and the
// student's answer to it.
type QuestionInput struct {
	ID     uuid.UUID
	Number int
	Type   string
	Points int
	// Answered is review.IsAnswered for the student's answer.
	Answered         bool
	SelectedOptionID *uuid.UUID
	// CorrectOptionIDs are the options marked correct. Normally one.
	CorrectOptionIDs []uuid.UUID
}

// MaxHalf is the question's maximum in half-marks.
func (q QuestionInput) MaxHalf() int { return q.Points * 2 }

// TeacherMark is what a teacher saved for one answer.
type TeacherMark struct {
	// MarksHalf is nil when only a comment was saved, or the mark was cleared.
	MarksHalf *int
	Comment   *string
	Version   int
	MarkedAt  time.Time
}

// Line is the marking state of one question.
type Line struct {
	QuestionID     uuid.UUID
	QuestionNumber int
	Type           string
	MaxHalf        int
	Answered       bool
	// CorrectOptionID is set for a multiple-choice question that has a correct
	// option, while the paper is a draft. It is for the teacher only.
	CorrectOptionID *uuid.UUID
	// SuggestedHalf is the automatic mark, or nil when the teacher must decide.
	SuggestedHalf *int
	// TeacherHalf is the teacher's mark, if any.
	TeacherHalf *int
	// EffectiveHalf is what counts: the teacher's mark, else the suggestion.
	EffectiveHalf *int
	Source        MarkSource
	Comment       *string
	// Version is the mark's version, 0 when the teacher has saved nothing.
	Version  int
	MarkedAt *time.Time
	// ExceedsMax is true when the teacher's mark is higher than the question's
	// maximum now (its points were lowered after marking). It does not count
	// until the teacher saves a valid mark.
	ExceedsMax bool
}

// Totals summarises a paper.
type Totals struct {
	TotalHalf int
	MaxHalf   int
	// Marked is the number of questions with an effective mark.
	Marked    int
	Questions int
	// Unmarked are the numbers of the questions without one, ascending.
	Unmarked []int
}

// Complete reports whether every question has a mark.
func (t Totals) Complete() bool { return len(t.Unmarked) == 0 }

// SnapshotLine is one question's frozen mark.
type SnapshotLine struct {
	QuestionID     uuid.UUID  `json:"questionId"`
	QuestionNumber int        `json:"questionNumber"`
	Type           string     `json:"type"`
	MaxHalf        int        `json:"maxHalf"`
	AwardedHalf    int        `json:"awardedHalf"`
	Source         MarkSource `json:"source"`
	Comment        *string    `json:"comment,omitempty"`
}

// Snapshot is a paper's marks as they were when it was finalised.
type Snapshot struct {
	Version int            `json:"version"`
	Lines   []SnapshotLine `json:"lines"`
}

// SnapshotVersion identifies the snapshot layout.
const SnapshotVersion = 1

func sortedNumbers(n []int) []int {
	out := append([]int(nil), n...)
	sort.Ints(out)
	return out
}
