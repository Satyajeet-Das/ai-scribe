package answer

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Service interface {
	GetAnswer(ctx context.Context, id uuid.UUID) (*Answer, error)
	ListAnswersBySession(ctx context.Context, sessionID uuid.UUID) ([]Answer, error)
	SubmitAnswer(ctx context.Context, req SubmitAnswerRequest, candidateID uuid.UUID) (*Answer, error)
}

type answerService struct {
	repo   Repository
	logger *zerolog.Logger
}

func NewService(repo Repository, logger *zerolog.Logger) Service {
	return &answerService{
		repo:   repo,
		logger: logger,
	}
}

func (s *answerService) GetAnswer(ctx context.Context, id uuid.UUID) (*Answer, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrAnswerNotFound
	}
	return a, nil
}

func (s *answerService) ListAnswersBySession(ctx context.Context, sessionID uuid.UUID) ([]Answer, error) {
	return s.repo.ListBySessionID(ctx, sessionID)
}

func (s *answerService) SubmitAnswer(ctx context.Context, req SubmitAnswerRequest, candidateID uuid.UUID) (*Answer, error) {
	now := time.Now().UTC()
	a := &Answer{
		Base: model.Base{
			BaseWithId:        model.BaseWithId{ID: uuid.New()},
			BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
			BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
		},
		SessionID:    req.SessionID,
		QuestionID:   req.QuestionID,
		CandidateID:  candidateID,
		ResponseText: req.ResponseText,
		AudioURL:     req.AudioURL,
	}

	if err := s.repo.Create(ctx, a); err != nil {
		s.logger.Error().Err(err).Msg("failed to record candidate answer")
		return nil, err
	}

	return a, nil
}
