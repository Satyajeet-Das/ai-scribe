package exam

import (
	"context"
	"testing"
	"time"

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
		// Return copy
		copy := *e
		return &copy, nil
	}
	return nil, nil
}

func (m *mockExamRepo) List(ctx context.Context, limit, offset int, status *Status) ([]Exam, int, error) {
	res := make([]Exam, 0, len(m.exams))
	for _, e := range m.exams {
		if status == nil || e.Status == *status {
			res = append(res, *e)
		}
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

func (m *mockExamRepo) Publish(ctx context.Context, id uuid.UUID, publishedAt time.Time) error {
	if e, ok := m.exams[id]; ok {
		if e.Status != StatusDraft {
			return ErrInvalidExamState
		}
		e.Status = StatusPublished
		e.PublishedAt = &publishedAt
		return nil
	}
	return ErrExamNotFound
}

func (m *mockExamRepo) Archive(ctx context.Context, id uuid.UUID) error {
	if e, ok := m.exams[id]; ok {
		e.Status = StatusArchived
		return nil
	}
	return ErrExamNotFound
}

func TestExamService_CreateAndGet(t *testing.T) {
	repo := newMockExamRepo()
	logger := zerolog.Nop()
	svc := NewService(repo, &logger)
	ctx := context.Background()
	creatorID := uuid.New()

	req := CreateExamRequest{
		Title:        "Data Structures Exam",
		Subject:      "Computer Science",
		Description:  "Midterm covering trees and graphs",
		DurationMins: 90,
	}

	created, err := svc.CreateExam(ctx, req, creatorID)
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "Data Structures Exam", created.Title)
	assert.Equal(t, StatusDraft, created.Status)
	assert.Equal(t, creatorID, created.CreatedBy)

	// Fetch existing
	fetched, err := svc.GetExam(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, fetched.ID)

	// Fetch missing
	_, err = svc.GetExam(ctx, uuid.New())
	assert.ErrorIs(t, err, ErrExamNotFound)
}

func TestExamService_Update(t *testing.T) {
	repo := newMockExamRepo()
	logger := zerolog.Nop()
	svc := NewService(repo, &logger)
	ctx := context.Background()
	creatorID := uuid.New()
	otherUser := uuid.New()

	exam, err := svc.CreateExam(ctx, CreateExamRequest{
		Title:        "Calculus I",
		Subject:      "Mathematics",
		DurationMins: 60,
	}, creatorID)
	require.NoError(t, err)

	// Update by creator in DRAFT status succeeds
	newTitle := "Calculus I (Advanced)"
	updated, err := svc.UpdateExam(ctx, exam.ID, UpdateExamRequest{Title: &newTitle}, creatorID)
	require.NoError(t, err)
	assert.Equal(t, newTitle, updated.Title)

	// Update by unauthorized user fails
	_, err = svc.UpdateExam(ctx, exam.ID, UpdateExamRequest{Title: &newTitle}, otherUser)
	assert.ErrorIs(t, err, ErrUnauthorizedCreator)

	// Publish the exam
	_, err = svc.PublishExam(ctx, exam.ID, creatorID)
	require.NoError(t, err)

	// Update after published fails (draft immutability)
	_, err = svc.UpdateExam(ctx, exam.ID, UpdateExamRequest{Title: &newTitle}, creatorID)
	assert.ErrorIs(t, err, ErrExamNotDraft)
}

func TestExamService_PublishAndArchive(t *testing.T) {
	repo := newMockExamRepo()
	logger := zerolog.Nop()
	svc := NewService(repo, &logger)
	ctx := context.Background()
	creatorID := uuid.New()

	exam, err := svc.CreateExam(ctx, CreateExamRequest{
		Title:        "Chemistry Final",
		Subject:      "Chemistry",
		DurationMins: 120,
	}, creatorID)
	require.NoError(t, err)

	// Publish succeeds
	published, err := svc.PublishExam(ctx, exam.ID, creatorID)
	require.NoError(t, err)
	assert.Equal(t, StatusPublished, published.Status)
	assert.NotNil(t, published.PublishedAt)

	// Publish already published fails
	_, err = svc.PublishExam(ctx, exam.ID, creatorID)
	assert.ErrorIs(t, err, ErrExamAlreadyPublished)

	// Archive succeeds
	archived, err := svc.ArchiveExam(ctx, exam.ID, creatorID)
	require.NoError(t, err)
	assert.Equal(t, StatusArchived, archived.Status)

	// Archive already archived fails
	_, err = svc.ArchiveExam(ctx, exam.ID, creatorID)
	assert.ErrorIs(t, err, ErrExamAlreadyArchived)
}

func TestExamDTO_Validation(t *testing.T) {
	validReq := CreateExamRequest{
		Title:        "Operating Systems",
		Subject:      "CS",
		DurationMins: 60,
	}
	assert.NoError(t, validReq.Validate())

	invalidReq := CreateExamRequest{
		Title:        "",
		Subject:      "",
		DurationMins: 0,
	}
	assert.Error(t, invalidReq.Validate())
}
