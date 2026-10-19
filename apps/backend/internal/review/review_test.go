package review

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/model"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
)

type fakeRepo struct {
	page      *StudentPage
	responses *Responses
	file      *RecordingFile
	history   *RevisionHistory
	err       error

	gotParams          ListParams
	gotAllowInProgress bool
}

func (f *fakeRepo) ListStudents(_ context.Context, p ListParams) (*StudentPage, error) {
	f.gotParams = p
	return f.page, f.err
}

func (f *fakeRepo) GetRecording(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*RecordingFile, error) {
	return f.file, f.err
}

func (f *fakeRepo) GetRevisions(_ context.Context, _, _, _ uuid.UUID, allow bool) (*RevisionHistory, error) {
	f.gotAllowInProgress = allow
	return f.history, f.err
}

func (f *fakeRepo) GetResponses(_ context.Context, _, _ uuid.UUID, allow bool) (*Responses, error) {
	f.gotAllowInProgress = allow
	return f.responses, f.err
}

type fakeExams struct {
	owner uuid.UUID
	exam  *exam.Exam
}

func (f *fakeExams) GetExamForCaller(_ context.Context, id uuid.UUID, c exam.Caller) (*exam.Exam, error) {
	if f.exam == nil || f.exam.ID != id {
		return nil, exam.ErrExamNotFound
	}
	if !c.CanManage(f.owner) {
		return nil, exam.ErrUnauthorizedCreator
	}
	return f.exam, nil
}

type fixture struct {
	svc     Service
	repo    *fakeRepo
	exams   *fakeExams
	examID  uuid.UUID
	teacher Caller
	other   Caller
	admin   Caller
	student Caller
}

func newFixture(policy Policy) *fixture {
	teacher := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}
	examID := uuid.New()
	ex := &exam.Exam{Title: "Physics", Subject: "Science", DurationMins: 60, Status: exam.StatusPublished}
	ex.Base = model.Base{BaseWithId: model.BaseWithId{ID: examID}}
	ex.CreatedBy = teacher.ID
	repo := &fakeRepo{page: &StudentPage{Rows: []StudentRow{}}}
	exams := &fakeExams{owner: teacher.ID, exam: ex}
	return &fixture{
		svc:     NewService(repo, exams, policy, nil),
		repo:    repo,
		exams:   exams,
		examID:  examID,
		teacher: teacher,
		other:   Caller{ID: uuid.New(), Role: platformauth.RoleTeacher},
		admin:   Caller{ID: uuid.New(), Role: platformauth.RoleAdmin},
		student: Caller{ID: uuid.New(), Role: platformauth.RoleStudent},
	}
}

func TestAuthorization(t *testing.T) {
	f := newFixture(DefaultPolicy())
	ctx := context.Background()
	student := uuid.New()

	cases := []struct {
		name   string
		caller Caller
		want   error
	}{
		{"owner teacher", f.teacher, nil},
		{"administrator", f.admin, nil},
		{"another teacher gets not found", f.other, ErrExamNotFound},
		{"student is forbidden", f.student, ErrForbiddenRole},
		{"proctor is forbidden", Caller{ID: uuid.New(), Role: platformauth.RoleProctor}, ErrForbiddenRole},
		{"missing identity is forbidden", Caller{Role: platformauth.RoleTeacher}, ErrForbiddenRole},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.svc.ListStudents(ctx, f.examID, ListQuery{}, tc.caller)
			if tc.want == nil {
				require.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tc.want)
			}

			f.repo.responses = &Responses{StudentID: student}
			_, err = f.svc.GetStudentResponses(ctx, f.examID, student, tc.caller)
			if tc.want == nil {
				require.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tc.want)
			}
		})
	}

	t.Run("unknown exam looks the same as someone else's exam", func(t *testing.T) {
		_, err := f.svc.ListStudents(ctx, uuid.New(), ListQuery{}, f.teacher)
		assert.ErrorIs(t, err, ErrExamNotFound)
		_, err = f.svc.ListStudents(ctx, f.examID, ListQuery{}, f.other)
		assert.ErrorIs(t, err, ErrExamNotFound)
	})

	t.Run("the repository is never reached when unauthorized", func(t *testing.T) {
		g := newFixture(DefaultPolicy())
		g.repo.page = nil // would panic if dereferenced
		_, err := g.svc.ListStudents(ctx, g.examID, ListQuery{}, g.other)
		assert.ErrorIs(t, err, ErrExamNotFound)
	})
}

