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
	GetExamForCaller(ctx context.Context, id uuid.UUID, caller Caller) (*Exam, error)
	ListExams(ctx context.Context, params ListExamsParams, caller Caller) ([]Exam, int, error)
	CreateExam(ctx context.Context, req CreateExamRequest, caller Caller) (*Exam, error)
	UpdateExam(ctx context.Context, id uuid.UUID, req UpdateExamRequest, caller Caller) (*Exam, error)
	PublishExam(ctx context.Context, id uuid.UUID, caller Caller) (*Exam, error)
	UnpublishExam(ctx context.Context, id uuid.UUID, caller Caller) (*Exam, error)
	ArchiveExam(ctx context.Context, id uuid.UUID, caller Caller) (*Exam, error)
	DeleteExam(ctx context.Context, id uuid.UUID, caller Caller) error
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

func (s *examService) GetExamForCaller(ctx context.Context, id uuid.UUID, caller Caller) (*Exam, error) {
	e, err := s.GetExam(ctx, id)
	if err != nil {
		return nil, err
	}

	if caller.IsAdmin() {
		return e, nil
	}

	if caller.IsTeacher() {
		if !caller.CanManage(e.CreatedBy) {
			return nil, ErrUnauthorizedCreator
		}
		return e, nil
	}

	if caller.IsStudent() {
		if e.Status != StatusPublished {
			return nil, ErrExamNotFound
		}
		isAssigned, err := s.repo.IsStudentAssigned(ctx, id, caller.ID)
		if err != nil {
			return nil, err
		}
		if !isAssigned {
			return nil, ErrUnauthorizedCreator
		}
		return e, nil
	}

	return nil, ErrInvalidCallerRole
}

func (s *examService) ListExams(ctx context.Context, params ListExamsParams, caller Caller) ([]Exam, int, error) {
	params.Defaults()

	if caller.IsStudent() {
		pub := StatusPublished
		params.Status = &pub
		params.AssignedStudentID = &caller.ID
		return s.repo.List(ctx, params)
	}

	if caller.IsTeacher() {
		if !caller.IsAdmin() {
			params.CreatedBy = &caller.ID
		}
		return s.repo.List(ctx, params)
	}

	if caller.IsAdmin() {
		return s.repo.List(ctx, params)
	}

	return nil, 0, ErrInvalidCallerRole
}

func (s *examService) CreateExam(ctx context.Context, req CreateExamRequest, caller Caller) (*Exam, error) {
	if !caller.IsAdmin() && !caller.IsTeacher() {
		return nil, ErrInvalidCallerRole
	}

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
		CreatedBy:    caller.ID,
	}

	if err := s.repo.Create(ctx, e); err != nil {
		s.logger.Error().Err(err).Str("title", req.Title).Msg("failed to create exam")
		return nil, err
	}

	s.logger.Info().
		Str("event", "exam.created").
		Str("exam_id", e.ID.String()).
		Str("created_by", caller.ID.String()).
		Msg("exam created successfully")

	return e, nil
}

func (s *examService) UpdateExam(ctx context.Context, id uuid.UUID, req UpdateExamRequest, caller Caller) (*Exam, error) {
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

	if !caller.CanManage(existing.CreatedBy) {
		return nil, ErrUnauthorizedCreator
	}

	if existing.Status == StatusArchived {
		return nil, ErrExamAlreadyArchived
	}

	if existing.Status == StatusPublished {
		// Published exams cannot have unsafe structural changes (duration alteration)
		if req.DurationMins != nil && *req.DurationMins != existing.DurationMins {
			return nil, ErrPublishedStructuralChange
		}
	} else if existing.Status != StatusDraft {
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

func (s *examService) PublishExam(ctx context.Context, id uuid.UUID, caller Caller) (*Exam, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrExamNotFound
	}

	if !caller.CanManage(existing.CreatedBy) {
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

func (s *examService) UnpublishExam(ctx context.Context, id uuid.UUID, caller Caller) (*Exam, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrExamNotFound
	}

	if !caller.CanManage(existing.CreatedBy) {
		return nil, ErrUnauthorizedCreator
	}

	if existing.Status == StatusDraft {
		return nil, ErrExamNotPublished
	}
	if existing.Status == StatusArchived {
		return nil, ErrExamAlreadyArchived
	}
	if existing.Status != StatusPublished {
		return nil, ErrInvalidExamState
	}

	// Emergency unpublishing: log if active sessions/assignments exist, but allow unpublishing
	hasRelations, err := s.repo.HasActiveSessionsOrAssignments(ctx, id)
	if err != nil {
		s.logger.Error().Err(err).Str("exam_id", id.String()).Msg("failed to check relations before unpublish")
		return nil, err
	}
	if hasRelations {
		s.logger.Warn().Str("exam_id", id.String()).Msg("emergency unpublish invoked while active sessions or assignments exist")
	}

	if err := s.repo.Unpublish(ctx, id); err != nil {
		s.logger.Error().Err(err).Str("exam_id", id.String()).Msg("failed to unpublish exam")
		return nil, err
	}

	existing.Status = StatusDraft
	existing.PublishedAt = nil
	existing.UpdatedAt = time.Now().UTC()

	s.logger.Info().
		Str("event", "exam.unpublished").
		Str("exam_id", id.String()).
		Msg("exam unpublished successfully")

	return existing, nil
}

func (s *examService) ArchiveExam(ctx context.Context, id uuid.UUID, caller Caller) (*Exam, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrExamNotFound
	}

	if !caller.CanManage(existing.CreatedBy) {
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

func (s *examService) DeleteExam(ctx context.Context, id uuid.UUID, caller Caller) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrExamNotFound
	}

	if !caller.CanManage(existing.CreatedBy) {
		return ErrUnauthorizedCreator
	}

	hasRelations, err := s.repo.HasActiveSessionsOrAssignments(ctx, id)
	if err != nil {
		s.logger.Error().Err(err).Str("exam_id", id.String()).Msg("failed to check relations before delete")
		return err
	}
	if hasRelations {
		return ErrCannotDeleteActiveExam
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		s.logger.Error().Err(err).Str("exam_id", id.String()).Msg("failed to delete exam")
		return err
	}

	s.logger.Info().
		Str("event", "exam.deleted").
		Str("exam_id", id.String()).
		Msg("exam deleted successfully")

	return nil
}
