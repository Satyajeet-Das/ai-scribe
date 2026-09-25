package answer_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/answer"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
)

type mockRepo struct {
	answers map[string]answer.Answer
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		answers: make(map[string]answer.Answer),
	}
}

func (m *mockRepo) GetByID(ctx context.Context, id uuid.UUID) (*answer.Answer, error) {
	for _, a := range m.answers {
		if a.ID == id {
			return &a, nil
		}
	}
	return nil, answer.ErrAnswerNotFound
}

func (m *mockRepo) Upsert(ctx context.Context, a *answer.Answer) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now
	key := a.SessionID.String() + ":" + a.QuestionID.String()
	m.answers[key] = *a
	return nil
}

func (m *mockRepo) GetBySessionAndQuestion(ctx context.Context, sessionID, questionID uuid.UUID) (*answer.Answer, error) {
	key := sessionID.String() + ":" + questionID.String()
	a, ok := m.answers[key]
	if !ok {
		return nil, answer.ErrAnswerNotFound
	}
	return &a, nil
}

func (m *mockRepo) ListBySession(ctx context.Context, sessionID uuid.UUID) ([]answer.Answer, error) {
	var list []answer.Answer
	for _, a := range m.answers {
		if a.SessionID == sessionID {
			list = append(list, a)
		}
	}
	return list, nil
}

type mockSessionReader struct {
	sessions map[uuid.UUID]*session.Session
}

func (m *mockSessionReader) GetSession(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*session.Session, int, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, 0, session.ErrSessionNotFound
	}
	return s, 60, nil
}

type mockQuestionReader struct {
	questions map[uuid.UUID]*question.Question
	options   map[uuid.UUID]*question.QuestionOption
}

func (m *mockQuestionReader) GetQuestion(ctx context.Context, id uuid.UUID) (*question.Question, error) {
	q, ok := m.questions[id]
	if !ok {
		return nil, question.ErrQuestionNotFound
	}
	return q, nil
}

func (m *mockQuestionReader) GetOption(ctx context.Context, id uuid.UUID) (*question.QuestionOption, error) {
	opt, ok := m.options[id]
	if !ok {
		return nil, question.ErrOptionNotFound
	}
	return opt, nil
}

func setupTestService() (answer.Service, *mockRepo, *mockSessionReader, *mockQuestionReader) {
	repo := newMockRepo()
	sessReader := &mockSessionReader{sessions: make(map[uuid.UUID]*session.Session)}
	qReader := &mockQuestionReader{
		questions: make(map[uuid.UUID]*question.Question),
		options:   make(map[uuid.UUID]*question.QuestionOption),
	}
	logger := zerolog.Nop()
	svc := answer.NewService(repo, sessReader, qReader, &logger)
	return svc, repo, sessReader, qReader
}