func TestListQueryNormalize(t *testing.T) {
	id := uuid.New()

	p, err := ListQuery{}.Normalize(id)
	require.NoError(t, err)
	assert.Equal(t, defaultLimit, p.Limit)
	assert.Equal(t, SortName, p.Sort)
	assert.Equal(t, OrderAsc, p.Order)
	assert.Nil(t, p.Status)

	p, err = ListQuery{Search: "  ana ", Status: "submitted", Sort: "rollNo", Order: "DESC", Limit: 50, Offset: 10}.Normalize(id)
	require.NoError(t, err)
	assert.Equal(t, "ana", p.Search)
	require.NotNil(t, p.Status)
	assert.Equal(t, StatusSubmitted, *p.Status)
	assert.Equal(t, SortRollNo, p.Sort)
	assert.Equal(t, OrderDesc, p.Order)

	bad := []ListQuery{
		{Limit: -1}, {Limit: maxLimit + 1}, {Offset: -5},
		{Status: "graded"}, {Sort: "password"}, {Order: "sideways"},
		{Search: string(make([]rune, maxSearchLength+1))},
	}
	for _, q := range bad {
		q := q
		if q.Search != "" {
			q.Search = "x"
			for len([]rune(q.Search)) <= maxSearchLength {
				q.Search += "x"
			}
		}
		_, err := q.Normalize(id)
		assert.ErrorIs(t, err, ErrInvalidQuery, "%+v", q)
	}
}

func TestListStudentsPassesValidatedFilters(t *testing.T) {
	f := newFixture(DefaultPolicy())
	f.repo.page = &StudentPage{
		Rows:           []StudentRow{{StudentID: uuid.New(), Name: "Ana", Status: StatusNotStarted}},
		Total:          7,
		TotalQuestions: 3,
		Counts:         StatusCounts{Assigned: 7, NotStarted: 7},
	}
	res, err := f.svc.ListStudents(context.Background(), f.examID, ListQuery{Search: "an", Status: "not_started", Limit: 5, Offset: 5}, f.teacher)
	require.NoError(t, err)
	assert.Equal(t, 7, res.Total)
	assert.Equal(t, 5, res.Limit)
	assert.Equal(t, 5, res.Offset)
	assert.Equal(t, 3, res.Exam.TotalQuestions)
	assert.Equal(t, "an", f.repo.gotParams.Search)
	assert.Equal(t, f.examID, f.repo.gotParams.ExamID)
	require.NotNil(t, f.repo.gotParams.Status)
	assert.Equal(t, StatusNotStarted, *f.repo.gotParams.Status)

	_, err = f.svc.ListStudents(context.Background(), f.examID, ListQuery{Status: "bogus"}, f.teacher)
	assert.ErrorIs(t, err, ErrInvalidQuery)
}

func sampleResponses(status Status) *Responses {
	optA, optB := uuid.New(), uuid.New()
	q1, q2, q3 := uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC()
	r := &Responses{
		StudentID: uuid.New(), RollNo: "R1", Name: "Ana Roy", TotalQuestions: 3, Loaded: true,
		Session: &SessionInfo{ID: uuid.New(), Status: status, StartedAt: now},
		Questions: []QuestionRecord{
			{ID: q1, QuestionNumber: 1, Text: "Pick", Type: "MCQ", Points: 2,
				Options: []OptionRecord{{ID: optA, Key: "A", Text: "one", IsCorrect: true}, {ID: optB, Key: "B", Text: "two"}},
				Answer:  &AnswerRecord{SelectedOptionID: &optB, UpdatedAt: now}},
			{ID: q2, QuestionNumber: 2, Text: "Explain", Type: "ESSAY", Points: 5,
				Answer: &AnswerRecord{TextAnswer: "   \n ", UpdatedAt: now}}, // blank text is unanswered
			{ID: q3, QuestionNumber: 3, Text: "Why", Type: "ESSAY", Points: 5},
		},
	}
	if status == StatusSubmitted {
		r.Session.SubmittedAt = &now
	}
	return r
}

