package session

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/assignment"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/model"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
)

type mockSessionRepo struct {
	sessions map[uuid.UUID]*Session
}

func newMockSessionRepo() *mockSessionRepo {
	return &mockSessionRepo{
		sessions: make(map[uuid.UUID]*Session),
	}
}

func (m *mockSessionRepo) GetByID(ctx context.Context, id uuid.UUID) (*Session, error) {
	if s, ok := m.sessions[id]; ok {
		copy := *s
		return &copy, nil
	}
	return nil, nil
}

func (m *mockSessionRepo) GetActiveByAssignmentID(ctx context.Context, assignmentID uuid.UUID) (*Session, error) {
	for _, s := range m.sessions {
		if s.AssignmentID == assignmentID && s.Status == StatusInProgress {
			copy := *s
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *mockSessionRepo) ListByStudentID(ctx context.Context, studentID uuid.UUID, limit, offset int) ([]Session, int, error) {
	res := make([]Session, 0)
	for _, s := range m.sessions {
		if s.StudentID == studentID {
			res = append(res, *s)
		}
	}
	return res, len(res), nil
}

func (m *mockSessionRepo) Create(ctx context.Context, s *Session) error {
	m.sessions[s.ID] = s
	return nil
}

func (m *mockSessionRepo) Submit(ctx context.Context, id uuid.UUID, submittedAt time.Time) error {
	if s, ok := m.sessions[id]; ok {
		s.Status = StatusSubmitted
		s.SubmittedAt = &submittedAt
		return nil
	}
	return ErrSessionNotFound
}

func (m *mockSessionRepo) Expire(ctx context.Context, id uuid.UUID) error {
	if s, ok := m.sessions[id]; ok {
		s.Status = StatusExpired
		return nil
	}
	return ErrSessionNotFound
}

type mockAssignmentReader struct {
	assignments map[uuid.UUID]*assignment.Assignment
}

func (m *mockAssignmentReader) GetAssignment(ctx context.Context, id uuid.UUID) (*assignment.Assignment, error) {
	if a, ok := m.assignments[id]; ok {
		return a, nil
	}
	return nil, assignment.ErrAssignmentNotFound
}

func (m *mockAssignmentReader) GetActiveAssignment(ctx context.Context, examID, studentID uuid.UUID) (*assignment.Assignment, error) {
	for _, a := range m.assignments {
		if a.ExamID == examID && a.StudentID == studentID && a.Status == assignment.StatusAssigned {
			return a, nil
		}
	}
	return nil, nil
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

type mockQuestionReader struct {
	questions map[uuid.UUID][]question.Question
}

func (m *mockQuestionReader) ListQuestionsByExam(ctx context.Context, examID uuid.UUID) ([]question.Question, error) {
	if q, ok := m.questions[examID]; ok {
		return q, nil
	}
	return nil, nil
}

type mockCache struct {
	state map[uuid.UUID]*Session
}

func newMockCache() *mockCache {
	return &mockCache{state: make(map[uuid.UUID]*Session)}
}

func (m *mockCache) AcquireLock(ctx context.Context, sessionID uuid.UUID, ttl time.Duration) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}

func (m *mockCache) SetState(ctx context.Context, sessionID uuid.UUID, session *Session, ttl time.Duration) error {
	m.state[sessionID] = session
	return nil
}

func (m *mockCache) GetState(ctx context.Context, sessionID uuid.UUID) (*Session, error) {
	if s, ok := m.state[sessionID]; ok {
		return s, nil
	}
	return nil, ErrCacheMiss
}

func (m *mockCache) DeleteState(ctx context.Context, sessionID uuid.UUID) error {
	delete(m.state, sessionID)
	return nil
}

type mockTxManager struct{}

func (m *mockTxManager) WithTx(ctx context.Context, fn func(tx database.DBTX) error) error {
	return fn(nil)
}

type mockTaskEnqueuer struct{}

func (m *mockTaskEnqueuer) EnqueueExpireSessionTask(sessionID uuid.UUID, delay time.Duration) error {
	return nil
}

func TestSessionService_StartSession(t *testing.T) {
	repo := newMockSessionRepo()
	logger := zerolog.Nop()
	studentID := uuid.New()
	examID := uuid.New()
	assignmentID := uuid.New()

	asgnReader := &mockAssignmentReader{
		assignments: map[uuid.UUID]*assignment.Assignment{
			assignmentID: {
				ExamID:    examID,
				StudentID: studentID,
				Status:    assignment.StatusAssigned,
			},
		},
	}
	examReader := &mockExamReader{
		exams: map[uuid.UUID]*exam.Exam{
			examID: {
				Title:        "Biology 101",
				Status:       exam.StatusPublished,
				DurationMins: 60,
			},
		},
	}

	questionReader := &mockQuestionReader{
		questions: map[uuid.UUID][]question.Question{
			examID: {
				{Base: model.Base{BaseWithId: model.BaseWithId{ID: uuid.New()}}},
			},
		},
	}

	svc := NewService(repo, asgnReader, examReader, questionReader, newMockCache(), &mockTxManager{}, &mockTaskEnqueuer{}, &logger)
	ctx := context.Background()

	// 1. Starting valid session succeeds
	req := StartSessionRequest{AssignmentID: &assignmentID}
	sess, duration, err := svc.StartSession(ctx, req, studentID)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, sess.Status)
	assert.Equal(t, 60, duration)
	assert.Equal(t, studentID, sess.StudentID)

	// 2. Active session already in progress returns existing session for seamless rejoin
	rejoinedSess, rejoinedDuration, err := svc.StartSession(ctx, req, studentID)
	require.NoError(t, err)
	assert.Equal(t, sess.ID, rejoinedSess.ID)
	assert.Equal(t, 60, rejoinedDuration)
	assert.Equal(t, StatusInProgress, rejoinedSess.Status)

	// 2b. If active session time has elapsed beyond duration, rejoining auto-expires and returns error
	sess.StartedAt = time.Now().UTC().Add(-65 * time.Minute)
	_, _, err = svc.StartSession(ctx, req, studentID)
	assert.ErrorIs(t, err, ErrSessionExpired)
	assert.Equal(t, StatusExpired, sess.Status)

	// Reset session status for subsequent tests
	sess.Status = StatusInProgress
	sess.StartedAt = time.Now().UTC()

	// 3. Unauthorized student fails
	otherStudent := uuid.New()
	asgn2 := uuid.New()
	asgnReader.assignments[asgn2] = &assignment.Assignment{
		ExamID:    examID,
		StudentID: studentID,
		Status:    assignment.StatusAssigned,
	}
	_, _, err = svc.StartSession(ctx, StartSessionRequest{AssignmentID: &asgn2}, otherStudent)
	assert.ErrorIs(t, err, ErrUnauthorizedStudent)

	// 4. Revoked assignment fails
	asgnRevoked := uuid.New()
	asgnReader.assignments[asgnRevoked] = &assignment.Assignment{
		ExamID:    examID,
		StudentID: studentID,
		Status:    assignment.StatusRevoked,
	}
	_, _, err = svc.StartSession(ctx, StartSessionRequest{AssignmentID: &asgnRevoked}, studentID)
	assert.ErrorIs(t, err, ErrAssignmentRevoked)

	// 5. Starting session by ExamID succeeds for assigned student
	examID2 := uuid.New()
	asgn3 := uuid.New()
	asgnReader.assignments[asgn3] = &assignment.Assignment{
		Base:      model.Base{BaseWithId: model.BaseWithId{ID: asgn3}},
		ExamID:    examID2,
		StudentID: studentID,
		Status:    assignment.StatusAssigned,
	}
	examReader.exams[examID2] = &exam.Exam{
		Base:         model.Base{BaseWithId: model.BaseWithId{ID: examID2}},
		Title:        "Chemistry 101",
		Status:       exam.StatusPublished,
		DurationMins: 45,
	}
	questionReader.questions[examID2] = []question.Question{
		{Base: model.Base{BaseWithId: model.BaseWithId{ID: uuid.New()}}},
	}
	sess2, duration2, err := svc.StartSession(ctx, StartSessionRequest{ExamID: &examID2}, studentID)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, sess2.Status)
	assert.Equal(t, 45, duration2)
	assert.Equal(t, examID2, sess2.ExamID)
}

