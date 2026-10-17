package grading

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/audit"
	"github.com/Satyajeet-Das/ai-scribe/internal/review"
)

// Limits on the reason given for reopening a paper, in characters.
const (
	MinReasonRunes = 3
	MaxReasonRunes = 500
)

// FinaliseRequest names the paper to finalise as the teacher last saw it.
type FinaliseRequest struct {
	SessionID uuid.UUID
	// ExpectedPaperVersion is the paper's version when the teacher read it. If the
	// paper has changed since, they have not seen what they are about to freeze.
	ExpectedPaperVersion int
}

// ReopenRequest names the paper to reopen and why.
type ReopenRequest struct {
	SessionID uuid.UUID
	Reason    string
}

func normalizeReason(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	switch {
	case !utf8.ValidString(text) || strings.ContainsRune(text, 0):
		return "", &ValidationError{Field: "reason", Message: "the reason contains characters that cannot be saved"}
	case utf8.RuneCountInString(text) < MinReasonRunes:
		return "", &ValidationError{Field: "reason", Message: "give a reason of at least 3 characters"}
	case utf8.RuneCountInString(text) > MaxReasonRunes:
		return "", &ValidationError{Field: "reason", Message: "the reason is too long"}
	}
	return text, nil
}

// lockedPaper resolves the attempt the request is about, checks it is the one
// the teacher was looking at and has ended, and locks its grading record.
func lockedPaper(ctx context.Context, w Writer, examID, studentID, sessionID uuid.UUID) (*review.SessionInfo, *PaperRow, error) {
	sess, err := w.Session(ctx, examID, studentID)
	if err != nil {
		return nil, nil, err
	}
	if sess == nil {
		return nil, nil, ErrSessionNotEnded
	}
	if sess.ID != sessionID {
		return nil, nil, ErrSessionChanged
	}
	if sess.Status == review.StatusInProgress {
		return nil, nil, ErrSessionNotEnded
	}
	paper, err := w.LockPaper(ctx, examID, sess.ID)
	if err != nil {
		return nil, nil, err
	}
	return sess, paper, nil
}

func (s *service) Finalise(ctx context.Context, examID, studentID uuid.UUID, req FinaliseRequest, caller Caller, meta AccessMeta) (*Marking, error) {
	if err := s.authorize(ctx, examID, caller); err != nil {
		return nil, err
	}
	if req.ExpectedPaperVersion < 0 {
		return nil, &ValidationError{Field: "expectedPaperVersion", Message: "must not be negative"}
	}

	var out *Marking
	err := s.repo.Write(ctx, func(w Writer) error {
		sess, paper, err := lockedPaper(ctx, w, examID, studentID, req.SessionID)
		if err != nil {
			return err
		}
		// Already final: the teacher's goal is met, whatever version they hold.
		if paper.Status != StatusFinal {
			if req.ExpectedPaperVersion != paper.Version {
				return ErrStaleVersion
			}
			// The marks as they stand now, under the lock: nothing can change them
			// between this read and the write below.
			current, err := readMarking(ctx, w, examID, studentID)
			if err != nil {
				return err
			}
			snap, totals, err := BuildSnapshot(current.Lines)
			if err != nil {
				return err
			}
			version, err := w.SetFinal(ctx, sess.ID, caller.ID, snap, totals.TotalHalf, totals.MaxHalf)
			if err != nil {
				return err
			}
			total, max := totals.TotalHalf, totals.MaxHalf
			if err := w.AddEvent(ctx, GradingEvent{
				SessionID: sess.ID, ExamID: examID, Event: EventFinalised,
				ActorID: caller.ID, ActorRole: string(caller.Role),
				TotalHalf: &total, MaxHalf: &max, Snapshot: &snap,
			}); err != nil {
				return err
			}
			if err := w.Audit(ctx, audit.Event{
				ActorID: caller.ID, ActorRole: string(caller.Role),
				Action: audit.ActionPaperFinalise, ResourceType: audit.ResourcePaper,
				ResourceID: sess.ID, SessionID: sess.ID, Outcome: audit.OutcomeOK,
				RequestID: meta.RequestID, IP: meta.IP,
				Metadata: map[string]any{
					"exam_id": examID.String(), "student_id": studentID.String(),
					"total_half": total, "max_half": max, "questions": totals.Questions,
					"paper_version": version,
				},
			}); err != nil {
				s.paperAuditFailed("finalise", examID, err)
				return ErrAuditUnavailable
			}
		}
		out, err = readMarking(ctx, w, examID, studentID)
		return err
	})
	return out, mapNotAssigned(err)
}

func (s *service) Reopen(ctx context.Context, examID, studentID uuid.UUID, req ReopenRequest, caller Caller, meta AccessMeta) (*Marking, error) {
	if err := s.authorize(ctx, examID, caller); err != nil {
		return nil, err
	}
	reason, err := normalizeReason(req.Reason)
	if err != nil {
		return nil, err
	}

	var out *Marking
	err = s.repo.Write(ctx, func(w Writer) error {
		sess, paper, err := lockedPaper(ctx, w, examID, studentID, req.SessionID)
		if err != nil {
			return err
		}
		if paper.Status == StatusFinal {
			version, err := w.SetDraft(ctx, sess.ID)
			if err != nil {
				return err
			}
			if err := w.AddEvent(ctx, GradingEvent{
				SessionID: sess.ID, ExamID: examID, Event: EventReopened,
				ActorID: caller.ID, ActorRole: string(caller.Role), Reason: &reason,
				TotalHalf: paper.TotalHalf, MaxHalf: paper.MaxHalf, Snapshot: paper.Snapshot,
			}); err != nil {
				return err
			}
			// The reason lives in the history table; the audit entry holds ids and
			// totals only.
			if err := w.Audit(ctx, audit.Event{
				ActorID: caller.ID, ActorRole: string(caller.Role),
				Action: audit.ActionPaperReopen, ResourceType: audit.ResourcePaper,
				ResourceID: sess.ID, SessionID: sess.ID, Outcome: audit.OutcomeOK,
				RequestID: meta.RequestID, IP: meta.IP,
				Metadata: map[string]any{
					"exam_id": examID.String(), "student_id": studentID.String(),
					"previous_total_half": paper.TotalHalf, "previous_max_half": paper.MaxHalf,
					"paper_version": version,
				},
			}); err != nil {
				s.paperAuditFailed("reopen", examID, err)
				return ErrAuditUnavailable
			}
		}
		out, err = readMarking(ctx, w, examID, studentID)
		return err
	})
	return out, mapNotAssigned(err)
}

func mapNotAssigned(err error) error {
	if errors.Is(err, review.ErrStudentNotAssigned) {
		return ErrExamNotFound
	}
	return err
}

func (s *service) paperAuditFailed(action string, examID uuid.UUID, err error) {
	if s.logger == nil {
		return
	}
	s.logger.Error().Err(err).Str("event", "grading.audit_failed").Str("action", action).
		Str("exam_id", examID.String()).Msg("paper not changed: the change could not be recorded")
}
