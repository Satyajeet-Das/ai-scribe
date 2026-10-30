package document

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
)

// Draft is a document with the questions extracted from it, for review.
type Draft struct {
	Document  *Document
	Questions []DraftQuestion
}

// QuestionResult is one draft question after a change, and the draft's version.
type QuestionResult struct {
	Question     DraftQuestion
	DraftVersion int
}

// AddQuestionRequest adds a question the extraction missed.
type AddQuestionRequest struct {
	ExpectedDraftVersion int
	// Position is where the question goes (1 is first); 0, or anything past the
	// end, puts it last.
	Position int
	Question QuestionInput
}

// ReplaceQuestionRequest replaces everything a teacher can edit in a question.
type ReplaceQuestionRequest struct {
	ExpectedDraftVersion int
	Question             QuestionInput
}

// ReorderRequest gives the whole draft in its new order.
type ReorderRequest struct {
	ExpectedDraftVersion int
	QuestionIDs          []uuid.UUID
}

func (s *service) GetDraft(ctx context.Context, examID, documentID uuid.UUID, caller Caller) (*Draft, error) {
	d, err := s.load(ctx, examID, documentID, caller)
	if err != nil {
		return nil, err
	}
	// A draft exists once extraction has finished, and stays readable after it
	// was approved. Before that there is nothing to show yet.
	if d.Status != StatusReviewRequired && d.Status != StatusCompleted {
		return nil, ErrInvalidState
	}
	qs, err := s.repo.ListDraft(ctx, documentID)
	if err != nil {
		return nil, err
	}
	return &Draft{Document: d, Questions: qs}, nil
}

// editDraft runs a change to the draft under the document's row lock. The
// caller's version must be the current one, and the document must be waiting for
// review: a draft that is being re-extracted, or was approved, cannot be edited.
// The draft's version rises by one when fn succeeds.
func (s *service) editDraft(ctx context.Context, examID, documentID uuid.UUID, expected int, caller Caller,
	fn func(tx database.DBTX, d *Document) error) (*Document, error) {
	if _, err := s.load(ctx, examID, documentID, caller); err != nil {
		return nil, err
	}
	var out *Document
	err := s.tx.WithTx(ctx, func(tx database.DBTX) error {
		d, err := s.repo.GetForUpdate(ctx, documentID, tx)
		if err != nil {
			return err
		}
		switch d.Status {
		case StatusReviewRequired:
		case StatusCompleted:
			return ErrAlreadyApproved
		default:
			return ErrInvalidState
		}
		if d.DraftVersion != expected {
			return ErrStaleVersion
		}
		if err := fn(tx, d); err != nil {
			return err
		}
		v, err := s.repo.BumpDraftVersion(ctx, documentID, tx)
		if err != nil {
			return err
		}
		d.DraftVersion = v
		out = d
		return nil
	})
	return out, err
}

func (s *service) AddQuestion(ctx context.Context, examID, documentID uuid.UUID, req AddQuestionRequest, caller Caller) (*QuestionResult, error) {
	var created DraftQuestion
	d, err := s.editDraft(ctx, examID, documentID, req.ExpectedDraftVersion, caller, func(tx database.DBTX, d *Document) error {
		if err := validateInput(req.Question, d.PageCount); err != nil {
			return err
		}
		existing, err := s.repo.ListDraft(ctx, documentID, tx)
		if err != nil {
			return err
		}
		if len(existing) >= maxDraftQuestions {
			return ErrDraftTooLarge
		}
		pos := req.Position
		if pos < 1 || pos > len(existing)+1 {
			pos = len(existing) + 1
		}
		if pos <= len(existing) {
			if err := s.repo.ShiftPositions(ctx, documentID, pos, tx); err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		q := DraftQuestion{
			ID: uuid.New(), DocumentID: documentID, Position: pos,
			Confidence: extract.ConfidenceHigh, Origin: OriginTeacher,
			SourcePages: []int{}, Warnings: []extract.Warning{}, CreatedAt: now, UpdatedAt: now,
		}
		applyInput(&q, req.Question)
		q.Edited = false // it was written by a teacher; nothing was changed after extraction
		if err := s.repo.InsertDraftQuestion(ctx, &q, tx); err != nil {
			return err
		}
		created = q
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.log.Info().Str("event", "document.draft.question_added").Str("document_id", documentID.String()).Msg("question added to draft")
	return &QuestionResult{Question: created, DraftVersion: d.DraftVersion}, nil
}

func (s *service) ReplaceQuestion(ctx context.Context, examID, documentID, questionID uuid.UUID, req ReplaceQuestionRequest, caller Caller) (*QuestionResult, error) {
	var updated DraftQuestion
	d, err := s.editDraft(ctx, examID, documentID, req.ExpectedDraftVersion, caller, func(tx database.DBTX, d *Document) error {
		if err := validateInput(req.Question, d.PageCount); err != nil {
			return err
		}
		q, err := s.repo.GetDraftQuestion(ctx, documentID, questionID, tx)
		if err != nil {
			return err
		}
		applyInput(q, req.Question)
		if err := s.repo.UpdateDraftQuestion(ctx, q, tx); err != nil {
			return err
		}
		updated = *q
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &QuestionResult{Question: updated, DraftVersion: d.DraftVersion}, nil
}

func (s *service) DeleteQuestion(ctx context.Context, examID, documentID, questionID uuid.UUID, expectedVersion int, caller Caller) (*Draft, error) {
	var qs []DraftQuestion
	d, err := s.editDraft(ctx, examID, documentID, expectedVersion, caller, func(tx database.DBTX, d *Document) error {
		if err := s.repo.DeleteDraftQuestion(ctx, documentID, questionID, tx); err != nil {
			return err
		}
		var err error
		qs, err = s.repo.ListDraft(ctx, documentID, tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Draft{Document: d, Questions: qs}, nil
}

func (s *service) Reorder(ctx context.Context, examID, documentID uuid.UUID, req ReorderRequest, caller Caller) (*Draft, error) {
	var qs []DraftQuestion
	d, err := s.editDraft(ctx, examID, documentID, req.ExpectedDraftVersion, caller, func(tx database.DBTX, d *Document) error {
		current, err := s.repo.ListDraft(ctx, documentID, tx)
		if err != nil {
			return err
		}
		// The new order must name every question exactly once: a partial list
		// would leave the rest with positions that collide.
		if len(req.QuestionIDs) != len(current) {
			return invalid("questionIds", "must list every question in the draft exactly once (%d expected)", len(current))
		}
		have := make(map[uuid.UUID]bool, len(current))
		for _, q := range current {
			have[q.ID] = true
		}
		seen := make(map[uuid.UUID]bool, len(req.QuestionIDs))
		for _, id := range req.QuestionIDs {
			if !have[id] {
				return invalid("questionIds", "contains a question that is not in this draft")
			}
			if seen[id] {
				return invalid("questionIds", "lists a question more than once")
			}
			seen[id] = true
		}
		if err := s.repo.SetOrder(ctx, documentID, req.QuestionIDs, tx); err != nil {
			return err
		}
		qs, err = s.repo.ListDraft(ctx, documentID, tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Draft{Document: d, Questions: qs}, nil
}
