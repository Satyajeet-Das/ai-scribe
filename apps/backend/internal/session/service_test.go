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

type mockExamReader struct {
	exams map[uuid.UUID]*exam.Exam
}

func (m *mockExamReader) GetExam(ctx context.Context, id uuid.UUID) (*exam.Exam, error) {
	if e, ok := m.exams[id]; ok {
		return e, nil
	}
	return nil, exam.ErrExamNotFound
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

	svc := NewService(repo, asgnReader, examReader, &logger)
	ctx := context.Background()

	// 1. Starting valid session succeeds
	req := StartSessionRequest{AssignmentID: assignmentID}
	sess, duration, err := svc.StartSession(ctx, req, studentID)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, sess.Status)
	assert.Equal(t, 60, duration)
	assert.Equal(t, studentID, sess.StudentID)

	// 2. Duplicate active session fails
	_, _, err = svc.StartSession(ctx, req, studentID)
	assert.ErrorIs(t, err, ErrActiveSessionAlreadyExists)

	// 3. Unauthorized student fails
	otherStudent := uuid.New()
	asgn2 := uuid.New()
	asgnReader.assignments[asgn2] = &assignment.Assignment{
		ExamID:    examID,
		StudentID: studentID,
		Status:    assignment.StatusAssigned,
	}
	_, _, err = svc.StartSession(ctx, StartSessionRequest{AssignmentID: asgn2}, otherStudent)
	assert.ErrorIs(t, err, ErrUnauthorizedStudent)

	// 4. Revoked assignment fails
	asgnRevoked := uuid.New()
	asgnReader.assignments[asgnRevoked] = &assignment.Assignment{
		ExamID:    examID,
		StudentID: studentID,
		Status:    assignment.StatusRevoked,
	}
	_, _, err = svc.StartSession(ctx, StartSessionRequest{AssignmentID: asgnRevoked}, studentID)
	assert.ErrorIs(t, err, ErrAssignmentRevoked)
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

	svc := NewService(repo, nil, examReader, &logger)
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
