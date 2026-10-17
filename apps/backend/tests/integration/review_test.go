package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/review"
)

type reviewSeed struct {
	pool *pgxpool.Pool
	ctx  context.Context
	t    *testing.T
}

func (s reviewSeed) exec(sql string, args ...any) {
	s.t.Helper()
	_, err := s.pool.Exec(s.ctx, sql, args...)
	require.NoError(s.t, err)
}

func (s reviewSeed) user(role, first, last, roll string) uuid.UUID {
	s.t.Helper()
	id := uuid.New()
	var rollArg *string
	if roll != "" {
		rollArg = &roll
	}
	s.exec(`INSERT INTO users (id, email, first_name, last_name, role, roll_no)
	        VALUES ($1, $2, $3, $4, $5, $6)`, id, id.String()+"@test.local", first, last, role, rollArg)
	return id
}

func (s reviewSeed) exam(owner uuid.UUID, title string) uuid.UUID {
	s.t.Helper()
	id := uuid.New()
	s.exec(`INSERT INTO exams (id, title, subject, duration_mins, status, created_by, published_at)
	        VALUES ($1, $2, 'Subject', 60, 'PUBLISHED', $3, NOW())`, id, title, owner)
	return id
}

func (s reviewSeed) question(examID uuid.UUID, number int, qtype, text string, points int) uuid.UUID {
	s.t.Helper()
	id := uuid.New()
	s.exec(`INSERT INTO questions (id, exam_id, question_number, text, type, points)
	        VALUES ($1, $2, $3, $4, $5, $6)`, id, examID, number, text, qtype, points)
	return id
}

func (s reviewSeed) option(questionID uuid.UUID, key, text string, order int, correct bool) uuid.UUID {
	s.t.Helper()
	id := uuid.New()
	s.exec(`INSERT INTO question_options (id, question_id, option_key, option_text, display_order, is_correct)
	        VALUES ($1, $2, $3, $4, $5, $6)`, id, questionID, key, text, order, correct)
	return id
}

func (s reviewSeed) assign(examID, studentID uuid.UUID, status string) uuid.UUID {
	s.t.Helper()
	id := uuid.New()
	s.exec(`INSERT INTO assignments (id, exam_id, student_id, status) VALUES ($1, $2, $3, $4)`,
		id, examID, studentID, status)
	return id
}