func TestGetStudentResponses(t *testing.T) {
	ctx := context.Background()

	t.Run("answered and unanswered are distinguished and counted", func(t *testing.T) {
		f := newFixture(DefaultPolicy())
		f.repo.responses = sampleResponses(StatusSubmitted)
		res, err := f.svc.GetStudentResponses(ctx, f.examID, f.repo.responses.StudentID, f.teacher)
		require.NoError(t, err)
		assert.True(t, res.ResponseAvailable)
		assert.Equal(t, StatusSubmitted, res.Status)
		require.NotNil(t, res.Counts)
		assert.Equal(t, Counts{Total: 3, Answered: 1, Unanswered: 2}, *res.Counts)

		dto := ToStudentResponsesResponse(res)
		require.Len(t, dto.Questions, 3)
		assert.Equal(t, AnswerStatusAnswered, dto.Questions[0].Answer.Status)
		assert.Equal(t, "B", dto.Questions[0].Answer.SelectedOptionKey)
		assert.Equal(t, "two", dto.Questions[0].Answer.SelectedOptionText)
		assert.Equal(t, AnswerStatusUnanswered, dto.Questions[1].Answer.Status, "blank text is unanswered")
		assert.Empty(t, dto.Questions[1].Answer.TextAnswer)
		assert.NotNil(t, dto.Questions[1].Answer.SavedAt, "a saved blank row still reports its save time")
		assert.Equal(t, AnswerStatusUnanswered, dto.Questions[2].Answer.Status)
		assert.Nil(t, dto.Questions[2].Answer.SavedAt)
		assert.Equal(t, 2, dto.Questions[0].MaxMarks)
		assert.True(t, dto.Session.IsFinal)
	})

	t.Run("correct answers are hidden by default", func(t *testing.T) {
		f := newFixture(DefaultPolicy())
		f.repo.responses = sampleResponses(StatusSubmitted)
		res, err := f.svc.GetStudentResponses(ctx, f.examID, f.repo.responses.StudentID, f.teacher)
		require.NoError(t, err)
		body, err := json.Marshal(ToStudentResponsesResponse(res))
		require.NoError(t, err)
		assert.NotContains(t, string(body), "isCorrect")
	})

	t.Run("correct answers appear only when the policy allows", func(t *testing.T) {
		f := newFixture(Policy{AllowInProgress: true, ExposeCorrectAnswers: true})
		f.repo.responses = sampleResponses(StatusSubmitted)
		res, err := f.svc.GetStudentResponses(ctx, f.examID, f.repo.responses.StudentID, f.teacher)
		require.NoError(t, err)
		dto := ToStudentResponsesResponse(res)
		require.NotNil(t, dto.Questions[0].Options[0].IsCorrect)
		assert.True(t, *dto.Questions[0].Options[0].IsCorrect)
		assert.False(t, *dto.Questions[0].Options[1].IsCorrect)
	})

	t.Run("in progress is flagged as not final and the policy reaches the repository", func(t *testing.T) {
		f := newFixture(DefaultPolicy())
		f.repo.responses = sampleResponses(StatusInProgress)
		res, err := f.svc.GetStudentResponses(ctx, f.examID, f.repo.responses.StudentID, f.teacher)
		require.NoError(t, err)
		assert.True(t, f.repo.gotAllowInProgress)
		dto := ToStudentResponsesResponse(res)
		assert.False(t, dto.Session.IsFinal)
		assert.Equal(t, StatusInProgress, dto.Status)
	})

	t.Run("in progress can be disabled by policy", func(t *testing.T) {
		f := newFixture(Policy{AllowInProgress: false})
		f.repo.responses = &Responses{StudentID: uuid.New(), TotalQuestions: 3,
			Session: &SessionInfo{ID: uuid.New(), Status: StatusInProgress, StartedAt: time.Now()}}
		res, err := f.svc.GetStudentResponses(ctx, f.examID, f.repo.responses.StudentID, f.teacher)
		require.NoError(t, err)
		assert.False(t, f.repo.gotAllowInProgress)
		assert.False(t, res.ResponseAvailable)
		assert.Equal(t, ReasonInProgressBlocked, res.UnavailableReason)
		dto := ToStudentResponsesResponse(res)
		assert.NotNil(t, dto.Questions)
		assert.Empty(t, dto.Questions)
		assert.Nil(t, dto.Counts)
	})

	t.Run("a student who never started has nothing to show", func(t *testing.T) {
		f := newFixture(DefaultPolicy())
		f.repo.responses = &Responses{StudentID: uuid.New(), RollNo: "R2", Name: "Bo", TotalQuestions: 3}
		res, err := f.svc.GetStudentResponses(ctx, f.examID, f.repo.responses.StudentID, f.teacher)
		require.NoError(t, err)
		assert.Equal(t, StatusNotStarted, res.Status)
		assert.False(t, res.ResponseAvailable)
		assert.Equal(t, ReasonNotStarted, res.UnavailableReason)
		assert.Nil(t, ToStudentResponsesResponse(res).Session)
	})

	t.Run("repository errors surface unchanged", func(t *testing.T) {
		f := newFixture(DefaultPolicy())
		f.repo.err = ErrStudentNotAssigned
		_, err := f.svc.GetStudentResponses(ctx, f.examID, uuid.New(), f.teacher)
		assert.ErrorIs(t, err, ErrStudentNotAssigned)
	})

	t.Run("nil student id is rejected", func(t *testing.T) {
		f := newFixture(DefaultPolicy())
		_, err := f.svc.GetStudentResponses(ctx, f.examID, uuid.Nil, f.teacher)
		assert.ErrorIs(t, err, ErrInvalidQuery)
	})
}

