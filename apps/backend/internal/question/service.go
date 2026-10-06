package question

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/model"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
)

type ExamReader interface {
	GetExam(ctx context.Context, id uuid.UUID) (*exam.Exam, error)
}

type Service interface {
	GetQuestion(ctx context.Context, id uuid.UUID) (*Question, error)
	GetQuestionForCaller(ctx context.Context, id uuid.UUID, caller Caller) (*Question, error)
	ListQuestionsByExam(ctx context.Context, examID uuid.UUID) ([]Question, error)
	ListQuestionsByExamForCaller(ctx context.Context, examID uuid.UUID, caller Caller) ([]Question, error)
	CreateQuestion(ctx context.Context, examID uuid.UUID, req CreateQuestionRequest, caller Caller) (*Question, error)
	UpdateQuestion(ctx context.Context, id uuid.UUID, req UpdateQuestionRequest, caller Caller) (*Question, error)
	DeleteQuestion(ctx context.Context, id uuid.UUID, caller Caller) error
	ReorderQuestions(ctx context.Context, examID uuid.UUID, req ReorderQuestionsRequest, caller Caller) ([]Question, error)

	GetOption(ctx context.Context, id uuid.UUID) (*QuestionOption, error)
	GetOptionForCaller(ctx context.Context, questionID, optionID uuid.UUID, caller Caller) (*QuestionOption, error)
	ListOptions(ctx context.Context, questionID uuid.UUID) ([]QuestionOption, error)
	ListOptionsForCaller(ctx context.Context, questionID uuid.UUID, caller Caller) ([]QuestionOption, error)
	CreateOption(ctx context.Context, questionID uuid.UUID, req CreateOptionRequest, caller Caller) (*QuestionOption, error)
	UpdateOption(ctx context.Context, questionID, optionID uuid.UUID, req UpdateOptionRequest, caller Caller) (*QuestionOption, error)
	DeleteOption(ctx context.Context, questionID, optionID uuid.UUID, caller Caller) error
	SetCorrectOption(ctx context.Context, questionID, optionID uuid.UUID, caller Caller) (*QuestionOption, error)
}

type nopTxManager struct{}

func (n *nopTxManager) WithTx(ctx context.Context, fn func(database.DBTX) error) error {
	return fn(nil)
}

type questionService struct {
	repo       Repository
	examReader ExamReader
	txManager  database.TxManager
	logger     *zerolog.Logger
}

func NewService(repo Repository, examReader ExamReader, txManager database.TxManager, logger *zerolog.Logger) Service {
	if txManager == nil {
		txManager = &nopTxManager{}
	}
	return &questionService{
		repo:       repo,
		examReader: examReader,
		txManager:  txManager,
		logger:     logger,
	}
}

func (s *questionService) validateExamForModification(ctx context.Context, examID uuid.UUID, caller Caller) (*exam.Exam, error) {
	ex, err := s.examReader.GetExam(ctx, examID)
	if err != nil {
		return nil, err
	}
	if ex == nil {
		return nil, exam.ErrExamNotFound
	}

	if !caller.CanManage(ex.CreatedBy) {
		return nil, ErrUnauthorizedCreator
	}

	if ex.Status == exam.StatusArchived {
		return nil, ErrExamArchived
	}
	if ex.Status != exam.StatusDraft {
		return nil, ErrExamNotDraft
	}

	return ex, nil
}

func (s *questionService) GetQuestion(ctx context.Context, id uuid.UUID) (*Question, error) {
	return s.repo.GetByIDWithOptions(ctx, id)
}

func (s *questionService) GetQuestionForCaller(ctx context.Context, id uuid.UUID, caller Caller) (*Question, error) {
	q, err := s.repo.GetByIDWithOptions(ctx, id)
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

	// If exam is draft, students cannot view questions
	if caller.IsStudent() && ex.Status == exam.StatusDraft {
		return nil, exam.ErrExamNotFound
	}

	return q, nil
}

func (s *questionService) ListQuestionsByExam(ctx context.Context, examID uuid.UUID) ([]Question, error) {
	return s.repo.ListByExamID(ctx, examID)
}

func (s *questionService) ListQuestionsByExamForCaller(ctx context.Context, examID uuid.UUID, caller Caller) ([]Question, error) {
	ex, err := s.examReader.GetExam(ctx, examID)
	if err != nil {
		return nil, err
	}
	if ex == nil {
		return nil, exam.ErrExamNotFound
	}

	if caller.IsStudent() && ex.Status == exam.StatusDraft {
		return nil, exam.ErrExamNotFound
	}

	return s.repo.ListByExamID(ctx, examID)
}

