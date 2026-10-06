package question

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
)

type mockQuestionRepo struct {
	questions map[uuid.UUID]*Question
	options   map[uuid.UUID]*QuestionOption
}

func newMockQuestionRepo() *mockQuestionRepo {
	return &mockQuestionRepo{
		questions: make(map[uuid.UUID]*Question),
		options:   make(map[uuid.UUID]*QuestionOption),
	}
}

func (m *mockQuestionRepo) GetByID(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (*Question, error) {
	if q, ok := m.questions[id]; ok {
		copy := *q
		return &copy, nil
	}
	return nil, nil
}

func (m *mockQuestionRepo) GetByIDWithOptions(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (*Question, error) {
	q, err := m.GetByID(ctx, id, tx...)
	if err != nil || q == nil {
		return q, err
	}
	opts, _ := m.ListOptionsByQuestionID(ctx, id, tx...)
	q.Options = opts
	return q, nil
}

func (m *mockQuestionRepo) ListByExamID(ctx context.Context, examID uuid.UUID, tx ...database.DBTX) ([]Question, error) {
	res := make([]Question, 0)
	for _, q := range m.questions {
		if q.ExamID == examID {
			copy := *q
			opts, _ := m.ListOptionsByQuestionID(ctx, q.ID, tx...)
			copy.Options = opts
			res = append(res, copy)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].QuestionNumber < res[j].QuestionNumber
	})
	return res, nil
}

func (m *mockQuestionRepo) Create(ctx context.Context, q *Question, tx ...database.DBTX) error {
	m.questions[q.ID] = q
	return nil
}

func (m *mockQuestionRepo) Update(ctx context.Context, q *Question, tx ...database.DBTX) error {
	m.questions[q.ID] = q
	return nil
}

func (m *mockQuestionRepo) Delete(ctx context.Context, id uuid.UUID, tx ...database.DBTX) error {
	delete(m.questions, id)
	for optID, opt := range m.options {
		if opt.QuestionID == id {
			delete(m.options, optID)
		}
	}
	return nil
}

func (m *mockQuestionRepo) GetOptionByID(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (*QuestionOption, error) {
	if opt, ok := m.options[id]; ok {
		copy := *opt
		return &copy, nil
	}
	return nil, nil
}

func (m *mockQuestionRepo) ListOptionsByQuestionID(ctx context.Context, questionID uuid.UUID, tx ...database.DBTX) ([]QuestionOption, error) {
	res := make([]QuestionOption, 0)
	for _, opt := range m.options {
		if opt.QuestionID == questionID {
			res = append(res, *opt)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].DisplayOrder == res[j].DisplayOrder {
			return res[i].OptionKey < res[j].OptionKey
		}
		return res[i].DisplayOrder < res[j].DisplayOrder
	})
	return res, nil
}

func (m *mockQuestionRepo) CreateOption(ctx context.Context, opt *QuestionOption, tx ...database.DBTX) error {
	m.options[opt.ID] = opt
	return nil
}

func (m *mockQuestionRepo) UpdateOption(ctx context.Context, opt *QuestionOption, tx ...database.DBTX) error {
	m.options[opt.ID] = opt
	return nil
}

func (m *mockQuestionRepo) DeleteOption(ctx context.Context, id uuid.UUID, tx ...database.DBTX) error {
	delete(m.options, id)
	return nil
}

func (m *mockQuestionRepo) SetCorrectOption(ctx context.Context, questionID, optionID uuid.UUID, tx ...database.DBTX) error {
	for _, opt := range m.options {
		if opt.QuestionID == questionID {
			opt.IsCorrect = (opt.ID == optionID)
		}
	}
	return nil
}

func (m *mockQuestionRepo) GetMaxQuestionNumber(ctx context.Context, examID uuid.UUID, tx ...database.DBTX) (int, error) {
	maxNum := 0
	for _, q := range m.questions {
		if q.ExamID == examID && q.QuestionNumber > maxNum {
			maxNum = q.QuestionNumber
		}
	}
	return maxNum, nil
}

func (m *mockQuestionRepo) ReorderQuestions(ctx context.Context, examID uuid.UUID, orderMap map[uuid.UUID]int, tx ...database.DBTX) error {
	for id, num := range orderMap {
		if q, ok := m.questions[id]; ok {
			q.QuestionNumber = num
		}
	}
	return nil
}

func (m *mockQuestionRepo) ResequenceQuestions(ctx context.Context, examID uuid.UUID, tx ...database.DBTX) error {
	list, err := m.ListByExamID(ctx, examID, tx...)
	if err != nil {
		return err
	}
	for i, q := range list {
		if existing, ok := m.questions[q.ID]; ok {
			existing.QuestionNumber = i + 1
		}
	}
	return nil
}

type mockTxManager struct {
	failOnTx bool
}

func (m *mockTxManager) WithTx(ctx context.Context, fn func(database.DBTX) error) error {
	if m.failOnTx {
		return errors.New("simulated tx failure")
	}
	return fn(nil)
}

type mockExamReader struct {
	exams map[uuid.UUID]*exam.Exam
}

func (m *mockExamReader) GetExam(ctx context.Context, id uuid.UUID) (*exam.Exam, error) {
	if e, ok := m.exams[id]; ok {
		return e, nil
	}
	return nil, exam.ErrExamNotFound
}

func TestQuestionService_CreateQuestion(t *testing.T) {
	repo := newMockQuestionRepo()
	logger := zerolog.Nop()
	creatorID := uuid.New()
	adminID := uuid.New()
	examID := uuid.New()

	examReader := &mockExamReader{
		exams: map[uuid.UUID]*exam.Exam{
			examID: {
				Title:     "Physics Exam",
				Status:    exam.StatusDraft,
				CreatedBy: creatorID,
			},
		},
	}
	txMgr := &mockTxManager{}
	svc := NewService(repo, examReader, txMgr, &logger)
	ctx := context.Background()

	creatorCaller := Caller{ID: creatorID, Role: platformauth.RoleTeacher}
	adminCaller := Caller{ID: adminID, Role: platformauth.RoleAdmin}
	otherTeacher := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}

	t.Run("success on DRAFT exam by creator", func(t *testing.T) {
		req := CreateQuestionRequest{
			QuestionNumber: 1,
			Text:           "What is Newton's second law?",
			Type:           TypeEssay,
			Points:         5,
		}
		created, err := svc.CreateQuestion(ctx, examID, req, creatorCaller)
		require.NoError(t, err)
		assert.Equal(t, 1, created.QuestionNumber)
		assert.Equal(t, TypeEssay, created.Type)
	})

	t.Run("success by admin", func(t *testing.T) {
		req := CreateQuestionRequest{
			QuestionNumber: 2,
			Text:           "Explain force and acceleration.",
			Type:           TypeEssay,
			Points:         5,
		}
		created, err := svc.CreateQuestion(ctx, examID, req, adminCaller)
		require.NoError(t, err)
		assert.Equal(t, 2, created.QuestionNumber)
	})

	t.Run("auto-calculates question number if omitted", func(t *testing.T) {
		req := CreateQuestionRequest{
			Text:   "Describe gravitational pull.",
			Type:   TypeEssay,
			Points: 3,
		}
		created, err := svc.CreateQuestion(ctx, examID, req, creatorCaller)
		require.NoError(t, err)
		assert.Equal(t, 3, created.QuestionNumber)
	})

	t.Run("atomic creation with MCQ options", func(t *testing.T) {
		req := CreateQuestionRequest{
			Text:   "What is the SI unit of force?",
			Type:   TypeMCQ,
			Points: 2,
			Options: []CreateOptionRequest{
				{OptionKey: "A", OptionText: "Joule", DisplayOrder: 1, IsCorrect: false},
				{OptionKey: "B", OptionText: "Newton", DisplayOrder: 2, IsCorrect: true},
			},
		}
		created, err := svc.CreateQuestion(ctx, examID, req, creatorCaller)
		require.NoError(t, err)
		assert.Equal(t, 4, created.QuestionNumber)
		assert.Len(t, created.Options, 2)
		assert.Equal(t, "A", created.Options[0].OptionKey)
		assert.Equal(t, "Newton", created.Options[1].OptionText)
		assert.True(t, created.Options[1].IsCorrect)
	})

	t.Run("rejects if non-creator teacher attempts modification", func(t *testing.T) {
		req := CreateQuestionRequest{
			Text:   "Unauthorized question?",
			Type:   TypeEssay,
			Points: 1,
		}
		_, err := svc.CreateQuestion(ctx, examID, req, otherTeacher)
		assert.ErrorIs(t, err, ErrUnauthorizedCreator)
	})

	t.Run("rejects if exam is PUBLISHED", func(t *testing.T) {
		examReader.exams[examID].Status = exam.StatusPublished
		req := CreateQuestionRequest{
			Text:   "Published exam question?",
			Type:   TypeEssay,
			Points: 1,
		}
		_, err := svc.CreateQuestion(ctx, examID, req, creatorCaller)
		assert.ErrorIs(t, err, ErrExamNotDraft)
	})

	t.Run("rejects if exam is ARCHIVED", func(t *testing.T) {
		examReader.exams[examID].Status = exam.StatusArchived
		req := CreateQuestionRequest{
			Text:   "Archived exam question?",
			Type:   TypeEssay,
			Points: 1,
		}
		_, err := svc.CreateQuestion(ctx, examID, req, creatorCaller)
		assert.ErrorIs(t, err, ErrExamArchived)
	})
}

