package exam

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Service interface {
	GetExam(ctx context.Context, id uuid.UUID) (*Exam, error)
	ListExams(ctx context.Context, limit, offset int) ([]Exam, int, error)
	CreateExam(ctx context.Context, req CreateExamRequest, createdBy uuid.UUID) (*Exam, error)
}

type examService struct {
	repo   Repository
	logger *zerolog.Logger
}

func NewService(repo Repository, logger *zerolog.Logger) Service {
	return &examService{
		repo:   repo,
		logger: logger,
	}
}

func (s *examService) GetExam(ctx context.Context, id uuid.UUID) (*Exam, error) {
	e, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrExamNotFound
	}
	return e, nil
}

func (s *examService) ListExams(ctx context.Context, limit, offset int) ([]Exam, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.List(ctx, limit, offset)
}

func (s *examService) CreateExam(ctx context.Context, req CreateExamRequest, createdBy uuid.UUID) (*Exam, error) {
	now := time.Now().UTC()
	e := &Exam{
		Base: model.Base{
			BaseWithId:        model.BaseWithId{ID: uuid.New()},
			BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
			BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
		},
		Title:           req.Title,
		Description:     req.Description,
		Subject:         req.Subject,
		DurationMinutes: req.DurationMinutes,
		Status:          StatusDraft,
		ScheduledStart:  req.ScheduledStart,
		ScheduledEnd:    req.ScheduledEnd,
		CreatedBy:       createdBy,
	}

	if err := s.repo.Create(ctx, e); err != nil {
		s.logger.Error().Err(err).Str("title", req.Title).Msg("failed to create exam")
		return nil, err
	}

	return e, nil
}
