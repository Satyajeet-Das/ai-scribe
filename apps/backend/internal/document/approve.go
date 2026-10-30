package document

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
)

// ApproveResult is the document after approval and the questions it created.
type ApproveResult struct {
	Document  *Document
	Questions []question.Question
}

// Approve copies the reviewed draft into the exam's questions. It is the only
// path from extracted content to the exam, it is taken only by an explicit
// request from the exam's owner (or an administrator), and it goes no further
// than a draft exam: publishing and assigning stay with the exam module.
//
// Everything happens in one transaction. The document row is locked first, so
// two approvals cannot run at once; then the exam row, so the exam cannot be
// published between the check that it is a draft and the import. If anything
// fails, nothing is added and the document is still waiting for review.
func (s *service) Approve(ctx context.Context, examID, documentID uuid.UUID, expectedVersion int, caller Caller) (*ApproveResult, error) {
	if _, err := s.load(ctx, examID, documentID, caller); err != nil {
		return nil, err
	}

	var created []question.Question
	err := s.tx.WithTx(ctx, func(tx database.DBTX) error {
		d, err := s.repo.GetForUpdate(ctx, documentID, tx)
		if err != nil {
			return err
		}
		switch d.Status {
		case StatusCompleted:
			return ErrAlreadyApproved
		case StatusReviewRequired:
		default:
			return ErrInvalidState
		}
		if d.DraftVersion != expectedVersion {
			return ErrStaleVersion
		}

		// Ownership is checked again against the locked row: the check before the
		// transaction could be out of date by now.
		ex, err := s.repo.LockExam(ctx, examID, tx)
		if err != nil {
			return err
		}
		if ex.Deleted || (caller.Role != platformauth.RoleAdmin && ex.CreatedBy != caller.ID) {
			return ErrNotFound
		}
		if ex.Status != exam.StatusDraft {
			return ErrExamNotDraft
		}

		qs, err := s.repo.ListDraft(ctx, documentID, tx)
		if err != nil {
			return err
		}
		if problems := problemsForApproval(qs); len(problems) > 0 {
			return &IncompleteError{Problems: problems}
		}

		items := make([]question.ImportItem, 0, len(qs))
		for _, q := range qs {
			it := question.ImportItem{
				SourceDocumentID: documentID, SourceDraftQuestionID: q.ID,
				Text: ComposeText(q), Type: question.Type(q.Type), Points: *q.Points,
			}
			for _, o := range q.Options {
				it.Options = append(it.Options, question.ImportOption{Key: strings.ToUpper(strings.TrimSpace(o.Label)), Text: o.Text, IsCorrect: o.IsCorrect})
			}
			items = append(items, it)
		}
		created, err = s.questions.ImportQuestions(ctx, tx, examID, items)
		if err != nil {
			if isUniqueViolation(err) {
				// Someone added a question to the exam at the same moment.
				return ErrConcurrentChange
			}
			return err
		}
		return s.repo.MarkApproved(ctx, documentID, caller.ID, tx)
	})
	if err != nil {
		return nil, err
	}

	d, err := s.repo.Get(ctx, documentID)
	if err != nil {
		return nil, err
	}
	s.log.Info().
		Str("event", "document.approved").
		Str("document_id", documentID.String()).
		Str("exam_id", examID.String()).
		Int("questions", len(created)).
		Msg("reviewed draft added to the exam")
	return &ApproveResult{Document: d, Questions: created}, nil
}