func TestQuestionService_ReorderAndResequence(t *testing.T) {
	repo := newMockQuestionRepo()
	logger := zerolog.Nop()
	creatorID := uuid.New()
	examID := uuid.New()

	examReader := &mockExamReader{
		exams: map[uuid.UUID]*exam.Exam{
			examID: {
				Title:     "Chemistry Exam",
				Status:    exam.StatusDraft,
				CreatedBy: creatorID,
			},
		},
	}
	txMgr := &mockTxManager{}
	svc := NewService(repo, examReader, txMgr, &logger)
	ctx := context.Background()
	caller := Caller{ID: creatorID, Role: platformauth.RoleTeacher}

	// Create 3 questions
	q1, err := svc.CreateQuestion(ctx, examID, CreateQuestionRequest{Text: "Question 1", Type: TypeEssay, Points: 1}, caller)
	require.NoError(t, err)
	q2, err := svc.CreateQuestion(ctx, examID, CreateQuestionRequest{Text: "Question 2", Type: TypeEssay, Points: 1}, caller)
	require.NoError(t, err)
	q3, err := svc.CreateQuestion(ctx, examID, CreateQuestionRequest{Text: "Question 3", Type: TypeEssay, Points: 1}, caller)
	require.NoError(t, err)

	assert.Equal(t, 1, q1.QuestionNumber)
	assert.Equal(t, 2, q2.QuestionNumber)
	assert.Equal(t, 3, q3.QuestionNumber)

	t.Run("reorders questions using QuestionIDs list", func(t *testing.T) {
		// Reverse order: q3, q2, q1
		reordered, err := svc.ReorderQuestions(ctx, examID, ReorderQuestionsRequest{
			QuestionIDs: []uuid.UUID{q3.ID, q2.ID, q1.ID},
		}, caller)
		require.NoError(t, err)
		require.Len(t, reordered, 3)
		assert.Equal(t, q3.ID, reordered[0].ID)
		assert.Equal(t, 1, reordered[0].QuestionNumber)
		assert.Equal(t, q2.ID, reordered[1].ID)
		assert.Equal(t, 2, reordered[1].QuestionNumber)
		assert.Equal(t, q1.ID, reordered[2].ID)
		assert.Equal(t, 3, reordered[2].QuestionNumber)
	})

	t.Run("reorders questions using explicit Orders", func(t *testing.T) {
		reordered, err := svc.ReorderQuestions(ctx, examID, ReorderQuestionsRequest{
			Orders: []QuestionOrderItem{
				{ID: q1.ID, QuestionNumber: 1},
				{ID: q2.ID, QuestionNumber: 2},
				{ID: q3.ID, QuestionNumber: 3},
			},
		}, caller)
		require.NoError(t, err)
		assert.Equal(t, q1.ID, reordered[0].ID)
		assert.Equal(t, 1, reordered[0].QuestionNumber)
	})

	t.Run("rejects reordering if foreign question included", func(t *testing.T) {
		_, err := svc.ReorderQuestions(ctx, examID, ReorderQuestionsRequest{
			QuestionIDs: []uuid.UUID{q1.ID, q2.ID, uuid.New()},
		}, caller)
		assert.ErrorIs(t, err, ErrQuestionDoesNotBelongToExam)
	})

	t.Run("re-sequences questions on deletion", func(t *testing.T) {
		// Delete middle question q2
		err := svc.DeleteQuestion(ctx, q2.ID, caller)
		require.NoError(t, err)

		// Remaining questions: q1 and q3 should be resequenced to 1 and 2
		remaining, err := svc.ListQuestionsByExam(ctx, examID)
		require.NoError(t, err)
		require.Len(t, remaining, 2)
		assert.Equal(t, q1.ID, remaining[0].ID)
		assert.Equal(t, 1, remaining[0].QuestionNumber)
		assert.Equal(t, q3.ID, remaining[1].ID)
		assert.Equal(t, 2, remaining[1].QuestionNumber)
	})
}

