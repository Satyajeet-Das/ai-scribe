package session

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/model"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
)

func setupNavTest() (*mockSessionRepo, *mockExamReader, *mockQuestionReader, *mockCache, Service, uuid.UUID, uuid.UUID) {
	repo := newMockSessionRepo()
	logger := zerolog.Nop()
	studentID := uuid.New()
	examID := uuid.New()
	sessID := uuid.New()

	examReader := &mockExamReader{
		exams: map[uuid.UUID]*exam.Exam{
			examID: {
				Title:        "Math 101",
				Status:       exam.StatusPublished,
				DurationMins: 60,
			},
		},
	}

	q1 := uuid.New()
	q2 := uuid.New()
	q3 := uuid.New()

	questionReader := &mockQuestionReader{
		questions: map[uuid.UUID][]question.Question{
			examID: {
				{Base: model.Base{BaseWithId: model.BaseWithId{ID: q1}}},
				{Base: model.Base{BaseWithId: model.BaseWithId{ID: q2}}},
				{Base: model.Base{BaseWithId: model.BaseWithId{ID: q3}}},
			},
		},
	}

	sess := &Session{
		Base: model.Base{BaseWithId: model.BaseWithId{ID: sessID}},
		ExamID:     examID,
		StudentID:  studentID,
		Status:     StatusInProgress,
		StartedAt:  time.Now().UTC(),
		CurrentQuestionID: q1,
	}
	repo.sessions[sessID] = sess
	cache := newMockCache()
	_ = cache.SetState(context.Background(), sessID, sess, time.Hour)

	svc := NewService(repo, nil, examReader, questionReader, cache, &mockTxManager{}, &mockTaskEnqueuer{}, &logger)
	return repo, examReader, questionReader, cache, svc, sessID, studentID
}

func TestSessionService_NextQuestion(t *testing.T) {
	_, _, questionReader, _, svc, sessID, studentID := setupNavTest()
	ctx := context.Background()
	_, _ = questionReader.ListQuestionsByExam(ctx, uuid.Nil) // mock returns same anyway but examID is used in setup

	// Assume we have the mock correctly set up
	// Currently at q1
	sess, err := svc.NextQuestion(ctx, sessID, studentID)
	require.NoError(t, err)
	assert.Equal(t, sess.CurrentQuestionID, questionReader.questions[sess.ExamID][1].ID)
	assert.NotZero(t, sess.LastActivityAt)

	// Currently at q2, go to q3
	sess, err = svc.NextQuestion(ctx, sessID, studentID)
	require.NoError(t, err)
	assert.Equal(t, sess.CurrentQuestionID, questionReader.questions[sess.ExamID][2].ID)

	// Currently at q3 (last), try to go next should fail
	_, err = svc.NextQuestion(ctx, sessID, studentID)
	assert.ErrorIs(t, err, ErrNoNextQuestion)
}

func TestSessionService_PreviousQuestion(t *testing.T) {
	_, _, questionReader, cache, svc, sessID, studentID := setupNavTest()
	ctx := context.Background()

	// Manually set to q2
	sess, _ := cache.GetState(ctx, sessID)
	sess.CurrentQuestionID = questionReader.questions[sess.ExamID][1].ID
	_ = cache.SetState(ctx, sessID, sess, time.Hour)

	// From q2, go previous to q1
	sess, err := svc.PreviousQuestion(ctx, sessID, studentID)
	require.NoError(t, err)
	assert.Equal(t, sess.CurrentQuestionID, questionReader.questions[sess.ExamID][0].ID)

	// From q1 (first), try to go previous should fail
	_, err = svc.PreviousQuestion(ctx, sessID, studentID)
	assert.ErrorIs(t, err, ErrNoPreviousQuestion)
}

func TestSessionService_ExpireSession(t *testing.T) {
	_, _, _, cache, svc, sessID, _ := setupNavTest()
	ctx := context.Background()

	err := svc.ExpireSession(ctx, sessID)
	require.NoError(t, err)

	// Should be deleted from cache
	_, err = cache.GetState(ctx, sessID)
	assert.ErrorIs(t, err, ErrCacheMiss)

	// Should fail if we try to expire again (or return nil since it's already expired)
	err = svc.ExpireSession(ctx, sessID)
	require.NoError(t, err) // It is idempotent and ignores already expired/submitted
}

