package answer

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/question"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
)

type SessionReader interface {
	GetSession(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*session.Session, int, error)
}

type QuestionReader interface {
	GetQuestion(ctx context.Context, id uuid.UUID) (*question.Question, error)
	GetOption(ctx context.Context, id uuid.UUID) (*question.QuestionOption, error)
}

type Service interface {
	SubmitAnswer(ctx context.Context, sessionID, questionID uuid.UUID, req SubmitAnswerRequest, callerID uuid.UUID) (*Answer, error)
	ListAnswers(ctx context.Context, sessionID uuid.UUID, callerID uuid.UUID) ([]Answer, error)
}

type answerService struct {
	repo           Repository
	sessionReader  SessionReader
	questionReader QuestionReader
	logger         *zerolog.Logger
}

func NewService(repo Repository, sessionReader SessionReader, questionReader QuestionReader, logger *zerolog.Logger) Service {
	return &answerService{
		repo:           repo,
		sessionReader:  sessionReader,
		questionReader: questionReader,
		logger:         logger,
	}
}

func (s *answerService) SubmitAnswer(ctx context.Context, sessionID, questionID uuid.UUID, req SubmitAnswerRequest, callerID uuid.UUID) (*Answer, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	sess, _, err := s.sessionReader.GetSession(ctx, sessionID, callerID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, session.ErrSessionNotFound
	}

	if callerID != uuid.Nil && sess.StudentID != callerID {
		return nil, ErrUnauthorizedStudent
	}

	if sess.Status != session.StatusInProgress {
		return nil, ErrSessionNotActive
	}

	q, err := s.questionReader.GetQuestion(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, question.ErrQuestionNotFound
	}

	if q.ExamID != sess.ExamID {
		return nil, ErrQuestionNotForExam
	}

	if req.SelectedOptionID != nil {
		opt, err := s.questionReader.GetOption(ctx, *req.SelectedOptionID)
		if err != nil {
			return nil, err
		}
		if opt == nil || opt.QuestionID != q.ID {
			return nil, ErrOptionNotForQuestion
		}
	}

	ans := &Answer{
		SessionID:        sessionID,
		QuestionID:       questionID,
		SelectedOptionID: req.SelectedOptionID,
		TextAnswer:       req.TextAnswer,
	}

	if err := s.repo.Upsert(ctx, ans); err != nil {
		s.logger.Error().Err(err).
			Str("session_id", sessionID.String()).
			Str("question_id", questionID.String()).
			Msg("failed to save answer")
		return nil, err
	}

	s.logger.Info().
		Str("event", "answer.submitted").
		Str("answer_id", ans.ID.String()).
		Str("session_id", sessionID.String()).
		Str("question_id", questionID.String()).
		Msg("answer submitted successfully")

	return ans, nil
}

func (s *answerService) ListAnswers(ctx context.Context, sessionID uuid.UUID, callerID uuid.UUID) ([]Answer, error) {
	sess, _, err := s.sessionReader.GetSession(ctx, sessionID, callerID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, session.ErrSessionNotFound
	}

	if callerID != uuid.Nil && sess.StudentID != callerID {
		return nil, ErrUnauthorizedStudent
	}

	return s.repo.ListBySession(ctx, sessionID)
}
