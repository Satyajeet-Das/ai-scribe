package question

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
)

type mockQuestionRepo struct {
	questions map[uuid.UUID]*Question
	options   map[uuid.UUID]*QuestionOption
}

func newMockQuestionRepo() *mockQuestionRepo {
	return &mockQuestionRepo{
		questions: make(map[uuid.UUID]*Question),
		options:   make(map[uuid.UUID]*QuestionOption),
	}
}

func (m *mockQuestionRepo) GetByID(ctx context.Context, id uuid.UUID) (*Question, error) {
	if q, ok := m.questions[id]; ok {
		copy := *q
		return &copy, nil
	}
	return nil, nil
}

func (m *mockQuestionRepo) GetByIDWithOptions(ctx context.Context, id uuid.UUID) (*Question, error) {
	q, err := m.GetByID(ctx, id)
	if err != nil || q == nil {
		return q, err
	}
	opts, _ := m.ListOptionsByQuestionID(ctx, id)
	q.Options = opts
	return q, nil
}

func (m *mockQuestionRepo) ListByExamID(ctx context.Context, examID uuid.UUID) ([]Question, error) {
	res := make([]Question, 0)
	for _, q := range m.questions {
		if q.ExamID == examID {
			res = append(res, *q)
		}
	}
	return res, nil
}

func (m *mockQuestionRepo) Create(ctx context.Context, q *Question) error {
	m.questions[q.ID] = q
	return nil
}

func (m *mockQuestionRepo) Update(ctx context.Context, q *Question) error {
	m.questions[q.ID] = q
	return nil
}

func (m *mockQuestionRepo) Delete(ctx context.Context, id uuid.UUID) error {
	delete(m.questions, id)
	return nil
}

func (m *mockQuestionRepo) GetOptionByID(ctx context.Context, id uuid.UUID) (*QuestionOption, error) {
	if opt, ok := m.options[id]; ok {
		copy := *opt
		return &copy, nil
	}
	return nil, nil
}

func (m *mockQuestionRepo) ListOptionsByQuestionID(ctx context.Context, questionID uuid.UUID) ([]QuestionOption, error) {
	res := make([]QuestionOption, 0)
	for _, opt := range m.options {
		if opt.QuestionID == questionID {
			res = append(res, *opt)
		}
	}
	return res, nil
}

func (m *mockQuestionRepo) CreateOption(ctx context.Context, opt *QuestionOption) error {
	m.options[opt.ID] = opt
	return nil
}

func (m *mockQuestionRepo) DeleteOption(ctx context.Context, id uuid.UUID) error {
	delete(m.options, id)
	return nil
}

type mockExamReader struct {
	exams map[uuid.UUID]*exam.Exam
}

func (m *mockExamReader) GetExam(ctx context.Context, id uuid.UUID) (*exam.Exam, error) {
	if e, ok := m.exams[id]; ok {
		return e, nil
	}
	return nil, exam.ErrExamNotFound
}

func TestQuestionService_CreateQuestion(t *testing.T) {
	repo := newMockQuestionRepo()
	logger := zerolog.Nop()
	creatorID := uuid.New()
	examID := uuid.New()

	examReader := &mockExamReader{
		exams: map[uuid.UUID]*exam.Exam{
			examID: {
				Title:     "Physics Exam",
				Status:    exam.StatusDraft,
				CreatedBy: creatorID,
			},
		},
	}
	svc := NewService(repo, examReader, &logger)
	ctx := context.Background()

	// 1. Success on DRAFT exam
	req := CreateQuestionRequest{
		QuestionNumber: 1,
		Text:           "What is Newton's second law?",
		Type:           TypeEssay,
		Points:         5,
	}
	created, err := svc.CreateQuestion(ctx, examID, req, creatorID)
	require.NoError(t, err)
	assert.Equal(t, 1, created.QuestionNumber)
	assert.Equal(t, TypeEssay, created.Type)

	// 2. Reject if unauthorized creator
	otherUser := uuid.New()
	_, err = svc.CreateQuestion(ctx, examID, req, otherUser)
	assert.ErrorIs(t, err, ErrUnauthorizedCreator)

	// 3. Reject if exam is PUBLISHED
	examReader.exams[examID].Status = exam.StatusPublished
	_, err = svc.CreateQuestion(ctx, examID, req, creatorID)
	assert.ErrorIs(t, err, ErrExamNotDraft)

	// 4. Reject if exam is ARCHIVED
	examReader.exams[examID].Status = exam.StatusArchived
	_, err = svc.CreateQuestion(ctx, examID, req, creatorID)
	assert.ErrorIs(t, err, ErrExamArchived)
}

func TestQuestionService_Options(t *testing.T) {
	repo := newMockQuestionRepo()
	logger := zerolog.Nop()
	creatorID := uuid.New()
	examID := uuid.New()
	mcqQuestionID := uuid.New()
	essayQuestionID := uuid.New()

	examReader := &mockExamReader{
		exams: map[uuid.UUID]*exam.Exam{
			examID: {
				Title:     "Biology Exam",
				Status:    exam.StatusDraft,
				CreatedBy: creatorID,
			},
		},
	}

	repo.questions[mcqQuestionID] = &Question{
		ExamID:         examID,
		QuestionNumber: 1,
		Text:           "What is the powerhouse of the cell?",
		Type:           TypeMCQ,
	}
	repo.questions[essayQuestionID] = &Question{
		ExamID:         examID,
		QuestionNumber: 2,
		Text:           "Describe photosynthesis.",
		Type:           TypeEssay,
	}

	svc := NewService(repo, examReader, &logger)
	ctx := context.Background()

	// 1. Adding option to MCQ succeeds
	optA, err := svc.CreateOption(ctx, mcqQuestionID, CreateOptionRequest{
		OptionKey:    "A",
		OptionText:   "Mitochondria",
		DisplayOrder: 1,
		IsCorrect:    true,
	}, creatorID)
	require.NoError(t, err)
	assert.Equal(t, "A", optA.OptionKey)
	assert.True(t, optA.IsCorrect)

	// 2. Adding duplicate option key fails
	_, err = svc.CreateOption(ctx, mcqQuestionID, CreateOptionRequest{
		OptionKey:    "a", // case-insensitive check
		OptionText:   "Nucleus",
		DisplayOrder: 2,
		IsCorrect:    false,
	}, creatorID)
	assert.ErrorIs(t, err, ErrOptionKeyDuplicate)

	// 3. Adding option to non-MCQ fails
	_, err = svc.CreateOption(ctx, essayQuestionID, CreateOptionRequest{
		OptionKey:  "A",
		OptionText: "Invalid option",
	}, creatorID)
	assert.ErrorIs(t, err, ErrOptionsOnlyForMCQ)
}