func TestSessionService_LockAndValidate_PassiveExpiration(t *testing.T) {
	repo, examReader, _, cache, svc, sessID, studentID := setupNavTest()
	ctx := context.Background()

	sess, _ := cache.GetState(ctx, sessID)
	
	// Fast forward time to make it expired passively
	sess.StartedAt = time.Now().UTC().Add(-2 * time.Hour)
	examReader.exams[sess.ExamID].DurationMins = 60
	_ = cache.SetState(ctx, sessID, sess, time.Hour)
	repo.sessions[sessID].StartedAt = sess.StartedAt // Sync repo

	// Try to record answer, LockAndValidate should passively expire it and return ErrSessionExpired
	_, _, err := svc.LockAndValidate(ctx, sessID, EventRecordAnswer, studentID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSessionExpired)
	
	// Should be deleted from cache due to passive expiration
	_, err = cache.GetState(ctx, sessID)
	assert.ErrorIs(t, err, ErrCacheMiss)
}

func TestSessionService_GetCurrentQuestion(t *testing.T) {
	repo, _, questionReader, _, svc, sessID, studentID := setupNavTest()
	ctx := context.Background()

	// Add options to q1 with IsCorrect set
	examID := repo.sessions[sessID].ExamID
	opt1ID := uuid.New()
	opt2ID := uuid.New()
	questionReader.questions[examID][0].Options = []question.QuestionOption{
		{
			Base:         model.Base{BaseWithId: model.BaseWithId{ID: opt1ID}},
			QuestionID:   questionReader.questions[examID][0].ID,
			OptionKey:    "A",
			OptionText:   "Option 1",
			DisplayOrder: 1,
			IsCorrect:    true, // Must NOT leak to student
		},
		{
			Base:         model.Base{BaseWithId: model.BaseWithId{ID: opt2ID}},
			QuestionID:   questionReader.questions[examID][0].ID,
			OptionKey:    "B",
			OptionText:   "Option 2",
			DisplayOrder: 2,
			IsCorrect:    false,
		},
	}

	// 1. Get current question (initially at q1)
	qResp, err := svc.GetCurrentQuestion(ctx, sessID, studentID)
	require.NoError(t, err)
	assert.Equal(t, questionReader.questions[examID][0].ID, qResp.ID)
	assert.True(t, qResp.IsFirst)
	assert.False(t, qResp.IsLast)
	assert.Equal(t, 3, qResp.TotalQuestions)
	assert.Equal(t, 1, qResp.CurrentIndex)
	require.Len(t, qResp.Options, 2)
	assert.Equal(t, "A", qResp.Options[0].OptionKey)
	assert.Equal(t, "Option 1", qResp.Options[0].OptionText)

	// 2. Unauthorized student cannot view question
	_, err = svc.GetCurrentQuestion(ctx, sessID, uuid.New())
	assert.ErrorIs(t, err, ErrUnauthorizedStudent)

	// 3. Navigate to last question (q3)
	_, err = svc.NextQuestion(ctx, sessID, studentID)
	require.NoError(t, err)
	_, err = svc.NextQuestion(ctx, sessID, studentID)
	require.NoError(t, err)

	qResp3, err := svc.GetCurrentQuestion(ctx, sessID, studentID)
	require.NoError(t, err)
	assert.Equal(t, questionReader.questions[examID][2].ID, qResp3.ID)
	assert.False(t, qResp3.IsFirst)
	assert.True(t, qResp3.IsLast)
	assert.Equal(t, 3, qResp3.CurrentIndex)

	// 4. Submit session - GetCurrentQuestion should be rejected
	_, err = svc.SubmitSession(ctx, sessID, studentID)
	require.NoError(t, err)

	_, err = svc.GetCurrentQuestion(ctx, sessID, studentID)
	assert.ErrorIs(t, err, ErrSessionAlreadySubmitted)
}
