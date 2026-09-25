package session

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/assignment"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type AssignmentReader interface {
	GetAssignment(ctx context.Context, id uuid.UUID) (*assignment.Assignment, error)
}

type ExamReader interface {
	GetExam(ctx context.Context, id uuid.UUID) (*exam.Exam, error)
}

type Service interface {
	GetSession(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Session, int, error)
	StartSession(ctx context.Context, req StartSessionRequest, callerID uuid.UUID) (*Session, int, error)
	SubmitSession(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Session, error)
}

type sessionService struct {
	repo             Repository
	assignmentReader AssignmentReader
	examReader       ExamReader
	logger           *zerolog.Logger
}

func NewService(repo Repository, assignmentReader AssignmentReader, examReader ExamReader, logger *zerolog.Logger) Service {
	return &sessionService{
		repo:             repo,
		assignmentReader: assignmentReader,
		examReader:       examReader,
		logger:           logger,
	}
}

func (s *sessionService) GetSession(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Session, int, error) {
	sess, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	if sess == nil {
		return nil, 0, ErrSessionNotFound
	}

	ex, err := s.examReader.GetExam(ctx, sess.ExamID)
	if err != nil {
		return nil, 0, err
	}
	if ex == nil {
		return nil, 0, exam.ErrExamNotFound
	}

	// Auto-expire if time has elapsed
	if sess.Status == StatusInProgress && ex.DurationMins > 0 {
		maxAllowed := time.Duration(ex.DurationMins) * time.Minute
		if time.Since(sess.StartedAt) > maxAllowed {
			_ = s.repo.Expire(ctx, id)
			sess.Status = StatusExpired
		}
	}

	return sess, ex.DurationMins, nil
}

func (s *sessionService) StartSession(ctx context.Context, req StartSessionRequest, callerID uuid.UUID) (*Session, int, error) {
	if err := req.Validate(); err != nil {
		return nil, 0, err
	}

	asgn, err := s.assignmentReader.GetAssignment(ctx, req.AssignmentID)
	if err != nil {
		return nil, 0, err
	}
	if asgn == nil {
		return nil, 0, ErrInvalidAssignment
	}

	if callerID != uuid.Nil && asgn.StudentID != callerID {
		return nil, 0, ErrUnauthorizedStudent
	}

	if asgn.Status == assignment.StatusRevoked {
		return nil, 0, ErrAssignmentRevoked
	}

	ex, err := s.examReader.GetExam(ctx, asgn.ExamID)
	if err != nil {
		return nil, 0, err
	}
	if ex == nil {
		return nil, 0, exam.ErrExamNotFound
	}

	if ex.Status == exam.StatusArchived {
		return nil, 0, ErrExamArchived
	}
	if ex.Status != exam.StatusPublished {
		return nil, 0, ErrExamNotPublished
	}

	existingActive, err := s.repo.GetActiveByAssignmentID(ctx, req.AssignmentID)
	if err != nil {
		return nil, 0, err
	}
	if existingActive != nil {
		return nil, 0, ErrActiveSessionAlreadyExists
	}

	now := time.Now().UTC()
	sess := &Session{
		Base: model.Base{
			BaseWithId:        model.BaseWithId{ID: uuid.New()},
			BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
			BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
		},
		AssignmentID: req.AssignmentID,
		ExamID:       asgn.ExamID,
		StudentID:    asgn.StudentID,
		Status:       StatusInProgress,
		StartedAt:    now,
	}

	if err := s.repo.Create(ctx, sess); err != nil {
		s.logger.Error().Err(err).
			Str("assignment_id", req.AssignmentID.String()).
			Str("student_id", asgn.StudentID.String()).
			Msg("failed to create exam session")
		return nil, 0, err
	}

	s.logger.Info().
		Str("event", "session.started").
		Str("session_id", sess.ID.String()).
		Str("assignment_id", req.AssignmentID.String()).
		Str("student_id", asgn.StudentID.String()).
		Msg("exam session started successfully")

	return sess, ex.DurationMins, nil
}

func (s *sessionService) SubmitSession(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Session, error) {
	sess, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, ErrSessionNotFound
	}

	if callerID != uuid.Nil && sess.StudentID != callerID {
		return nil, ErrUnauthorizedStudent
	}

	if sess.Status == StatusSubmitted {
		return nil, ErrSessionAlreadySubmitted
	}

	ex, err := s.examReader.GetExam(ctx, sess.ExamID)
	if err != nil {
		return nil, err
	}
	if ex != nil && ex.DurationMins > 0 {
		maxAllowed := time.Duration(ex.DurationMins) * time.Minute
		if time.Since(sess.StartedAt) > maxAllowed {
			_ = s.repo.Expire(ctx, id)
			sess.Status = StatusExpired
			return nil, ErrSessionExpired
		}
	}

	if sess.Status != StatusInProgress {
		return nil, ErrSessionNotInProgress
	}

	now := time.Now().UTC()
	if err := s.repo.Submit(ctx, id, now); err != nil {
		s.logger.Error().Err(err).Str("session_id", id.String()).Msg("failed to submit session")
		return nil, err
	}

	sess.Status = StatusSubmitted
	sess.SubmittedAt = &now
	sess.UpdatedAt = now

	s.logger.Info().
		Str("event", "session.submitted").
		Str("session_id", id.String()).
		Str("student_id", sess.StudentID.String()).
		Msg("exam session submitted successfully")

	return sess, nil
}
