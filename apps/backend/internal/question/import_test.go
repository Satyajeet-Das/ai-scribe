package question

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newImportService() (*mockQuestionRepo, Service) {
	repo := newMockQuestionRepo()
	logger := zerolog.Nop()
	return repo, NewService(repo, nil, nil, &logger)
}

func TestImportQuestions_AppendsAfterExistingQuestionsWithProvenance(t *testing.T) {
	repo, svc := newImportService()
	examID, docID := uuid.New(), uuid.New()
	require.NoError(t, repo.Create(context.Background(), &Question{ExamID: examID, QuestionNumber: 4, Text: "existing", Type: TypeEssay}))

	d1, d2 := uuid.New(), uuid.New()
	got, err := svc.ImportQuestions(context.Background(), nil, examID, []ImportItem{
		{SourceDocumentID: docID, SourceDraftQuestionID: d1, Text: "What is the SI unit of force?", Type: TypeMCQ, Points: 2,
			Options: []ImportOption{{Key: "a", Text: "Newton"}, {Key: "b", Text: "Joule", IsCorrect: true}}},
		{SourceDocumentID: docID, SourceDraftQuestionID: d2, Text: "Explain Newton's first law.", Type: TypeEssay, Points: 5},
	})
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, 5, got[0].QuestionNumber, "numbered after the exam's last question")
	assert.Equal(t, 6, got[1].QuestionNumber)
	require.NotNil(t, got[0].SourceDocumentID)
	assert.Equal(t, docID, *got[0].SourceDocumentID)
	assert.Equal(t, d1, *got[0].SourceDraftQuestionID)

	require.Len(t, got[0].Options, 2)
	assert.Equal(t, "A", got[0].Options[0].OptionKey, "keys are stored in upper case, as for hand-made questions")
	assert.Equal(t, 1, got[0].Options[0].DisplayOrder)
	assert.True(t, got[0].Options[1].IsCorrect, "a teacher-set correct option is kept")
	assert.Empty(t, got[1].Options)
}

func TestImportQuestions_RejectsInvalidItemsBeforeWritingAnything(t *testing.T) {
	repo, svc := newImportService()
	examID := uuid.New()
	good := ImportItem{Text: "A fine question", Type: TypeEssay}

	cases := map[string]struct {
		item ImportItem
		want error
	}{
		"text too short":      {ImportItem{Text: " x ", Type: TypeEssay}, ErrImportInvalid},
		"unknown type":        {ImportItem{Text: "A fine question", Type: "TRUE_FALSE"}, ErrInvalidQuestionType},
		"negative points":     {ImportItem{Text: "A fine question", Type: TypeEssay, Points: -1}, ErrImportInvalid},
		"options on an essay": {ImportItem{Text: "A fine question", Type: TypeEssay, Options: []ImportOption{{Key: "A", Text: "x"}}}, ErrOptionsOnlyForMCQ},
		"duplicate keys": {ImportItem{Text: "A fine question", Type: TypeMCQ,
			Options: []ImportOption{{Key: "a", Text: "x"}, {Key: "A", Text: "y"}}}, ErrOptionKeyDuplicate},
		"two correct options": {ImportItem{Text: "A fine question", Type: TypeMCQ,
			Options: []ImportOption{{Key: "A", Text: "x", IsCorrect: true}, {Key: "B", Text: "y", IsCorrect: true}}}, ErrMultipleCorrectOptions},
		"key too long":      {ImportItem{Text: "A fine question", Type: TypeMCQ, Options: []ImportOption{{Key: "ABCDEFGHIJK", Text: "x"}}}, ErrImportInvalid},
		"empty option text": {ImportItem{Text: "A fine question", Type: TypeMCQ, Options: []ImportOption{{Key: "A", Text: "  "}}}, ErrImportInvalid},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.ImportQuestions(context.Background(), nil, examID, []ImportItem{good, tc.item})
			assert.ErrorIs(t, err, tc.want)
			assert.Empty(t, repo.questions, "the valid first item is not written when a later one is not valid")
		})
	}
}