func TestEscapeLike(t *testing.T) {
	assert.Equal(t, `50\%`, EscapeLike("50%"))
	assert.Equal(t, `a\_b`, EscapeLike("a_b"))
	assert.Equal(t, `a\\b`, EscapeLike(`a\b`))
	assert.Equal(t, "plain", EscapeLike("plain"))
}

// --- handler -----------------------------------------------------------------

type stubService struct {
	list    *StudentList
	resp    *StudentResponses
	history *RevisionHistory
	err     error
}

func (s *stubService) GetRevisions(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, Caller) (*RevisionHistory, error) {
	return s.history, s.err
}

func (s *stubService) ListStudents(context.Context, uuid.UUID, ListQuery, Caller) (*StudentList, error) {
	return s.list, s.err
}

func (s *stubService) GetStudentResponses(context.Context, uuid.UUID, uuid.UUID, Caller) (*StudentResponses, error) {
	return s.resp, s.err
}

func call(t *testing.T, h echo.HandlerFunc, target string, params map[string]string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	names, values := []string{}, []string{}
	for k, v := range params {
		names = append(names, k)
		values = append(values, v)
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	c.Set("user_id", uuid.New().String())
	c.Set("user_role", "teacher")
	return rec, h(c)
}

func statusOf(t *testing.T, err error) int {
	t.Helper()
	var he *httperrs.HTTPError
	require.True(t, errors.As(err, &he), "expected an HTTPError, got %v", err)
	return he.Status
}

func TestHandlerErrors(t *testing.T) {
	good := uuid.New().String()

	t.Run("invalid ids are 400", func(t *testing.T) {
		h := NewHandler(&stubService{})
		_, err := call(t, h.ListStudents, "/x", map[string]string{"exam_id": "nope"})
		assert.Equal(t, http.StatusBadRequest, statusOf(t, err))
		_, err = call(t, h.GetStudentResponses, "/x", map[string]string{"exam_id": good, "student_id": "nope"})
		assert.Equal(t, http.StatusBadRequest, statusOf(t, err))
		_, err = call(t, h.GetStudentResponses, "/x", map[string]string{"exam_id": "nope", "student_id": good})
		assert.Equal(t, http.StatusBadRequest, statusOf(t, err))
	})

	t.Run("non-numeric paging is 400", func(t *testing.T) {
		h := NewHandler(&stubService{})
		_, err := call(t, h.ListStudents, "/x?limit=abc", map[string]string{"exam_id": good})
		assert.Equal(t, http.StatusBadRequest, statusOf(t, err))
		_, err = call(t, h.ListStudents, "/x?offset=1.5", map[string]string{"exam_id": good})
		assert.Equal(t, http.StatusBadRequest, statusOf(t, err))
	})

	cases := map[string]struct {
		err  error
		want int
	}{
		"exam not found":   {ErrExamNotFound, http.StatusNotFound},
		"student missing":  {ErrStudentNotAssigned, http.StatusNotFound},
		"role forbidden":   {ErrForbiddenRole, http.StatusForbidden},
		"bad query":        {ErrInvalidQuery, http.StatusBadRequest},
		"database failure": {errors.New("connection refused: host=db password=secret"), http.StatusInternalServerError},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := NewHandler(&stubService{err: tc.err})
			_, err := call(t, h.ListStudents, "/x", map[string]string{"exam_id": good})
			assert.Equal(t, tc.want, statusOf(t, err))
			_, err = call(t, h.GetStudentResponses, "/x", map[string]string{"exam_id": good, "student_id": good})
			assert.Equal(t, tc.want, statusOf(t, err))
			if tc.want == http.StatusInternalServerError {
				var he *httperrs.HTTPError
				require.True(t, errors.As(err, &he))
				assert.NotContains(t, he.Message, "secret", "internal details must not reach the client")
			}
		})
	}
}