func (s *questionService) CreateQuestion(ctx context.Context, examID uuid.UUID, req CreateQuestionRequest, caller Caller) (*Question, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	if _, err := s.validateExamForModification(ctx, examID, caller); err != nil {
		return nil, err
	}

	// Validate options if provided
	if len(req.Options) > 0 {
		if req.Type != TypeMCQ {
			return nil, ErrOptionsOnlyForMCQ
		}

		keyMap := make(map[string]bool)
		correctCount := 0
		for _, opt := range req.Options {
			upperKey := strings.ToUpper(strings.TrimSpace(opt.OptionKey))
			if keyMap[upperKey] {
				return nil, ErrOptionKeyDuplicate
			}
			keyMap[upperKey] = true
			if opt.IsCorrect {
				correctCount++
			}
		}

		if correctCount > 1 {
			return nil, ErrMultipleCorrectOptions
		}
	}

	// Assign question number if omitted or <= 0
	qNum := req.QuestionNumber
	if qNum <= 0 {
		maxNum, err := s.repo.GetMaxQuestionNumber(ctx, examID)
		if err != nil {
			return nil, err
		}
		qNum = maxNum + 1
	}

	now := time.Now().UTC()
	q := &Question{
		Base: model.Base{
			BaseWithId:        model.BaseWithId{ID: uuid.New()},
			BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
			BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
		},
		ExamID:         examID,
		QuestionNumber: qNum,
		Text:           req.Text,
		Type:           req.Type,
		Points:         req.Points,
	}

	err := s.txManager.WithTx(ctx, func(tx database.DBTX) error {
		if err := s.repo.Create(ctx, q, tx); err != nil {
			return err
		}

		if len(req.Options) > 0 {
			for i, optReq := range req.Options {
				displayOrder := optReq.DisplayOrder
				if displayOrder <= 0 {
					displayOrder = i + 1
				}
				opt := &QuestionOption{
					Base: model.Base{
						BaseWithId:        model.BaseWithId{ID: uuid.New()},
						BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
						BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
					},
					QuestionID:   q.ID,
					OptionKey:    strings.ToUpper(strings.TrimSpace(optReq.OptionKey)),
					OptionText:   optReq.OptionText,
					DisplayOrder: displayOrder,
					IsCorrect:    optReq.IsCorrect,
				}
				if err := s.repo.CreateOption(ctx, opt, tx); err != nil {
					return err
				}
				q.Options = append(q.Options, *opt)
			}
		}
		return nil
	})

	if err != nil {
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

func (s *questionService) UpdateQuestion(ctx context.Context, id uuid.UUID, req UpdateQuestionRequest, caller Caller) (*Question, error) {
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

	if _, err := s.validateExamForModification(ctx, q.ExamID, caller); err != nil {
		return nil, err
	}

	if req.QuestionNumber != nil && *req.QuestionNumber != q.QuestionNumber {
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

	return s.repo.GetByIDWithOptions(ctx, id)
}

func (s *questionService) DeleteQuestion(ctx context.Context, id uuid.UUID, caller Caller) error {
	q, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if q == nil {
		return ErrQuestionNotFound
	}

	if _, err := s.validateExamForModification(ctx, q.ExamID, caller); err != nil {
		return err
	}

	err = s.txManager.WithTx(ctx, func(tx database.DBTX) error {
		if err := s.repo.Delete(ctx, id, tx); err != nil {
			return err
		}
		// Resequence remaining questions so question numbers remain contiguous without gaps
		return s.repo.ResequenceQuestions(ctx, q.ExamID, tx)
	})

	if err != nil {
		s.logger.Error().Err(err).Str("question_id", id.String()).Msg("failed to delete question")
		return err
	}

	s.logger.Info().
		Str("event", "question.deleted").
		Str("question_id", id.String()).
		Str("exam_id", q.ExamID.String()).
		Msg("question deleted and re-sequenced successfully")

	return nil
}

func (s *questionService) ReorderQuestions(ctx context.Context, examID uuid.UUID, req ReorderQuestionsRequest, caller Caller) ([]Question, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	if _, err := s.validateExamForModification(ctx, examID, caller); err != nil {
		return nil, err
	}

	existingQuestions, err := s.repo.ListByExamID(ctx, examID)
	if err != nil {
		return nil, err
	}

	existingMap := make(map[uuid.UUID]*Question, len(existingQuestions))
	for i := range existingQuestions {
		existingMap[existingQuestions[i].ID] = &existingQuestions[i]
	}

	orderMap := make(map[uuid.UUID]int)

	if len(req.Orders) > 0 {
		seenNumbers := make(map[int]bool)
		for _, item := range req.Orders {
			if _, ok := existingMap[item.ID]; !ok {
				return nil, ErrQuestionDoesNotBelongToExam
			}
			if seenNumbers[item.QuestionNumber] {
				return nil, ErrDuplicateQuestionNumbers
			}
			seenNumbers[item.QuestionNumber] = true
			orderMap[item.ID] = item.QuestionNumber
		}
	} else if len(req.QuestionIDs) > 0 {
		if len(req.QuestionIDs) != len(existingQuestions) {
			return nil, ErrInvalidQuestionOrder
		}
		seenIDs := make(map[uuid.UUID]bool)
		for i, qID := range req.QuestionIDs {
			if _, ok := existingMap[qID]; !ok {
				return nil, ErrQuestionDoesNotBelongToExam
			}
			if seenIDs[qID] {
				return nil, ErrDuplicateQuestionNumbers
			}
			seenIDs[qID] = true
			orderMap[qID] = i + 1
		}
	} else {
		return nil, ErrInvalidQuestionOrder
	}

	err = s.txManager.WithTx(ctx, func(tx database.DBTX) error {
		return s.repo.ReorderQuestions(ctx, examID, orderMap, tx)
	})

	if err != nil {
		s.logger.Error().Err(err).Str("exam_id", examID.String()).Msg("failed to reorder questions")
		return nil, err
	}

	s.logger.Info().
		Str("event", "questions.reordered").
		Str("exam_id", examID.String()).
		Msg("questions reordered successfully")

	return s.repo.ListByExamID(ctx, examID)
}

func (s *questionService) GetOption(ctx context.Context, id uuid.UUID) (*QuestionOption, error) {
	return s.repo.GetOptionByID(ctx, id)
}

func (s *questionService) GetOptionForCaller(ctx context.Context, questionID, optionID uuid.UUID, caller Caller) (*QuestionOption, error) {
	q, err := s.repo.GetByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, ErrQuestionNotFound
	}

	opt, err := s.repo.GetOptionByID(ctx, optionID)
	if err != nil {
		return nil, err
	}
	if opt == nil || opt.QuestionID != questionID {
		return nil, ErrOptionNotFound
	}

	ex, err := s.examReader.GetExam(ctx, q.ExamID)
	if err != nil {
		return nil, err
	}
	if ex == nil {
		return nil, exam.ErrExamNotFound
	}

	if caller.IsStudent() && ex.Status == exam.StatusDraft {
		return nil, exam.ErrExamNotFound
	}

	return opt, nil
}

func (s *questionService) ListOptions(ctx context.Context, questionID uuid.UUID) ([]QuestionOption, error) {
	return s.repo.ListOptionsByQuestionID(ctx, questionID)
}

func (s *questionService) ListOptionsForCaller(ctx context.Context, questionID uuid.UUID, caller Caller) ([]QuestionOption, error) {
	q, err := s.repo.GetByID(ctx, questionID)
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

	if caller.IsStudent() && ex.Status == exam.StatusDraft {
		return nil, exam.ErrExamNotFound
	}

	return s.repo.ListOptionsByQuestionID(ctx, questionID)
}

func (s *questionService) CreateOption(ctx context.Context, questionID uuid.UUID, req CreateOptionRequest, caller Caller) (*QuestionOption, error) {
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

	if _, err := s.validateExamForModification(ctx, q.ExamID, caller); err != nil {
		return nil, err
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

	displayOrder := req.DisplayOrder
	if displayOrder <= 0 {
		displayOrder = len(existingOpts) + 1
	}

	now := time.Now().UTC()
	opt := &QuestionOption{
		Base: model.Base{
			BaseWithId:        model.BaseWithId{ID: uuid.New()},
			BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
			BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
		},
		QuestionID:   questionID,
		OptionKey:    strings.ToUpper(strings.TrimSpace(req.OptionKey)),
		OptionText:   req.OptionText,
		DisplayOrder: displayOrder,
		IsCorrect:    req.IsCorrect,
	}

	err = s.txManager.WithTx(ctx, func(tx database.DBTX) error {
		if opt.IsCorrect {
			// If newly created option is correct, set others to false
			for _, o := range existingOpts {
				if o.IsCorrect {
					o.IsCorrect = false
					if err := s.repo.UpdateOption(ctx, &o, tx); err != nil {
						return err
					}
				}
			}
		}
		return s.repo.CreateOption(ctx, opt, tx)
	})

	if err != nil {
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

func (s *questionService) UpdateOption(ctx context.Context, questionID, optionID uuid.UUID, req UpdateOptionRequest, caller Caller) (*QuestionOption, error) {
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

	if _, err := s.validateExamForModification(ctx, q.ExamID, caller); err != nil {
		return nil, err
	}

	opt, err := s.repo.GetOptionByID(ctx, optionID)
	if err != nil {
		return nil, err
	}
	if opt == nil || opt.QuestionID != questionID {
		return nil, ErrOptionNotFound
	}

	existingOpts, err := s.repo.ListOptionsByQuestionID(ctx, questionID)
	if err != nil {
		return nil, err
	}

	if req.OptionKey != nil {
		newKey := strings.ToUpper(strings.TrimSpace(*req.OptionKey))
		for _, o := range existingOpts {
			if o.ID != optionID && strings.EqualFold(o.OptionKey, newKey) {
				return nil, ErrOptionKeyDuplicate
			}
		}
		opt.OptionKey = newKey
	}

	if req.OptionText != nil {
		opt.OptionText = *req.OptionText
	}
	if req.DisplayOrder != nil {
		opt.DisplayOrder = *req.DisplayOrder
	}
	if req.IsCorrect != nil {
		opt.IsCorrect = *req.IsCorrect
	}

	err = s.txManager.WithTx(ctx, func(tx database.DBTX) error {
		if opt.IsCorrect {
			// Ensure only this option is correct
			for _, o := range existingOpts {
				if o.ID != optionID && o.IsCorrect {
					o.IsCorrect = false
					if err := s.repo.UpdateOption(ctx, &o, tx); err != nil {
						return err
					}
				}
			}
		}
		return s.repo.UpdateOption(ctx, opt, tx)
	})

	if err != nil {
		s.logger.Error().Err(err).Str("option_id", optionID.String()).Msg("failed to update question option")
		return nil, err
	}

	s.logger.Info().
		Str("event", "question_option.updated").
		Str("option_id", optionID.String()).
		Msg("question option updated successfully")

	return opt, nil
}

func (s *questionService) DeleteOption(ctx context.Context, questionID, optionID uuid.UUID, caller Caller) error {
	q, err := s.repo.GetByID(ctx, questionID)
	if err != nil {
		return err
	}
	if q == nil {
		return ErrQuestionNotFound
	}

	if _, err := s.validateExamForModification(ctx, q.ExamID, caller); err != nil {
		return err
	}

	opt, err := s.repo.GetOptionByID(ctx, optionID)
	if err != nil {
		return err
	}
	if opt == nil || opt.QuestionID != questionID {
		return ErrOptionNotFound
	}

	if err := s.repo.DeleteOption(ctx, optionID); err != nil {
		s.logger.Error().Err(err).Str("option_id", optionID.String()).Msg("failed to delete question option")
		return err
	}

	s.logger.Info().
		Str("event", "question_option.deleted").
		Str("option_id", optionID.String()).
		Msg("question option deleted successfully")

	return nil
}

func (s *questionService) SetCorrectOption(ctx context.Context, questionID, optionID uuid.UUID, caller Caller) (*QuestionOption, error) {
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

	if _, err := s.validateExamForModification(ctx, q.ExamID, caller); err != nil {
		return nil, err
	}

	opt, err := s.repo.GetOptionByID(ctx, optionID)
	if err != nil {
		return nil, err
	}
	if opt == nil || opt.QuestionID != questionID {
		return nil, ErrOptionNotFound
	}

	err = s.txManager.WithTx(ctx, func(tx database.DBTX) error {
		return s.repo.SetCorrectOption(ctx, questionID, optionID, tx)
	})

	if err != nil {
		s.logger.Error().Err(err).Str("option_id", optionID.String()).Msg("failed to set correct option")
		return nil, err
	}

	s.logger.Info().
		Str("event", "question_option.set_correct").
		Str("option_id", optionID.String()).
		Str("question_id", questionID.String()).
		Msg("correct option set successfully")

	return s.repo.GetOptionByID(ctx, optionID)
}