func (s reviewSeed) session(assignmentID, examID, studentID uuid.UUID, status string, started time.Time) uuid.UUID {
	s.t.Helper()
	id := uuid.New()
	var submitted *time.Time
	if status == "SUBMITTED" {
		v := started.Add(30 * time.Minute)
		submitted = &v
	}
	s.exec(`INSERT INTO sessions (id, assignment_id, exam_id, student_id, status, started_at, submitted_at)
	        VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, assignmentID, examID, studentID, status, started, submitted)
	return id
}

func (s reviewSeed) answer(sessionID, questionID uuid.UUID, option *uuid.UUID, text string) {
	s.t.Helper()
	s.exec(`INSERT INTO answers (session_id, question_id, selected_option_id, text_answer)
	        VALUES ($1, $2, $3, $4)`, sessionID, questionID, option, text)
}

// snapshot captures every row the review must never change.
func (s reviewSeed) snapshot() string {
	s.t.Helper()
	var out string
	err := s.pool.QueryRow(s.ctx, `
		SELECT COALESCE((SELECT string_agg(a::text, '|' ORDER BY a.id) FROM answers a), '') || '#' ||
		       COALESCE((SELECT string_agg(x::text, '|' ORDER BY x.id) FROM sessions x), '') || '#' ||
		       COALESCE((SELECT string_agg(e::text, '|' ORDER BY e.id) FROM exams e), '') || '#' ||
		       COALESCE((SELECT string_agg(n::text, '|' ORDER BY n.id) FROM assignments n), '')`).Scan(&out)
	require.NoError(s.t, err)
	return out
}

func TestResponseReview_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	testDB, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	seed := reviewSeed{pool: testDB.Pool, ctx: ctx, t: t}
	nop := zerolog.Nop()

	teacher := seed.user("TEACHER", "Terry", "Teacher", "")
	otherTeacher := seed.user("TEACHER", "Olive", "Other", "")
	adminID := seed.user("ADMIN", "Ada", "Admin", "")

	// Names chosen so name order differs from roll-number order.
	zoe := seed.user("STUDENT", "Zoe", "Adams", "R001")       // submitted (also has a later retake in progress)
	bob := seed.user("STUDENT", "Bob", "Brown", "R002")       // in progress
	cara := seed.user("STUDENT", "Cara", "Clark", "R003")     // never started
	dan := seed.user("STUDENT", "Dan", "Davis", "R004")       // expired, nothing saved
	revoked := seed.user("STUDENT", "Rex", "Revoked", "R005") // assignment revoked
	outsider := seed.user("STUDENT", "Ola", "Outsider", "R006")

	examID := seed.exam(teacher, "Physics Midterm")
	// Inserted out of order on purpose: the API must return question order.
	q3 := seed.question(examID, 3, "MCQ", "Third question", 4)
	q1 := seed.question(examID, 1, "MCQ", "First question", 2)
	q2 := seed.question(examID, 2, "ESSAY", "Second question", 10)
	q1a := seed.option(q1, "A", "alpha", 1, true)
	q1b := seed.option(q1, "B", "beta", 2, false)
	q3a := seed.option(q3, "A", "gamma", 1, false)
	q3b := seed.option(q3, "B", "delta", 2, true)
	_, _ = q1a, q3a

	start := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	zoeAsg := seed.assign(examID, zoe, "ASSIGNED")
	zoeSess := seed.session(zoeAsg, examID, zoe, "SUBMITTED", start)
	seed.answer(zoeSess, q1, &q1b, "")
	seed.answer(zoeSess, q2, nil, "Long essay answer with 50% and _ characters")
	seed.answer(zoeSess, q3, &q3b, "")
	// A retake started after submission must not hide the submitted record.
	seed.session(zoeAsg, examID, zoe, "IN_PROGRESS", start.Add(2*time.Hour))

	bobAsg := seed.assign(examID, bob, "ASSIGNED")
	bobSess := seed.session(bobAsg, examID, bob, "IN_PROGRESS", start)
	seed.answer(bobSess, q1, &q1a, "")
	seed.answer(bobSess, q2, nil, "   ") // saved but blank: not answered

	seed.assign(examID, cara, "ASSIGNED")

	danAsg := seed.assign(examID, dan, "ASSIGNED")
	seed.session(danAsg, examID, dan, "EXPIRED", start)

	seed.assign(examID, revoked, "REVOKED")

	// A second exam owned by someone else, to prove isolation.
	otherExam := seed.exam(otherTeacher, "Chemistry")
	seed.question(otherExam, 1, "ESSAY", "Other question", 1)
	seed.assign(otherExam, outsider, "ASSIGNED")

	emptyExam := seed.exam(teacher, "Empty Exam")

	examSvc := exam.NewService(exam.NewRepository(testDB.Pool), &nop)
	repo := review.NewRepository(testDB.Pool)
	svc := review.NewService(repo, examSvc, review.DefaultPolicy(), &nop)

	teacherC := review.Caller{ID: teacher, Role: platformauth.RoleTeacher}
	otherC := review.Caller{ID: otherTeacher, Role: platformauth.RoleTeacher}
	adminC := review.Caller{ID: adminID, Role: platformauth.RoleAdmin}
	studentC := review.Caller{ID: zoe, Role: platformauth.RoleStudent}

	before := seed.snapshot()

	t.Run("overview lists every assigned student including not started", func(t *testing.T) {
		res, err := svc.ListStudents(ctx, examID, review.ListQuery{}, teacherC)
		require.NoError(t, err)
		assert.Equal(t, 4, res.Total, "the revoked assignment is not listed")
		assert.Equal(t, review.StatusCounts{Assigned: 4, NotStarted: 1, InProgress: 1, Submitted: 1, Expired: 1}, res.Counts)
		assert.Equal(t, 3, res.Exam.TotalQuestions)
		assert.Equal(t, "Physics Midterm", res.Exam.Title)

		names := []string{}
		byName := map[string]review.StudentRow{}
		for _, r := range res.Rows {
			names = append(names, r.Name)
			byName[r.Name] = r
		}
		assert.Equal(t, []string{"Bob Brown", "Cara Clark", "Dan Davis", "Zoe Adams"}, names, "sorted by name")

		z := byName["Zoe Adams"]
		assert.Equal(t, review.StatusSubmitted, z.Status, "the submitted session wins over a later retake")
		require.NotNil(t, z.SubmittedAt)
		require.NotNil(t, z.AnsweredCount)
		assert.Equal(t, 3, *z.AnsweredCount, "both MCQ choices and the essay")
		assert.Equal(t, "R001", z.RollNo)

		b := byName["Bob Brown"]
		assert.Equal(t, review.StatusInProgress, b.Status)
		assert.Nil(t, b.SubmittedAt)
		require.NotNil(t, b.AnsweredCount)
		assert.Equal(t, 1, *b.AnsweredCount, "a saved blank answer is not counted")

		c := byName["Cara Clark"]
		assert.Equal(t, review.StatusNotStarted, c.Status)
		assert.Nil(t, c.AnsweredCount)
		assert.Nil(t, c.SessionID)

		d := byName["Dan Davis"]
		assert.Equal(t, review.StatusExpired, d.Status)
		require.NotNil(t, d.AnsweredCount)
		assert.Equal(t, 0, *d.AnsweredCount)
	})

	t.Run("filters", func(t *testing.T) {
		res, err := svc.ListStudents(ctx, examID, review.ListQuery{Status: "SUBMITTED"}, teacherC)
		require.NoError(t, err)
		require.Len(t, res.Rows, 1)
		assert.Equal(t, "Zoe Adams", res.Rows[0].Name)
		assert.Equal(t, 4, res.Counts.Assigned, "summary ignores the filter")

		res, err = svc.ListStudents(ctx, examID, review.ListQuery{Search: "r003"}, teacherC)
		require.NoError(t, err)
		require.Len(t, res.Rows, 1)
		assert.Equal(t, "Cara Clark", res.Rows[0].Name)

		res, err = svc.ListStudents(ctx, examID, review.ListQuery{Search: "bRoWn"}, teacherC)
		require.NoError(t, err)
		require.Len(t, res.Rows, 1)

		res, err = svc.ListStudents(ctx, examID, review.ListQuery{Search: "Bob Brown"}, teacherC)
		require.NoError(t, err)
		require.Len(t, res.Rows, 1, "full name search")

		res, err = svc.ListStudents(ctx, examID, review.ListQuery{Search: "%"}, teacherC)
		require.NoError(t, err)
		assert.Empty(t, res.Rows, "wildcards are matched literally")
		assert.Equal(t, 0, res.Total)

		res, err = svc.ListStudents(ctx, examID, review.ListQuery{Search: "zoe", Status: "EXPIRED"}, teacherC)
		require.NoError(t, err)
		assert.Empty(t, res.Rows, "filters combine")

		res, err = svc.ListStudents(ctx, examID, review.ListQuery{Search: "Ola"}, teacherC)
		require.NoError(t, err)
		assert.Empty(t, res.Rows, "students of other exams never appear")
	})

	t.Run("sorting and stable pagination", func(t *testing.T) {
		collect := func(q review.ListQuery) []string {
			var out []string
			for offset := 0; ; offset += 3 {
				q.Limit, q.Offset = 3, offset
				res, err := svc.ListStudents(ctx, examID, q, teacherC)
				require.NoError(t, err)
				if len(res.Rows) == 0 {
					return out
				}
				for _, r := range res.Rows {
					out = append(out, r.RollNo)
				}
			}
		}
		assert.Equal(t, []string{"R002", "R003", "R004", "R001"}, collect(review.ListQuery{}))
		assert.Equal(t, []string{"R001", "R002", "R003", "R004"}, collect(review.ListQuery{Sort: "rollNo"}))
		assert.Equal(t, []string{"R004", "R003", "R002", "R001"}, collect(review.ListQuery{Sort: "rollNo", Order: "desc"}))

		res, err := svc.ListStudents(ctx, examID, review.ListQuery{Limit: 2, Offset: 2}, teacherC)
		require.NoError(t, err)
		assert.Len(t, res.Rows, 2)
		assert.Equal(t, 4, res.Total, "total is the full match count, not the page size")

		res, err = svc.ListStudents(ctx, examID, review.ListQuery{Limit: 2, Offset: 40}, teacherC)
		require.NoError(t, err)
		assert.Empty(t, res.Rows)
		assert.Equal(t, 4, res.Total)
	})

	t.Run("an exam with no assigned students is empty, not an error", func(t *testing.T) {
		res, err := svc.ListStudents(ctx, emptyExam, review.ListQuery{}, teacherC)
		require.NoError(t, err)
		assert.Empty(t, res.Rows)
		assert.Equal(t, 0, res.Total)
		assert.Equal(t, review.StatusCounts{}, res.Counts)
		assert.NotNil(t, res.Rows)
	})

	t.Run("authorization", func(t *testing.T) {
		_, err := svc.ListStudents(ctx, examID, review.ListQuery{}, otherC)
		assert.ErrorIs(t, err, review.ErrExamNotFound, "another teacher is told the exam does not exist")
		_, err = svc.GetStudentResponses(ctx, examID, zoe, otherC)
		assert.ErrorIs(t, err, review.ErrExamNotFound)
		_, err = svc.ListStudents(ctx, uuid.New(), review.ListQuery{}, teacherC)
		assert.ErrorIs(t, err, review.ErrExamNotFound, "same answer for an exam that does not exist")
		_, err = svc.ListStudents(ctx, examID, review.ListQuery{}, studentC)
		assert.ErrorIs(t, err, review.ErrForbiddenRole)
		_, err = svc.GetStudentResponses(ctx, examID, zoe, studentC)
		assert.ErrorIs(t, err, review.ErrForbiddenRole)

		res, err := svc.ListStudents(ctx, examID, review.ListQuery{}, adminC)
		require.NoError(t, err)
		assert.Equal(t, 4, res.Total, "an administrator may review any exam")
	})

	t.Run("submitted responses are associated with the right questions in order", func(t *testing.T) {
		res, err := svc.GetStudentResponses(ctx, examID, zoe, teacherC)
		require.NoError(t, err)
		assert.True(t, res.ResponseAvailable)
		assert.Equal(t, review.StatusSubmitted, res.Status)
		assert.Equal(t, "Zoe Adams", res.Name)
		assert.Equal(t, "R001", res.RollNo)
		require.NotNil(t, res.Session)
		assert.Equal(t, zoeSess, res.Session.ID, "the submitted session, not the retake")
		require.NotNil(t, res.Session.SubmittedAt)

		dto := review.ToStudentResponsesResponse(res)
		require.Len(t, dto.Questions, 3)
		assert.Equal(t, []int{1, 2, 3}, []int{dto.Questions[0].QuestionNumber, dto.Questions[1].QuestionNumber, dto.Questions[2].QuestionNumber})
		assert.Equal(t, q1, dto.Questions[0].ID)

		// Q1: option B chosen
		assert.Equal(t, "ANSWERED", dto.Questions[0].Answer.Status)
		assert.Equal(t, "B", dto.Questions[0].Answer.SelectedOptionKey)
		assert.Equal(t, "beta", dto.Questions[0].Answer.SelectedOptionText)
		assert.Equal(t, 2, dto.Questions[0].MaxMarks)
		require.Len(t, dto.Questions[0].Options, 2)
		assert.Equal(t, "A", dto.Questions[0].Options[0].Key, "options in display order")
		// Q2: essay
		assert.Equal(t, "Long essay answer with 50% and _ characters", dto.Questions[1].Answer.TextAnswer)
		assert.Empty(t, dto.Questions[1].Options)
		// Q3: option B (delta), which is a different option from Q1's B
		assert.Equal(t, "delta", dto.Questions[2].Answer.SelectedOptionText)

		require.NotNil(t, dto.Counts)
		assert.Equal(t, review.CountsDTO{Total: 3, Answered: 3, Unanswered: 0}, *dto.Counts)
		assert.True(t, dto.Session.IsFinal)
	})

	t.Run("answer keys are not returned", func(t *testing.T) {
		res, err := svc.GetStudentResponses(ctx, examID, zoe, teacherC)
		require.NoError(t, err)
		for _, q := range review.ToStudentResponsesResponse(res).Questions {
			for _, o := range q.Options {
				assert.Nil(t, o.IsCorrect)
			}
		}

		withKeys := review.NewService(repo, examSvc, review.Policy{AllowInProgress: true, ExposeCorrectAnswers: true}, &nop)
		res, err = withKeys.GetStudentResponses(ctx, examID, zoe, teacherC)
		require.NoError(t, err)
		opt := review.ToStudentResponsesResponse(res).Questions[0].Options[0]
		require.NotNil(t, opt.IsCorrect)
		assert.True(t, *opt.IsCorrect)
	})

	t.Run("in progress responses are readable and flagged not final", func(t *testing.T) {
		res, err := svc.GetStudentResponses(ctx, examID, bob, teacherC)
		require.NoError(t, err)
		assert.True(t, res.ResponseAvailable)
		dto := review.ToStudentResponsesResponse(res)
		assert.False(t, dto.Session.IsFinal)
		assert.Equal(t, review.StatusInProgress, dto.Status)
		require.NotNil(t, dto.Counts)
		assert.Equal(t, review.CountsDTO{Total: 3, Answered: 1, Unanswered: 2}, *dto.Counts)
		assert.Equal(t, "UNANSWERED", dto.Questions[1].Answer.Status, "blank saved text is unanswered")
		assert.Equal(t, "UNANSWERED", dto.Questions[2].Answer.Status, "no row at all")
		assert.Nil(t, dto.Questions[2].Answer.SavedAt)

		strict := review.NewService(repo, examSvc, review.Policy{AllowInProgress: false}, &nop)
		res, err = strict.GetStudentResponses(ctx, examID, bob, teacherC)
		require.NoError(t, err)
		assert.False(t, res.ResponseAvailable)
		assert.Equal(t, review.ReasonInProgressBlocked, res.UnavailableReason)
		assert.Empty(t, res.Questions, "answers are not even read when the policy forbids them")
	})

	t.Run("students with no session and expired sessions", func(t *testing.T) {
		res, err := svc.GetStudentResponses(ctx, examID, cara, teacherC)
		require.NoError(t, err)
		assert.Equal(t, review.StatusNotStarted, res.Status)
		assert.False(t, res.ResponseAvailable)
		assert.Equal(t, review.ReasonNotStarted, res.UnavailableReason)
		assert.Nil(t, res.Session)
		assert.Equal(t, 3, res.Exam.TotalQuestions)

		res, err = svc.GetStudentResponses(ctx, examID, dan, teacherC)
		require.NoError(t, err)
		assert.True(t, res.ResponseAvailable)
		assert.Equal(t, review.StatusExpired, res.Status)
		require.NotNil(t, res.Counts)
		assert.Equal(t, 0, res.Counts.Answered)
		assert.Equal(t, 3, res.Counts.Unanswered)
	})

	t.Run("students not assigned to the exam are not found", func(t *testing.T) {
		for name, id := range map[string]uuid.UUID{"revoked": revoked, "other exam": outsider, "random": uuid.New(), "teacher": teacher} {
			_, err := svc.GetStudentResponses(ctx, examID, id, teacherC)
			assert.ErrorIs(t, err, review.ErrStudentNotAssigned, name)
		}
	})

	t.Run("invalid input", func(t *testing.T) {
		_, err := svc.ListStudents(ctx, examID, review.ListQuery{Status: "nonsense"}, teacherC)
		assert.ErrorIs(t, err, review.ErrInvalidQuery)
		_, err = svc.GetStudentResponses(ctx, examID, uuid.Nil, teacherC)
		assert.ErrorIs(t, err, review.ErrInvalidQuery)
	})

	t.Run("database failures are returned, not hidden", func(t *testing.T) {
		pool, err := pgxpool.New(ctx, testDB.Pool.Config().ConnString())
		require.NoError(t, err)
		pool.Close()
		broken := review.NewService(review.NewRepository(pool), examSvc, review.DefaultPolicy(), &nop)
		_, err = broken.ListStudents(ctx, examID, review.ListQuery{}, teacherC)
		require.Error(t, err)
		assert.NotErrorIs(t, err, review.ErrExamNotFound)
		_, err = broken.GetStudentResponses(ctx, examID, zoe, teacherC)
		require.Error(t, err)
	})

	t.Run("reviewing never changes the submission", func(t *testing.T) {
		// After every call above, no row in any reviewed table has changed.
		assert.Equal(t, before, seed.snapshot(), "reviewing must not change answers, sessions, exams or assignments")
	})

	t.Run("a late save is visible to the next review without any caching", func(t *testing.T) {
		seed.answer(bobSess, q3, &q3a, "")
		res, err := svc.GetStudentResponses(ctx, examID, bob, teacherC)
		require.NoError(t, err)
		require.NotNil(t, res.Counts)
		assert.Equal(t, 2, res.Counts.Answered)
	})
}
