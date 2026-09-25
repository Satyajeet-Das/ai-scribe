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
	ListExams(ctx context.Context, limit, offset int, status *Status) ([]Exam, int, error)
	CreateExam(ctx context.Context, req CreateExamRequest, createdBy uuid.UUID) (*Exam, error)
	UpdateExam(ctx context.Context, id uuid.UUID, req UpdateExamRequest, callerID uuid.UUID) (*Exam, error)
	PublishExam(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Exam, error)
	ArchiveExam(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Exam, error)
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

func (s *examService) ListExams(ctx context.Context, limit, offset int, status *Status) ([]Exam, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.List(ctx, limit, offset, status)
}

func (s *examService) CreateExam(ctx context.Context, req CreateExamRequest, createdBy uuid.UUID) (*Exam, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	e := &Exam{
		Base: model.Base{
			BaseWithId:        model.BaseWithId{ID: uuid.New()},
			BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
			BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
		},
		Title:        req.Title,
		Subject:      req.Subject,
		Description:  req.Description,
		DurationMins: req.DurationMins,
		Status:       StatusDraft,
		CreatedBy:    createdBy,
	}

	if err := s.repo.Create(ctx, e); err != nil {
		s.logger.Error().Err(err).Str("title", req.Title).Msg("failed to create exam")
		return nil, err
	}

	s.logger.Info().
		Str("event", "exam.created").
		Str("exam_id", e.ID.String()).
		Str("created_by", createdBy.String()).
		Msg("exam created successfully")

	return e, nil
}

func (s *examService) UpdateExam(ctx context.Context, id uuid.UUID, req UpdateExamRequest, callerID uuid.UUID) (*Exam, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrExamNotFound
	}

	if callerID != uuid.Nil && existing.CreatedBy != callerID {
		return nil, ErrUnauthorizedCreator
	}

	if existing.Status != StatusDraft {
		return nil, ErrExamNotDraft
	}

	if req.Title != nil {
		existing.Title = *req.Title
	}
	if req.Subject != nil {
		existing.Subject = *req.Subject
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.DurationMins != nil {
		existing.DurationMins = *req.DurationMins
	}

	if err := s.repo.Update(ctx, existing); err != nil {
		s.logger.Error().Err(err).Str("exam_id", id.String()).Msg("failed to update exam")
		return nil, err
	}

	s.logger.Info().
		Str("event", "exam.updated").
		Str("exam_id", id.String()).
		Msg("exam updated successfully")

	return existing, nil
}

func (s *examService) PublishExam(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Exam, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrExamNotFound
	}

	if callerID != uuid.Nil && existing.CreatedBy != callerID {
		return nil, ErrUnauthorizedCreator
	}

	if existing.Status == StatusPublished {
		return nil, ErrExamAlreadyPublished
	}
	if existing.Status == StatusArchived {
		return nil, ErrExamAlreadyArchived
	}
	if existing.Status != StatusDraft {
		return nil, ErrInvalidExamState
	}

	if existing.DurationMins <= 0 || existing.Subject == "" || existing.Title == "" {
		return nil, ErrExamCannotPublish
	}

	now := time.Now().UTC()
	if err := s.repo.Publish(ctx, id, now); err != nil {
		s.logger.Error().Err(err).Str("exam_id", id.String()).Msg("failed to publish exam")
		return nil, err
	}

	existing.Status = StatusPublished
	existing.PublishedAt = &now
	existing.UpdatedAt = now

	s.logger.Info().
		Str("event", "exam.published").
		Str("exam_id", id.String()).
		Msg("exam published successfully")

	return existing, nil
}

func (s *examService) ArchiveExam(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Exam, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrExamNotFound
	}

	if callerID != uuid.Nil && existing.CreatedBy != callerID {
		return nil, ErrUnauthorizedCreator
	}

	if existing.Status == StatusArchived {
		return nil, ErrExamAlreadyArchived
	}

	if err := s.repo.Archive(ctx, id); err != nil {
		s.logger.Error().Err(err).Str("exam_id", id.String()).Msg("failed to archive exam")
		return nil, err
	}

	existing.Status = StatusArchived
	existing.UpdatedAt = time.Now().UTC()

	s.logger.Info().
		Str("event", "exam.archived").
		Str("exam_id", id.String()).
		Msg("exam archived successfully")

	return existing, nil
}