func TestSessionService_SubmitSession(t *testing.T) {
	repo := newMockSessionRepo()
	logger := zerolog.Nop()
	studentID := uuid.New()
	examID := uuid.New()
	sessID := uuid.New()

	examReader := &mockExamReader{
		exams: map[uuid.UUID]*exam.Exam{
			examID: {
				Title:        "English Lit",
				Status:       exam.StatusPublished,
				DurationMins: 60,
			},
		},
	}

	repo.sessions[sessID] = &Session{
		ExamID:    examID,
		StudentID: studentID,
		Status:    StatusInProgress,
		StartedAt: time.Now().UTC(),
	}

	cache := newMockCache()
	_ = cache.SetState(context.Background(), sessID, repo.sessions[sessID], time.Hour)

	questionReader := &mockQuestionReader{}

	svc := NewService(repo, nil, examReader, questionReader, cache, &mockTxManager{}, &mockTaskEnqueuer{}, &logger)
	ctx := context.Background()

	// 1. Submit session succeeds
	submitted, err := svc.SubmitSession(ctx, sessID, studentID)
	require.NoError(t, err)
	assert.Equal(t, StatusSubmitted, submitted.Status)
	assert.NotNil(t, submitted.SubmittedAt)

	// 2. Submit already submitted fails
	_, err = svc.SubmitSession(ctx, sessID, studentID)
	assert.ErrorIs(t, err, ErrSessionAlreadySubmitted)
}

func TestSessionService_ListSessions(t *testing.T) {
	logger := zerolog.Nop()
	repo := newMockSessionRepo()
	studentID := uuid.New()
	sess1 := uuid.New()
	sess2 := uuid.New()

	repo.sessions[sess1] = &Session{
		Base:      model.Base{BaseWithId: model.BaseWithId{ID: sess1}},
		StudentID: studentID,
		Status:    StatusInProgress,
		StartedAt: time.Now().UTC(),
	}
	repo.sessions[sess2] = &Session{
		Base:      model.Base{BaseWithId: model.BaseWithId{ID: sess2}},
		StudentID: studentID,
		Status:    StatusSubmitted,
		StartedAt: time.Now().UTC().Add(-2 * time.Hour),
	}

	svc := NewService(repo, nil, nil, nil, newMockCache(), &mockTxManager{}, &mockTaskEnqueuer{}, &logger)
	ctx := context.Background()

	sessions, total, err := svc.ListSessions(ctx, studentID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, sessions, 2)
}
