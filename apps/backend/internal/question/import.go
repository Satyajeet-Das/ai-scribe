package question

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
)

// ImportOption is one answer choice of an imported question.
type ImportOption struct {
	Key       string
	Text      string
	IsCorrect bool
}

// ImportItem is a reviewed question to add to an exam.
type ImportItem struct {
	// SourceDocumentID and SourceDraftQuestionID record where it came from. The
	// draft id is unique among questions, so importing the same draft twice
	// fails instead of duplicating the question.
	SourceDocumentID      uuid.UUID
	SourceDraftQuestionID uuid.UUID
	Text                  string
	Type                  Type
	Points                int
	Options               []ImportOption
}

// Validate applies the same rules as creating a question by hand.
func (it ImportItem) Validate() error {
	if utf8.RuneCountInString(strings.TrimSpace(it.Text)) < 2 {
		return ErrImportInvalid
	}
	switch it.Type {
	case TypeMCQ, TypeEssay, TypeVoice:
	default:
		return ErrInvalidQuestionType
	}
	if it.Points < 0 {
		return ErrImportInvalid
	}
	if len(it.Options) > 0 && it.Type != TypeMCQ {
		return ErrOptionsOnlyForMCQ
	}
	keys := map[string]bool{}
	correct := 0
	for _, o := range it.Options {
		key := strings.ToUpper(strings.TrimSpace(o.Key))
		if key == "" || utf8.RuneCountInString(key) > 10 || strings.TrimSpace(o.Text) == "" {
			return ErrImportInvalid
		}
		if keys[key] {
			return ErrOptionKeyDuplicate
		}
		keys[key] = true
		if o.IsCorrect {
			correct++
		}
	}
	if correct > 1 {
		return ErrMultipleCorrectOptions
	}
	return nil
}

// ImportQuestions adds reviewed questions to the end of an exam, numbered after
// the questions already there, using the caller's transaction. All of them are
// added or none are.
//
// It does not check who the caller is or what state the exam is in: that is the
// job of the module that holds the review, which must have locked the exam row
// in the same transaction so that publishing cannot happen in between.
func (s *questionService) ImportQuestions(ctx context.Context, tx database.DBTX, examID uuid.UUID, items []ImportItem) ([]Question, error) {
	for _, it := range items {
		if err := it.Validate(); err != nil {
			return nil, err
		}
	}
	maxNum, err := s.repo.GetMaxQuestionNumber(ctx, examID, tx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	created := make([]Question, 0, len(items))
	for i, it := range items {
		docID, draftID := it.SourceDocumentID, it.SourceDraftQuestionID
		q := &Question{
			Base: model.Base{
				BaseWithId:        model.BaseWithId{ID: uuid.New()},
				BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
				BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
			},
			ExamID:         examID,
			QuestionNumber: maxNum + i + 1,
			Text:           strings.TrimSpace(it.Text),
			Type:           it.Type,
			Points:         it.Points,
		}
		if docID != uuid.Nil {
			q.SourceDocumentID = &docID
		}
		if draftID != uuid.Nil {
			q.SourceDraftQuestionID = &draftID
		}
		if err := s.repo.Create(ctx, q, tx); err != nil {
			return nil, err
		}
		for j, o := range it.Options {
			opt := &QuestionOption{
				Base: model.Base{
					BaseWithId:        model.BaseWithId{ID: uuid.New()},
					BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
					BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
				},
				QuestionID:   q.ID,
				OptionKey:    strings.ToUpper(strings.TrimSpace(o.Key)),
				OptionText:   strings.TrimSpace(o.Text),
				DisplayOrder: j + 1,
				IsCorrect:    o.IsCorrect,
			}
			if err := s.repo.CreateOption(ctx, opt, tx); err != nil {
				return nil, err
			}
			q.Options = append(q.Options, *opt)
		}
		created = append(created, *q)
	}

	s.logger.Info().
		Str("event", "questions.imported").
		Str("exam_id", examID.String()).
		Int("count", len(created)).
		Msg("reviewed questions added to exam")
	return created, nil
}
