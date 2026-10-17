package grading

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/audit"
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

// AccessMeta describes the request, for the audit record only.
type AccessMeta struct {
	RequestID string
	IP        string
}

// SaveMarkRequest is the complete state a teacher wants for one answer: this is
// a replacement, not a patch. A nil MarksHalf clears the teacher's mark (the
// automatic one, if any, applies again); a nil Comment clears the comment.
type SaveMarkRequest struct {
	// SessionID is the attempt the teacher was looking at.
	SessionID uuid.UUID
	MarksHalf *int
	Comment   *string
	// ExpectedVersion is the mark's version when the teacher read it: 0 if
	// nothing had been saved for the answer.
	ExpectedVersion int
}

// SaveMarkResult is the answer's new state and the paper's.
type SaveMarkResult struct {
	Line  Line
	Paper PaperSummary
}

// Service lets teachers mark papers.
type Service interface {
	GetMarking(ctx context.Context, examID, studentID uuid.UUID, caller Caller) (*Marking, error)
	// SaveMark records the marks and comment for one answer of a paper that has
	// ended and is still a draft.
	SaveMark(ctx context.Context, examID, studentID, questionID uuid.UUID, req SaveMarkRequest, caller Caller, meta AccessMeta) (*SaveMarkResult, error)
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

// normalizeComment trims a comment and turns an empty one into nil. It refuses a
// comment the database could not store.
func normalizeComment(c *string) (*string, error) {
	if c == nil {
		return nil, nil
	}
	text := strings.TrimSpace(*c)
	switch {
	case !utf8.ValidString(text) || strings.ContainsRune(text, 0):
		return nil, &ValidationError{Field: "comment", Message: "the comment contains characters that cannot be saved"}
	case utf8.RuneCountInString(text) > MaxCommentRunes:
		return nil, &ValidationError{Field: "comment", Message: "the comment is too long"}
	case text == "":
		return nil, nil
	}
	return &text, nil
}

func sameInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (s *service) SaveMark(ctx context.Context, examID, studentID, questionID uuid.UUID, req SaveMarkRequest, caller Caller, meta AccessMeta) (*SaveMarkResult, error) {
	if err := s.authorize(ctx, examID, caller); err != nil {
		return nil, err
	}
	comment, err := normalizeComment(req.Comment)
	if err != nil {
		return nil, err
	}
	if req.ExpectedVersion < 0 {
		return nil, &ValidationError{Field: "expectedVersion", Message: "must not be negative"}
	}

	var result *SaveMarkResult
	err = s.repo.Write(ctx, func(w Writer) error {
		sess, err := w.Session(ctx, examID, studentID)
		if err != nil {
			return err
		}
		if sess == nil {
			return ErrSessionNotEnded
		}
		if sess.ID != req.SessionID {
			return ErrSessionChanged
		}
		if sess.Status == review.StatusInProgress {
			return ErrSessionNotEnded
		}

		// Everything that changes this paper takes this lock first. From here on
		// no other save, finalise or reopen of it can run, so what is checked
		// below is still true when it is written.
		paper, err := w.LockPaper(ctx, examID, sess.ID)
		if err != nil {
			return err
		}
		if paper.Status == StatusFinal {
			return ErrPaperFinalised
		}

		q, err := w.Question(ctx, examID, questionID)
		if err != nil {
			return err
		}
		if q == nil {
			return ErrExamNotFound
		}
		maxHalf := q.Points * 2
		if req.MarksHalf != nil {
			if err := ValidateHalfMarks(*req.MarksHalf, maxHalf); err != nil {
				return &ValidationError{Field: "marks", Message: validationText(err)}
			}
		}

		existing, err := w.Mark(ctx, sess.ID, questionID)
		if err != nil {
			return err
		}
		currentVersion := 0
		var prevMarks *int
		var prevComment *string
		if existing != nil {
			currentVersion, prevMarks, prevComment = existing.Version, existing.MarksHalf, existing.Comment
		}

		// Asking for what is already there changes nothing, so a retry after a
		// lost reply is safe, even though the version it carries is now old.
		unchanged := sameInt(prevMarks, req.MarksHalf) && sameString(prevComment, comment)
		if !unchanged {
			if req.ExpectedVersion != currentVersion {
				return ErrStaleVersion
			}
			written, err := w.UpsertMark(ctx, sess.ID, questionID, MarkWrite{
				MarksHalf: req.MarksHalf, MaxHalf: maxHalf, Comment: comment, MarkedBy: caller.ID,
				ExpectedVersion: req.ExpectedVersion,
			})
			if err != nil {
				return err
			}
			paperVersion, err := w.BumpPaper(ctx, sess.ID)
			if err != nil {
				return err
			}
			if err := w.Audit(ctx, audit.Event{
				ActorID: caller.ID, ActorRole: string(caller.Role),
				Action: audit.ActionMarkSave, ResourceType: audit.ResourceAnswerMark,
				ResourceID: questionID, SessionID: sess.ID, Outcome: audit.OutcomeOK,
				RequestID: meta.RequestID, IP: meta.IP,
				// Ids, marks and counts only: never the comment, never the answer.
				Metadata: map[string]any{
					"exam_id": examID.String(), "student_id": studentID.String(),
					"marks_half": req.MarksHalf, "previous_marks_half": prevMarks, "max_half": maxHalf,
					"cleared":         prevMarks != nil && req.MarksHalf == nil,
					"comment_changed": !sameString(prevComment, comment),
					"version":         written.Version, "paper_version": paperVersion,
				},
			}); err != nil {
				s.auditFailed(examID, questionID, err)
				return ErrAuditUnavailable
			}
		}

		marking, err := readMarking(ctx, w, examID, studentID)
		if err != nil {
			return err
		}
		for _, l := range marking.Lines {
			if l.QuestionID == questionID {
				result = &SaveMarkResult{Line: l, Paper: marking.Paper}
				return nil
			}
		}
		return ErrExamNotFound
	})
	if err != nil {
		if errors.Is(err, review.ErrStudentNotAssigned) {
			return nil, ErrExamNotFound
		}
		return nil, err
	}
	return result, nil
}

// validationText is the human part of a marks error.
func validationText(err error) string {
	msg := err.Error()
	if _, after, found := strings.Cut(msg, ": "); found {
		return after
	}
	return msg
}

func (s *service) auditFailed(examID, questionID uuid.UUID, err error) {
	if s.logger == nil {
		return
	}
	s.logger.Error().Err(err).Str("event", "grading.audit_failed").
		Str("exam_id", examID.String()).Str("question_id", questionID.String()).
		Msg("mark not saved: the change could not be recorded")
}