func TestAnswerService_SubmitAnswer(t *testing.T) {
	ctx := context.Background()
	studentID := uuid.New()
	examID := uuid.New()
	sessionID := uuid.New()
	questionID := uuid.New()
	optID := uuid.New()

	t.Run("successfully submit mcq answer", func(t *testing.T) {
		svc, _, sessReader, qReader := setupTestService()

		sess := &session.Session{
			StudentID: studentID,
			ExamID:    examID,
			Status:    session.StatusInProgress,
		}
		sess.ID = sessionID
		sessReader.sessions[sessionID] = sess

		q := &question.Question{
			ExamID: examID,
			Type:   question.TypeMCQ,
		}
		q.ID = questionID
		qReader.questions[questionID] = q

		opt := &question.QuestionOption{
			QuestionID: questionID,
		}
		opt.ID = optID
		qReader.options[optID] = opt

		req := answer.SubmitAnswerRequest{
			SelectedOptionID: &optID,
		}

		ans, err := svc.SubmitAnswer(ctx, sessionID, questionID, req, studentID)
		require.NoError(t, err)
		assert.NotNil(t, ans)
		assert.Equal(t, sessionID, ans.SessionID)
		assert.Equal(t, questionID, ans.QuestionID)
		assert.Equal(t, &optID, ans.SelectedOptionID)
	})

	t.Run("successfully submit text answer", func(t *testing.T) {
		svc, _, sessReader, qReader := setupTestService()

		sess := &session.Session{
			StudentID: studentID,
			ExamID:    examID,
			Status:    session.StatusInProgress,
		}
		sess.ID = sessionID
		sessReader.sessions[sessionID] = sess

		q := &question.Question{
			ExamID: examID,
			Type:   question.TypeEssay,
		}
		q.ID = questionID
		qReader.questions[questionID] = q

		req := answer.SubmitAnswerRequest{
			TextAnswer: "Photosynthesis occurs in chloroplasts.",
		}

		ans, err := svc.SubmitAnswer(ctx, sessionID, questionID, req, studentID)
		require.NoError(t, err)
		assert.NotNil(t, ans)
		assert.Equal(t, "Photosynthesis occurs in chloroplasts.", ans.TextAnswer)
	})

	t.Run("fails when neither option nor text provided", func(t *testing.T) {
		svc, _, _, _ := setupTestService()

		req := answer.SubmitAnswerRequest{}
		_, err := svc.SubmitAnswer(ctx, sessionID, questionID, req, studentID)
		require.ErrorIs(t, err, answer.ErrInvalidAnswer)
	})

	t.Run("fails when session not found", func(t *testing.T) {
		svc, _, _, _ := setupTestService()

		req := answer.SubmitAnswerRequest{TextAnswer: "hello"}
		_, err := svc.SubmitAnswer(ctx, sessionID, questionID, req, studentID)
		require.ErrorIs(t, err, session.ErrSessionNotFound)
	})

	t.Run("fails when student does not own session", func(t *testing.T) {
		svc, _, sessReader, _ := setupTestService()

		sess := &session.Session{
			StudentID: studentID,
			ExamID:    examID,
			Status:    session.StatusInProgress,
		}
		sess.ID = sessionID
		sessReader.sessions[sessionID] = sess

		otherStudent := uuid.New()
		req := answer.SubmitAnswerRequest{TextAnswer: "hello"}
		_, err := svc.SubmitAnswer(ctx, sessionID, questionID, req, otherStudent)
		require.ErrorIs(t, err, answer.ErrUnauthorizedStudent)
	})

	t.Run("fails when session is not in progress", func(t *testing.T) {
		svc, _, sessReader, _ := setupTestService()

		sess := &session.Session{
			StudentID: studentID,
			ExamID:    examID,
			Status:    session.StatusSubmitted,
		}
		sess.ID = sessionID
		sessReader.sessions[sessionID] = sess

		req := answer.SubmitAnswerRequest{TextAnswer: "hello"}
		_, err := svc.SubmitAnswer(ctx, sessionID, questionID, req, studentID)
		require.ErrorIs(t, err, answer.ErrSessionNotActive)
	})

	t.Run("fails when question not found", func(t *testing.T) {
		svc, _, sessReader, _ := setupTestService()

		sess := &session.Session{
			StudentID: studentID,
			ExamID:    examID,
			Status:    session.StatusInProgress,
		}
		sess.ID = sessionID
		sessReader.sessions[sessionID] = sess

		req := answer.SubmitAnswerRequest{TextAnswer: "hello"}
		_, err := svc.SubmitAnswer(ctx, sessionID, questionID, req, studentID)
		require.ErrorIs(t, err, question.ErrQuestionNotFound)
	})

	t.Run("fails when question does not belong to session exam", func(t *testing.T) {
		svc, _, sessReader, qReader := setupTestService()

		sess := &session.Session{
			StudentID: studentID,
			ExamID:    examID,
			Status:    session.StatusInProgress,
		}
		sess.ID = sessionID
		sessReader.sessions[sessionID] = sess

		otherExamID := uuid.New()
		q := &question.Question{
			ExamID: otherExamID,
		}
		q.ID = questionID
		qReader.questions[questionID] = q

		req := answer.SubmitAnswerRequest{TextAnswer: "hello"}
		_, err := svc.SubmitAnswer(ctx, sessionID, questionID, req, studentID)
		require.ErrorIs(t, err, answer.ErrQuestionNotForExam)
	})

	t.Run("fails when option does not belong to question", func(t *testing.T) {
		svc, _, sessReader, qReader := setupTestService()

		sess := &session.Session{
			StudentID: studentID,
			ExamID:    examID,
			Status:    session.StatusInProgress,
		}
		sess.ID = sessionID
		sessReader.sessions[sessionID] = sess

		q := &question.Question{
			ExamID: examID,
			Type:   question.TypeMCQ,
		}
		q.ID = questionID
		qReader.questions[questionID] = q

		otherQuestionID := uuid.New()
		opt := &question.QuestionOption{
			QuestionID: otherQuestionID,
		}
		opt.ID = optID
		qReader.options[optID] = opt

		req := answer.SubmitAnswerRequest{SelectedOptionID: &optID}
		_, err := svc.SubmitAnswer(ctx, sessionID, questionID, req, studentID)
		require.ErrorIs(t, err, answer.ErrOptionNotForQuestion)
	})
}

func TestAnswerService_ListAnswers(t *testing.T) {
	ctx := context.Background()
	studentID := uuid.New()
	sessionID := uuid.New()
	q1 := uuid.New()
	q2 := uuid.New()

	t.Run("successfully list answers for student session", func(t *testing.T) {
		svc, repo, sessReader, _ := setupTestService()

		sess := &session.Session{
			StudentID: studentID,
			Status:    session.StatusInProgress,
		}
		sess.ID = sessionID
		sessReader.sessions[sessionID] = sess

		_ = repo.Upsert(ctx, &answer.Answer{SessionID: sessionID, QuestionID: q1, TextAnswer: "Ans 1"})
		_ = repo.Upsert(ctx, &answer.Answer{SessionID: sessionID, QuestionID: q2, TextAnswer: "Ans 2"})

		list, err := svc.ListAnswers(ctx, sessionID, studentID)
		require.NoError(t, err)
		assert.Len(t, list, 2)
	})

	t.Run("fails when unauthorized student tries to list", func(t *testing.T) {
		svc, _, sessReader, _ := setupTestService()

		sess := &session.Session{
			StudentID: studentID,
			Status:    session.StatusInProgress,
		}
		sess.ID = sessionID
		sessReader.sessions[sessionID] = sess

		otherStudent := uuid.New()
		_, err := svc.ListAnswers(ctx, sessionID, otherStudent)
		require.ErrorIs(t, err, answer.ErrUnauthorizedStudent)
	})
}