func TestQuestionService_OptionsManagement(t *testing.T) {
	repo := newMockQuestionRepo()
	logger := zerolog.Nop()
	creatorID := uuid.New()
	examID := uuid.New()

	examReader := &mockExamReader{
		exams: map[uuid.UUID]*exam.Exam{
			examID: {
				Title:     "Biology Exam",
				Status:    exam.StatusDraft,
				CreatedBy: creatorID,
			},
		},
	}
	txMgr := &mockTxManager{}
	svc := NewService(repo, examReader, txMgr, &logger)
	ctx := context.Background()
	caller := Caller{ID: creatorID, Role: platformauth.RoleTeacher}

	mcq, err := svc.CreateQuestion(ctx, examID, CreateQuestionRequest{
		Text:   "What is the powerhouse of the cell?",
		Type:   TypeMCQ,
		Points: 2,
	}, caller)
	require.NoError(t, err)

	essay, err := svc.CreateQuestion(ctx, examID, CreateQuestionRequest{
		Text:   "Describe photosynthesis in detail.",
		Type:   TypeEssay,
		Points: 5,
	}, caller)
	require.NoError(t, err)

	t.Run("create option for MCQ succeeds", func(t *testing.T) {
		optA, err := svc.CreateOption(ctx, mcq.ID, CreateOptionRequest{
			OptionKey:  "A",
			OptionText: "Mitochondria",
			IsCorrect:  true,
		}, caller)
		require.NoError(t, err)
		assert.Equal(t, "A", optA.OptionKey)
		assert.True(t, optA.IsCorrect)
	})

	t.Run("duplicate option key fails", func(t *testing.T) {
		_, err := svc.CreateOption(ctx, mcq.ID, CreateOptionRequest{
			OptionKey:  "a", // case-insensitive check
			OptionText: "Duplicate A",
		}, caller)
		assert.ErrorIs(t, err, ErrOptionKeyDuplicate)
	})

	t.Run("cannot add option to non-MCQ", func(t *testing.T) {
		_, err := svc.CreateOption(ctx, essay.ID, CreateOptionRequest{
			OptionKey:  "A",
			OptionText: "Invalid",
		}, caller)
		assert.ErrorIs(t, err, ErrOptionsOnlyForMCQ)
	})

	t.Run("set correct option switches previous correct to false", func(t *testing.T) {
		optB, err := svc.CreateOption(ctx, mcq.ID, CreateOptionRequest{
			OptionKey:  "B",
			OptionText: "Ribosome",
			IsCorrect:  false,
		}, caller)
		require.NoError(t, err)
		assert.False(t, optB.IsCorrect)

		// Set B as correct option
		updatedB, err := svc.SetCorrectOption(ctx, mcq.ID, optB.ID, caller)
		require.NoError(t, err)
		assert.True(t, updatedB.IsCorrect)

		// Verify A is now false
		opts, err := svc.ListOptions(ctx, mcq.ID)
		require.NoError(t, err)
		require.Len(t, opts, 2)
		for _, o := range opts {
			if o.OptionKey == "B" {
				assert.True(t, o.IsCorrect)
			} else {
				assert.False(t, o.IsCorrect)
			}
		}
	})

	t.Run("update option details", func(t *testing.T) {
		opts, err := svc.ListOptions(ctx, mcq.ID)
		require.NoError(t, err)
		targetOpt := opts[0]

		newText := "Updated Option Text"
		updated, err := svc.UpdateOption(ctx, mcq.ID, targetOpt.ID, UpdateOptionRequest{
			OptionText: &newText,
		}, caller)
		require.NoError(t, err)
		assert.Equal(t, newText, updated.OptionText)
	})

	t.Run("delete option succeeds", func(t *testing.T) {
		opts, err := svc.ListOptions(ctx, mcq.ID)
		require.NoError(t, err)
		toDelete := opts[1]

		err = svc.DeleteOption(ctx, mcq.ID, toDelete.ID, caller)
		require.NoError(t, err)

		remaining, err := svc.ListOptions(ctx, mcq.ID)
		require.NoError(t, err)
		assert.Len(t, remaining, 1)
	})
}

