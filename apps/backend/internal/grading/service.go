package grading

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/review"
)

// Caller is the authenticated identity, taken from the verified token and never
// from the request.
type Caller struct {
	ID   uuid.UUID
	Role platformauth.Role
}

func (c Caller) canMark() bool {
	return c.Role == platformauth.RoleAdmin || c.Role == platformauth.RoleTeacher
}

func (c Caller) examCaller() exam.Caller { return exam.Caller{ID: c.ID, Role: c.Role} }

// ExamAuthorizer is the exam module's ownership check, as the review uses it.
type ExamAuthorizer interface {
	GetExamForCaller(ctx context.Context, id uuid.UUID, caller exam.Caller) (*exam.Exam, error)
}

// Why marking is not available for a paper.
const (
	ReasonNotStarted      = "NOT_STARTED"
	ReasonSessionNotEnded = "SESSION_NOT_ENDED"
)

// PaperSummary is where a paper stands.
type PaperSummary struct {
	Status      Status
	Version     int
	TotalHalf   int
	MaxHalf     int
	Marked      int
	Questions   int
	FinalisedAt *time.Time
	FinalisedBy *uuid.UUID
}

// Marking is everything a teacher needs to mark one student's paper.
type Marking struct {
	ExamID    uuid.UUID
	StudentID uuid.UUID
	// SessionID is the attempt being marked; nil if the student never started.
	SessionID     *uuid.UUID
	SessionStatus review.Status
	// MarkingAllowed is false while there is nothing to mark yet; Reason says why.
	MarkingAllowed bool
	Reason         string
	Paper          PaperSummary
	Lines          []Line
}

// Service lets teachers mark papers.
type Service interface {
	GetMarking(ctx context.Context, examID, studentID uuid.UUID, caller Caller) (*Marking, error)
}

type service struct {
	repo   Repository
	exams  ExamAuthorizer
	logger *zerolog.Logger
}

func NewService(repo Repository, exams ExamAuthorizer, logger *zerolog.Logger) Service {
	return &service{repo: repo, exams: exams, logger: logger}
}

// authorize checks the role, then that the caller owns the exam or is an
// administrator. A missing exam and one the caller does not own look the same.
func (s *service) authorize(ctx context.Context, examID uuid.UUID, caller Caller) error {
	if !caller.canMark() || caller.ID == uuid.Nil {
		s.denied(examID, caller, "role")
		return ErrForbiddenRole
	}
	if _, err := s.exams.GetExamForCaller(ctx, examID, caller.examCaller()); err != nil {
		if errors.Is(err, exam.ErrExamNotFound) || errors.Is(err, exam.ErrUnauthorizedCreator) {
			if errors.Is(err, exam.ErrUnauthorizedCreator) {
				s.denied(examID, caller, "ownership")
			}
			return ErrExamNotFound
		}
		return err
	}
	return nil
}

func (s *service) denied(examID uuid.UUID, caller Caller, reason string) {
	if s.logger == nil {
		return
	}
	s.logger.Warn().Str("event", "grading.denied").Str("reason", reason).
		Str("exam_id", examID.String()).Str("user_id", caller.ID.String()).Msg("marking denied")
}

func (s *service) GetMarking(ctx context.Context, examID, studentID uuid.UUID, caller Caller) (*Marking, error) {
	if err := s.authorize(ctx, examID, caller); err != nil {
		return nil, err
	}
	var out *Marking
	err := s.repo.Read(ctx, func(r Reader) error {
		m, err := readMarking(ctx, r, examID, studentID)
		out = m
		return err
	})
	if err != nil {
		if errors.Is(err, review.ErrStudentNotAssigned) {
			return nil, ErrExamNotFound
		}
		return nil, err
	}
	return out, nil
}

// readMarking describes a paper's marking state from a consistent view.
func readMarking(ctx context.Context, r Reader, examID, studentID uuid.UUID) (*Marking, error) {
	sess, err := r.Session(ctx, examID, studentID)
	if err != nil {
		return nil, err
	}
	out := &Marking{ExamID: examID, StudentID: studentID, Paper: PaperSummary{Status: StatusDraft}, Lines: []Line{}}
	if sess == nil {
		out.Reason = ReasonNotStarted
		return out, nil
	}
	id := sess.ID
	out.SessionID, out.SessionStatus = &id, sess.Status
	if sess.Status == review.StatusInProgress {
		// Nothing to mark while the student is still sitting the exam, and no
		// reason to show the answer key for it.
		out.Reason = ReasonSessionNotEnded
		return out, nil
	}
	out.MarkingAllowed = true

	paper, err := r.Paper(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	if paper != nil {
		out.Paper.Version = paper.Version
	}

	if paper != nil && paper.Status == StatusFinal && paper.Snapshot != nil {
		// A final paper is shown as it was frozen, whatever the questions say now.
		out.Lines = linesFromSnapshot(*paper.Snapshot)
		out.Paper.Status = StatusFinal
		out.Paper.FinalisedAt, out.Paper.FinalisedBy = paper.FinalisedAt, paper.FinalisedBy
		out.Paper.Questions = len(out.Lines)
		if paper.TotalHalf != nil {
			out.Paper.TotalHalf = *paper.TotalHalf
		}
		if paper.MaxHalf != nil {
			out.Paper.MaxHalf = *paper.MaxHalf
		}
		out.Paper.Marked = len(out.Lines)
		return out, nil
	}

	questions, err := r.Questions(ctx, examID, sess.ID)
	if err != nil {
		return nil, err
	}
	marks, err := r.Marks(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	for _, q := range questions {
		var saved *TeacherMark
		if m, ok := marks[q.ID]; ok {
			saved = &TeacherMark{MarksHalf: m.MarksHalf, Comment: m.Comment, Version: m.Version, MarkedAt: m.MarkedAt}
		}
		out.Lines = append(out.Lines, BuildLine(q, saved))
	}
	totals := Summarise(out.Lines)
	out.Paper.TotalHalf, out.Paper.MaxHalf = totals.TotalHalf, totals.MaxHalf
	out.Paper.Marked, out.Paper.Questions = totals.Marked, totals.Questions
	return out, nil
}

// linesFromSnapshot turns frozen marks back into lines. They carry no answer,
// suggestion or answer key: the snapshot is the record, not the question.
func linesFromSnapshot(s Snapshot) []Line {
	out := make([]Line, 0, len(s.Lines))
	for _, l := range s.Lines {
		awarded := l.AwardedHalf
		out = append(out, Line{
			QuestionID:     l.QuestionID,
			QuestionNumber: l.QuestionNumber,
			Type:           l.Type,
			MaxHalf:        l.MaxHalf,
			EffectiveHalf:  &awarded,
			Source:         l.Source,
			Comment:        l.Comment,
			FromSnapshot:   true,
		})
	}
	return out
}
