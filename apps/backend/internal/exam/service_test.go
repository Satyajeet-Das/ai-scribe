package exam

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockExamRepo struct {
	exams map[uuid.UUID]*Exam
}

func newMockExamRepo() *mockExamRepo {
	return &mockExamRepo{
		exams: make(map[uuid.UUID]*Exam),
	}
}

func (m *mockExamRepo) GetByID(ctx context.Context, id uuid.UUID) (*Exam, error) {
	if e, ok := m.exams[id]; ok {
		return e, nil
	}
	return nil, nil
}

func (m *mockExamRepo) List(ctx context.Context, limit, offset int) ([]Exam, int, error) {
	res := make([]Exam, 0, len(m.exams))
	for _, e := range m.exams {
		res = append(res, *e)
	}
	return res, len(res), nil
}

func (m *mockExamRepo) Create(ctx context.Context, e *Exam) error {
	m.exams[e.ID] = e
	return nil
}

func (m *mockExamRepo) Update(ctx context.Context, e *Exam) error {
	m.exams[e.ID] = e
	return nil
}

func TestExamService_CreateAndGetExam(t *testing.T) {
	repo := newMockExamRepo()
	logger := zerolog.Nop()
	svc := NewService(repo, &logger)

	ctx := context.Background()
	req := CreateExamRequest{
		Title:           "Biology Midterm Exam",
		Description:     "Comprehensive exam on cellular biology",
		Subject:         "Biology",
		DurationMinutes: 60,
	}
	creatorID := uuid.New()

	created, err := svc.CreateExam(ctx, req, creatorID)
	require.NoError(t, err)
	require.NotNil(t, created)

	assert.NotEqual(t, uuid.Nil, created.ID)
	assert.Equal(t, "Biology Midterm Exam", created.Title)
	assert.Equal(t, StatusDraft, created.Status)
	assert.Equal(t, creatorID, created.CreatedBy)

	// Fetch existing exam
	fetched, err := svc.GetExam(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, created.Title, fetched.Title)

	// Fetch non-existent exam
	_, err = svc.GetExam(ctx, uuid.New())
	assert.ErrorIs(t, err, ErrExamNotFound)
}

func TestExamDTO_Validation(t *testing.T) {
	validReq := CreateExamRequest{
		Title:           "Valid Title",
		Subject:         "Physics",
		DurationMinutes: 90,
	}
	assert.NoError(t, validReq.Validate())

	invalidReq := CreateExamRequest{
		Title:           "", // Missing title
		Subject:         "Physics",
		DurationMinutes: 0, // Too low
	}
	assert.Error(t, invalidReq.Validate())
}
