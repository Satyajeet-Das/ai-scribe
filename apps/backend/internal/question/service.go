package question

import (
	"context"
	"strings"
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
	GetQuestion(ctx context.Context, id uuid.UUID) (*Question, error)
	ListQuestionsByExam(ctx context.Context, examID uuid.UUID) ([]Question, error)
	CreateQuestion(ctx context.Context, examID uuid.UUID, req CreateQuestionRequest, callerID uuid.UUID) (*Question, error)
	UpdateQuestion(ctx context.Context, id uuid.UUID, req UpdateQuestionRequest, callerID uuid.UUID) (*Question, error)
	DeleteQuestion(ctx context.Context, id uuid.UUID, callerID uuid.UUID) error

	GetOption(ctx context.Context, id uuid.UUID) (*QuestionOption, error)
	CreateOption(ctx context.Context, questionID uuid.UUID, req CreateOptionRequest, callerID uuid.UUID) (*QuestionOption, error)
	ListOptions(ctx context.Context, questionID uuid.UUID) ([]QuestionOption, error)
}

type questionService struct {
	repo       Repository
	examReader ExamReader
	logger     *zerolog.Logger
}

func NewService(repo Repository, examReader ExamReader, logger *zerolog.Logger) Service {
	return &questionService{
		repo:       repo,
		examReader: examReader,
		logger:     logger,
	}
}

func (s *questionService) GetQuestion(ctx context.Context, id uuid.UUID) (*Question, error) {
	return s.repo.GetByIDWithOptions(ctx, id)
}

func (s *questionService) ListQuestionsByExam(ctx context.Context, examID uuid.UUID) ([]Question, error) {
	return s.repo.ListByExamID(ctx, examID)
}

func (s *questionService) CreateQuestion(ctx context.Context, examID uuid.UUID, req CreateQuestionRequest, callerID uuid.UUID) (*Question, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	ex, err := s.examReader.GetExam(ctx, examID)
	if err != nil {
		return nil, err
	}
	if ex == nil {
		return nil, exam.ErrExamNotFound
	}

	if callerID != uuid.Nil && ex.CreatedBy != callerID {
		return nil, ErrUnauthorizedCreator
	}

	if ex.Status == exam.StatusArchived {
		return nil, ErrExamArchived
	}
	if ex.Status != exam.StatusDraft {
		return nil, ErrExamNotDraft
	}

	now := time.Now().UTC()
	q := &Question{
		Base: model.Base{
			BaseWithId:        model.BaseWithId{ID: uuid.New()},
			BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
			BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
		},
		ExamID:         examID,
		QuestionNumber: req.QuestionNumber,
		Text:           req.Text,
		Type:           req.Type,
		Points:         req.Points,
	}

	if err := s.repo.Create(ctx, q); err != nil {
		s.logger.Error().Err(err).Str("exam_id", examID.String()).Msg("failed to create question")
		return nil, err
	}

	s.logger.Info().
		Str("event", "question.created").
		Str("question_id", q.ID.String()).
		Str("exam_id", examID.String()).
		Msg("question created successfully")

	return q, nil
}

func (s *questionService) UpdateQuestion(ctx context.Context, id uuid.UUID, req UpdateQuestionRequest, callerID uuid.UUID) (*Question, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	q, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, ErrQuestionNotFound
	}

	ex, err := s.examReader.GetExam(ctx, q.ExamID)
	if err != nil {
		return nil, err
	}
	if ex == nil {
		return nil, exam.ErrExamNotFound
	}

	if callerID != uuid.Nil && ex.CreatedBy != callerID {
		return nil, ErrUnauthorizedCreator
	}

	if ex.Status != exam.StatusDraft {
		return nil, ErrExamNotDraft
	}

	if req.QuestionNumber != nil {
		q.QuestionNumber = *req.QuestionNumber
	}
	if req.Text != nil {
		q.Text = *req.Text
	}
	if req.Points != nil {
		q.Points = *req.Points
	}

	if err := s.repo.Update(ctx, q); err != nil {
		s.logger.Error().Err(err).Str("question_id", id.String()).Msg("failed to update question")
		return nil, err
	}

	s.logger.Info().
		Str("event", "question.updated").
		Str("question_id", id.String()).
		Msg("question updated successfully")

	return q, nil
}

func (s *questionService) DeleteQuestion(ctx context.Context, id uuid.UUID, callerID uuid.UUID) error {
	q, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if q == nil {
		return ErrQuestionNotFound
	}

	ex, err := s.examReader.GetExam(ctx, q.ExamID)
	if err != nil {
		return err
	}
	if ex == nil {
		return exam.ErrExamNotFound
	}

	if callerID != uuid.Nil && ex.CreatedBy != callerID {
		return ErrUnauthorizedCreator
	}

	if ex.Status != exam.StatusDraft {
		return ErrExamNotDraft
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		s.logger.Error().Err(err).Str("question_id", id.String()).Msg("failed to delete question")
		return err
	}

	s.logger.Info().
		Str("event", "question.deleted").
		Str("question_id", id.String()).
		Msg("question deleted successfully")

	return nil
}

func (s *questionService) CreateOption(ctx context.Context, questionID uuid.UUID, req CreateOptionRequest, callerID uuid.UUID) (*QuestionOption, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	q, err := s.repo.GetByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, ErrQuestionNotFound
	}

	if q.Type != TypeMCQ {
		return nil, ErrOptionsOnlyForMCQ
	}

	ex, err := s.examReader.GetExam(ctx, q.ExamID)
	if err != nil {
		return nil, err
	}
	if ex == nil {
		return nil, exam.ErrExamNotFound
	}

	if callerID != uuid.Nil && ex.CreatedBy != callerID {
		return nil, ErrUnauthorizedCreator
	}

	if ex.Status != exam.StatusDraft {
		return nil, ErrExamNotDraft
	}

	existingOpts, err := s.repo.ListOptionsByQuestionID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	for _, opt := range existingOpts {
		if strings.EqualFold(opt.OptionKey, req.OptionKey) {
			return nil, ErrOptionKeyDuplicate
		}
	}

	now := time.Now().UTC()
	opt := &QuestionOption{
		Base: model.Base{
			BaseWithId:        model.BaseWithId{ID: uuid.New()},
			BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
			BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
		},
		QuestionID:   questionID,
		OptionKey:    strings.ToUpper(req.OptionKey),
		OptionText:   req.OptionText,
		DisplayOrder: req.DisplayOrder,
		IsCorrect:    req.IsCorrect,
	}

	if err := s.repo.CreateOption(ctx, opt); err != nil {
		s.logger.Error().Err(err).Str("question_id", questionID.String()).Msg("failed to create question option")
		return nil, err
	}

	s.logger.Info().
		Str("event", "question_option.created").
		Str("option_id", opt.ID.String()).
		Str("question_id", questionID.String()).
		Msg("question option created successfully")

	return opt, nil
}

func (s *questionService) GetOption(ctx context.Context, id uuid.UUID) (*QuestionOption, error) {
	return s.repo.GetOptionByID(ctx, id)
}

func (s *questionService) ListOptions(ctx context.Context, questionID uuid.UUID) ([]QuestionOption, error) {
	return s.repo.ListOptionsByQuestionID(ctx, questionID)
}