func TestQuestionService_StudentAccessAndVisibility(t *testing.T) {
	repo := newMockQuestionRepo()
	logger := zerolog.Nop()
	creatorID := uuid.New()
	studentID := uuid.New()
	examID := uuid.New()

	examReader := &mockExamReader{
		exams: map[uuid.UUID]*exam.Exam{
			examID: {
				Title:     "History Exam",
				Status:    exam.StatusDraft,
				CreatedBy: creatorID,
			},
		},
	}
	txMgr := &mockTxManager{}
	svc := NewService(repo, examReader, txMgr, &logger)
	ctx := context.Background()

	teacherCaller := Caller{ID: creatorID, Role: platformauth.RoleTeacher}
	studentCaller := Caller{ID: studentID, Role: platformauth.RoleStudent}

	q, err := svc.CreateQuestion(ctx, examID, CreateQuestionRequest{
		Text:   "Who was the first president?",
		Type:   TypeMCQ,
		Points: 2,
		Options: []CreateOptionRequest{
			{OptionKey: "A", OptionText: "George Washington", IsCorrect: true},
			{OptionKey: "B", OptionText: "Abraham Lincoln", IsCorrect: false},
		},
	}, teacherCaller)
	require.NoError(t, err)

	t.Run("students cannot view draft exam questions", func(t *testing.T) {
		_, err := svc.GetQuestionForCaller(ctx, q.ID, studentCaller)
		assert.ErrorIs(t, err, exam.ErrExamNotFound)

		_, err = svc.ListQuestionsByExamForCaller(ctx, examID, studentCaller)
		assert.ErrorIs(t, err, exam.ErrExamNotFound)
	})

	t.Run("students can view questions once published", func(t *testing.T) {
		examReader.exams[examID].Status = exam.StatusPublished

		viewedQ, err := svc.GetQuestionForCaller(ctx, q.ID, studentCaller)
		require.NoError(t, err)
		assert.Equal(t, q.ID, viewedQ.ID)

		// Convert to Student DTO to verify answers are stripped
		studentResp := ToStudentQuestionResponse(viewedQ)
		require.Len(t, studentResp.Options, 2)
		assert.Equal(t, "A", studentResp.Options[0].OptionKey)
		assert.Equal(t, "George Washington", studentResp.Options[0].OptionText)
	})
}
