package assignment

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type ExamReader interface {
	GetExam(ctx context.Context, id uuid.UUID) (*exam.Exam, error)
}

type Service interface {
	GetAssignment(ctx context.Context, id uuid.UUID) (*Assignment, error)
	ListAssignments(ctx context.Context, limit, offset int, examID, studentID *uuid.UUID, status *Status) ([]Assignment, int, error)
	CreateAssignment(ctx context.Context, req CreateAssignmentRequest, callerID uuid.UUID) (*Assignment, error)
	RevokeAssignment(ctx context.Context, id uuid.UUID, callerID uuid.UUID) error
}

type assignmentService struct {
	repo       Repository
	examReader ExamReader
	logger     *zerolog.Logger
}

func NewService(repo Repository, examReader ExamReader, logger *zerolog.Logger) Service {
	return &assignmentService{
		repo:       repo,
		examReader: examReader,
		logger:     logger,
	}
}

func (s *assignmentService) GetAssignment(ctx context.Context, id uuid.UUID) (*Assignment, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *assignmentService) ListAssignments(ctx context.Context, limit, offset int, examID, studentID *uuid.UUID, status *Status) ([]Assignment, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.List(ctx, limit, offset, examID, studentID, status)
}

func (s *assignmentService) CreateAssignment(ctx context.Context, req CreateAssignmentRequest, callerID uuid.UUID) (*Assignment, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	ex, err := s.examReader.GetExam(ctx, req.ExamID)
	if err != nil {
		return nil, err
	}
	if ex == nil {
		return nil, exam.ErrExamNotFound
	}

	if callerID != uuid.Nil && ex.CreatedBy != callerID {
		return nil, ErrUnauthorized
	}

	if ex.Status != exam.StatusPublished {
		return nil, ErrExamNotPublished
	}

	existing, err := s.repo.GetActiveByExamAndStudent(ctx, req.ExamID, req.StudentID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrDuplicateAssignment
	}

	now := time.Now().UTC()
	a := &Assignment{
		Base: model.Base{
			BaseWithId:        model.BaseWithId{ID: uuid.New()},
			BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
			BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
		},
		ExamID:     req.ExamID,
		StudentID:  req.StudentID,
		AssignedAt: now,
		Status:     StatusAssigned,
	}

	if err := s.repo.Create(ctx, a); err != nil {
		s.logger.Error().Err(err).
			Str("exam_id", req.ExamID.String()).
			Str("student_id", req.StudentID.String()).
			Msg("failed to create assignment")
		return nil, err
	}

	s.logger.Info().
		Str("event", "assignment.created").
		Str("assignment_id", a.ID.String()).
		Str("exam_id", req.ExamID.String()).
		Str("student_id", req.StudentID.String()).
		Msg("assignment created successfully")

	return a, nil
}

func (s *assignmentService) RevokeAssignment(ctx context.Context, id uuid.UUID, callerID uuid.UUID) error {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return ErrAssignmentNotFound
	}

	if a.Status == StatusRevoked {
		return ErrAssignmentAlreadyRevoked
	}

	ex, err := s.examReader.GetExam(ctx, a.ExamID)
	if err != nil {
		return err
	}
	if ex == nil {
		return exam.ErrExamNotFound
	}

	if callerID != uuid.Nil && ex.CreatedBy != callerID {
		return ErrUnauthorized
	}

	if err := s.repo.Revoke(ctx, id); err != nil {
		s.logger.Error().Err(err).Str("assignment_id", id.String()).Msg("failed to revoke assignment")
		return err
	}

	s.logger.Info().
		Str("event", "assignment.revoked").
		Str("assignment_id", id.String()).
		Msg("assignment revoked successfully")

	return nil
}
