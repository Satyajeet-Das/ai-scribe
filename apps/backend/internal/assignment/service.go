package assignment

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Service interface {
	GetAssignment(ctx context.Context, id uuid.UUID) (*Assignment, error)
	CreateAssignment(ctx context.Context, req CreateAssignmentRequest) (*Assignment, error)
}

type assignmentService struct {
	repo   Repository
	logger *zerolog.Logger
}

func NewService(repo Repository, logger *zerolog.Logger) Service {
	return &assignmentService{
		repo:   repo,
		logger: logger,
	}
}

func (s *assignmentService) GetAssignment(ctx context.Context, id uuid.UUID) (*Assignment, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrAssignmentNotFound
	}
	return a, nil
}

func (s *assignmentService) CreateAssignment(ctx context.Context, req CreateAssignmentRequest) (*Assignment, error) {
	now := time.Now().UTC()
	a := &Assignment{
		Base: model.Base{
			BaseWithId:        model.BaseWithId{ID: uuid.New()},
			BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
			BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
		},
		ExamID:      req.ExamID,
		CandidateID: req.CandidateID,
		Status:      StatusAssigned,
	}

	if err := s.repo.Create(ctx, a); err != nil {
		s.logger.Error().Err(err).Msg("failed to create assignment")
		return nil, err
	}

	return a, nil
}
