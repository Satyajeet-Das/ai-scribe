package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type faultMockCache struct {
	failAcquireLock bool
	failGetState    bool
}

func (m *faultMockCache) AcquireLock(ctx context.Context, sessionID uuid.UUID, ttl time.Duration) (func(context.Context) error, error) {
	if m.failAcquireLock {
		return nil, errors.New("redis connection refused")
	}
	return func(context.Context) error { return nil }, nil
}

func (m *faultMockCache) SetState(ctx context.Context, sessionID uuid.UUID, session *Session, ttl time.Duration) error {
	return nil
}

func (m *faultMockCache) GetState(ctx context.Context, sessionID uuid.UUID) (*Session, error) {
	if m.failGetState {
		return nil, errors.New("redis connection reset by peer")
	}
	return nil, ErrCacheMiss
}

func (m *faultMockCache) DeleteState(ctx context.Context, sessionID uuid.UUID) error {
	return nil
}

type faultMockRepo struct {
	*mockSessionRepo
	failGetByID bool
}

func (m *faultMockRepo) GetByID(ctx context.Context, id uuid.UUID) (*Session, error) {
	if m.failGetByID {
		return nil, errors.New("database connection refused")
	}
	return m.mockSessionRepo.GetByID(ctx, id)
}

func TestSessionService_FailureScenarios(t *testing.T) {
	baseRepo := newMockSessionRepo()
	repo := &faultMockRepo{mockSessionRepo: baseRepo}
	cache := &faultMockCache{}
	logger := zerolog.Nop()

	examReader := &mockExamReader{
		exams: map[uuid.UUID]*exam.Exam{
			uuid.Nil: {DurationMins: 60},
		},
	}
	questionReader := &mockQuestionReader{}
	
	svc := NewService(repo, nil, examReader, questionReader, cache, &mockTxManager{}, &mockTaskEnqueuer{}, &logger)
	ctx := context.Background()

	sessID := uuid.New()
	studentID := uuid.New()

	baseRepo.sessions[sessID] = &Session{
		Base: model.Base{BaseWithId: model.BaseWithId{ID: sessID}},
		StudentID: studentID,
		Status: StatusInProgress,
	}

	t.Run("Redis Unavailable on AcquireLock", func(t *testing.T) {
		cache.failAcquireLock = true
		cache.failGetState = false
		repo.failGetByID = false

		_, _, err := svc.LockAndValidate(ctx, sessID, EventRecordAnswer, studentID)
		require.Error(t, err)
		assert.Equal(t, "redis connection refused", err.Error())
	})

	t.Run("Redis Unavailable on GetState", func(t *testing.T) {
		cache.failAcquireLock = false
		cache.failGetState = true
		repo.failGetByID = false

		_, _, err := svc.LockAndValidate(ctx, sessID, EventRecordAnswer, studentID)
		require.Error(t, err)
		assert.Equal(t, "redis connection reset by peer", err.Error())
	})

	t.Run("PostgreSQL Unavailable on GetByID Fallback", func(t *testing.T) {
		cache.failAcquireLock = false
		cache.failGetState = false // Will return ErrCacheMiss, falling back to DB
		repo.failGetByID = true

		_, _, err := svc.LockAndValidate(ctx, sessID, EventRecordAnswer, studentID)
		require.Error(t, err)
		assert.Equal(t, "database connection refused", err.Error())
	})
}
