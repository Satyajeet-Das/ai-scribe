package session

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Service interface {
	GetSession(ctx context.Context, id uuid.UUID) (*Session, error)
	StartSession(ctx context.Context, assignmentID, candidateID, examID uuid.UUID) (*Session, error)
}

type sessionService struct {
	repo   Repository
	logger *zerolog.Logger
}

func NewService(repo Repository, logger *zerolog.Logger) Service {
	return &sessionService{
		repo:   repo,
		logger: logger,
	}
}

func (s *sessionService) GetSession(ctx context.Context, id uuid.UUID) (*Session, error) {
	sess, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, ErrSessionNotFound
	}
	return sess, nil
}

func (s *sessionService) StartSession(ctx context.Context, assignmentID, candidateID, examID uuid.UUID) (*Session, error) {
	now := time.Now().UTC()
	sess := &Session{
		Base: model.Base{
			BaseWithId:        model.BaseWithId{ID: uuid.New()},
			BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
			BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
		},
		AssignmentID: assignmentID,
		CandidateID:  candidateID,
		ExamID:       examID,
		Status:       StatusInProgress,
		StartedAt:    &now,
		CurrentIndex: 0,
	}

	if err := s.repo.Create(ctx, sess); err != nil {
		s.logger.Error().Err(err).Msg("failed to start session")
		return nil, err
	}

	return sess, nil
}