func TestHandlerSuccessIsNotCacheable(t *testing.T) {
	good := uuid.New().String()
	h := NewHandler(&stubService{
		list: &StudentList{Rows: []StudentRow{}},
		resp: &StudentResponses{Status: StatusNotStarted},
	})

	rec, err := call(t, h.ListStudents, "/x?search=ana&status=submitted&limit=5", map[string]string{"exam_id": good})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	var list map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.Equal(t, []any{}, list["data"], "an empty roster is an empty array, not null")

	rec, err = call(t, h.GetStudentResponses, "/x", map[string]string{"exam_id": good, "student_id": good})
	require.NoError(t, err)
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	var detail map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &detail))
	assert.Equal(t, []any{}, detail["questions"])
	assert.Equal(t, false, detail["responseAvailable"])
}

func TestOnlyGetRoutesAreRegistered(t *testing.T) {
	e := echo.New()
	NewHandler(&stubService{}).RegisterRoutes(e.Group("/api/v1"), nil, nil)
	routes := e.Routes()
	require.NotEmpty(t, routes)
	for _, r := range routes {
		assert.Equal(t, http.MethodGet, r.Method, "review route %s must be read-only", r.Path)
	}
}

func TestRevisions_SameAccessRulesAsTheReview(t *testing.T) {
	f := newFixture(Policy{AllowInProgress: false})
	ctx := context.Background()
	student, question := uuid.New(), uuid.New()
	f.repo.history = &RevisionHistory{Revisions: []RevisionRecord{}}

	_, err := f.svc.GetRevisions(ctx, f.examID, student, question, f.teacher)
	require.NoError(t, err)
	assert.False(t, f.repo.gotAllowInProgress, "the in-progress policy is passed on")

	_, err = f.svc.GetRevisions(ctx, f.examID, student, question, f.other)
	assert.ErrorIs(t, err, ErrExamNotFound)
	_, err = f.svc.GetRevisions(ctx, f.examID, student, question, f.student)
	assert.ErrorIs(t, err, ErrForbiddenRole)
	_, err = f.svc.GetRevisions(ctx, f.examID, student, uuid.Nil, f.teacher)
	assert.ErrorIs(t, err, ErrInvalidQuery)
}

func TestRevisionsHandler(t *testing.T) {
	good := uuid.NewString()
	params := map[string]string{"exam_id": good, "student_id": good, "question_id": good}

	t.Run("a bad question id is a bad request", func(t *testing.T) {
		h := NewHandler(&stubService{})
		_, err := call(t, h.GetRevisions, "/x", map[string]string{"exam_id": good, "student_id": good, "question_id": "nope"})
		var he *httperrs.HTTPError
		require.True(t, errors.As(err, &he))
		assert.Equal(t, http.StatusBadRequest, he.Status)
	})
	t.Run("a question of another exam is not found", func(t *testing.T) {
		h := NewHandler(&stubService{err: ErrQuestionNotFound})
		_, err := call(t, h.GetRevisions, "/x", params)
		var he *httperrs.HTTPError
		require.True(t, errors.As(err, &he))
		assert.Equal(t, http.StatusNotFound, he.Status)
	})
	t.Run("the history is never cached, and is an array even when empty", func(t *testing.T) {
		transcript := "plants make shugar"
		h := NewHandler(&stubService{history: &RevisionHistory{Available: true, Revisions: []RevisionRecord{
			{RevisionNo: 1, Source: "STUDENT_CORRECTION", TextAnswer: "plants make sugar", Transcript: &transcript},
		}}})
		rec, err := call(t, h.GetRevisions, "/x", params)
		require.NoError(t, err)
		assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		var body RevisionHistoryResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Len(t, body.Revisions, 1)
		assert.True(t, body.Revisions[0].Corrected)
		assert.Equal(t, "plants make shugar", *body.Revisions[0].Transcript)
		assert.Equal(t, []uuid.UUID{}, body.Revisions[0].RecordingIDs)

		h = NewHandler(&stubService{history: &RevisionHistory{UnavailableReason: ReasonNotStarted}})
		rec, err = call(t, h.GetRevisions, "/x", params)
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
		assert.Equal(t, []any{}, raw["revisions"])
		assert.Equal(t, ReasonNotStarted, raw["unavailableReason"])
	})
}
