package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/logger"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/redis"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
)

func TestRedisCache_StateManagement(t *testing.T) {
	testDB, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	logSvc := logger.NewLoggerService(testDB.Config.Observability)
	l := logger.NewLoggerWithService(testDB.Config.Observability, logSvc)

	redisClient := redis.New(testDB.Config, &l, logSvc)
	defer redisClient.Close()

	cache := session.NewRedisCache(redisClient)

	sessID := uuid.New()
	sess := &session.Session{
		ExamID:            uuid.New(),
		StudentID:         uuid.New(),
		Status:            session.StatusInProgress,
		CurrentQuestionID: uuid.New(),
		LastActivityAt:    time.Now().UTC().Truncate(time.Millisecond),
	}
	sess.ID = sessID

	// 1. Store active session state
	err := cache.SetState(ctx, sessID, sess, time.Hour)
	require.NoError(t, err)

	// 2. Retrieve active session state
	retrieved, err := cache.GetState(ctx, sessID)
	require.NoError(t, err)
	assert.Equal(t, sess.ExamID, retrieved.ExamID)
	assert.Equal(t, sess.StudentID, retrieved.StudentID)
	assert.Equal(t, sess.Status, retrieved.Status)
	assert.Equal(t, sess.CurrentQuestionID, retrieved.CurrentQuestionID)
	assert.Equal(t, sess.LastActivityAt, retrieved.LastActivityAt)

	// 3. Update state
	sess.CurrentQuestionID = uuid.New()
	err = cache.SetState(ctx, sessID, sess, time.Hour)
	require.NoError(t, err)

	retrieved, err = cache.GetState(ctx, sessID)
	require.NoError(t, err)
	assert.Equal(t, sess.CurrentQuestionID, retrieved.CurrentQuestionID)

	// 4. Redis delete/miss
	err = cache.DeleteState(ctx, sessID)
	require.NoError(t, err)

	_, err = cache.GetState(ctx, sessID)
	assert.ErrorIs(t, err, session.ErrCacheMiss)

	// 5. Redis expiration
	sessID2 := uuid.New()
	err = cache.SetState(ctx, sessID2, sess, 1*time.Millisecond) // expire quickly
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	_, err = cache.GetState(ctx, sessID2)
	assert.ErrorIs(t, err, session.ErrCacheMiss)
}

func TestRedisCache_AcquireLock(t *testing.T) {
	testDB, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	logSvc := logger.NewLoggerService(testDB.Config.Observability)
	l := logger.NewLoggerWithService(testDB.Config.Observability, logSvc)

	redisClient := redis.New(testDB.Config, &l, logSvc)
	defer redisClient.Close()

	cache := session.NewRedisCache(redisClient)

	sessID := uuid.New()

	// Acquire lock initially
	unlock, err := cache.AcquireLock(ctx, sessID, 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, unlock)

	// Try acquiring again immediately, should timeout and fail
	ctxTimeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	_, err = cache.AcquireLock(ctxTimeout, sessID, 5*time.Second)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	// Release lock
	err = unlock(ctx)
	require.NoError(t, err)

	// Should be able to acquire again
	unlock2, err := cache.AcquireLock(ctx, sessID, 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, unlock2)

	_ = unlock2(ctx)
}
