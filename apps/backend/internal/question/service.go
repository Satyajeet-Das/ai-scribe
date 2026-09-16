package question

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Service interface {
	GetQuestion(ctx context.Context, id uuid.UUID) (*Question, error)
	ListQuestionsByExam(ctx context.Context, examID uuid.UUID) ([]Question, error)
	CreateQuestion(ctx context.Context, req CreateQuestionRequest) (*Question, error)
}

type questionService struct {
	repo   Repository
	logger *zerolog.Logger
}

func NewService(repo Repository, logger *zerolog.Logger) Service {
	return &questionService{
		repo:   repo,
		logger: logger,
	}
}

func (s *questionService) GetQuestion(ctx context.Context, id uuid.UUID) (*Question, error) {
	q, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, ErrQuestionNotFound
	}
	return q, nil
}

func (s *questionService) ListQuestionsByExam(ctx context.Context, examID uuid.UUID) ([]Question, error) {
	return s.repo.ListByExamID(ctx, examID)
}

func (s *questionService) CreateQuestion(ctx context.Context, req CreateQuestionRequest) (*Question, error) {
	now := time.Now().UTC()
	q := &Question{
		Base: model.Base{
			BaseWithId:        model.BaseWithId{ID: uuid.New()},
			BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
			BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
		},
		ExamID:        req.ExamID,
		SequenceOrder: req.SequenceOrder,
		Type:          req.Type,
		Prompt:        req.Prompt,
		Points:        req.Points,
	}

	if err := s.repo.Create(ctx, q); err != nil {
		s.logger.Error().Err(err).Str("examId", req.ExamID.String()).Msg("failed to create question")
		return nil, err
	}

	return q, nil
}
