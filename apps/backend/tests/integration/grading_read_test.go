package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/grading"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/review"
)

func TestGradingRead_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	testDB, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pool := testDB.Pool
	seed := reviewSeed{pool: pool, ctx: ctx, t: t}
	nop := zerolog.Nop()

	teacher := seed.user("TEACHER", "Terry", "Teacher", "")
	otherTeacher := seed.user("TEACHER", "Olive", "Other", "")
	adminID := seed.user("ADMIN", "Ada", "Admin", "")
	studentRole := seed.user("STUDENT", "Role", "Player", "R900")

	examID := seed.exam(teacher, "Physics")
	q1 := seed.question(examID, 1, "MCQ", "Pick one", 2)
	right := seed.option(q1, "A", "right", 1, true)
	wrong := seed.option(q1, "B", "wrong", 2, false)
	q2 := seed.question(examID, 2, "ESSAY", "Explain gravity", 10)
	q3 := seed.question(examID, 3, "ESSAY", "Explain light", 5)

	attempt := func(roll, name, status string) (student, session uuid.UUID) {
		student = seed.user("STUDENT", name, "Student", roll)
		session = seed.session(seed.assign(examID, student, "ASSIGNED"), examID, student, status, time.Now().Add(-time.Hour))
		return
	}
	a, aSess := attempt("R001", "Alice", "SUBMITTED") // right answer, two essays
	seed.answer(aSess, q1, &right, "")
	seed.answer(aSess, q2, nil, "Masses attract each other.")
	seed.answer(aSess, q3, nil, "Light is a wave and a particle.")

	b, bSess := attempt("R002", "Bob", "SUBMITTED") // wrong answer, left both essays blank
	seed.answer(bSess, q1, &wrong, "")

	c := seed.user("STUDENT", "Cara", "Student", "R003") // assigned, never started
	seed.assign(examID, c, "ASSIGNED")
	d, _ := attempt("R004", "Dan", "IN_PROGRESS")
	e, eSess := attempt("R005", "Eve", "EXPIRED")
	seed.answer(eSess, q1, &right, "")

	f, fSub := attempt("R006", "Finn", "SUBMITTED") // submitted, then a retake in progress
	seed.answer(fSub, q1, &right, "")
	var fAssignment uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `SELECT assignment_id FROM sessions WHERE id = $1`, fSub).Scan(&fAssignment))
	fRetake := seed.session(fAssignment, examID, f, "IN_PROGRESS", time.Now())

	stranger := seed.user("STUDENT", "Sid", "Stranger", "R007") // not assigned to the exam

	examSvc := exam.NewService(exam.NewRepository(pool), &nop)
	svc := grading.NewService(grading.NewRepository(pool), examSvc, &nop)
	owner := grading.Caller{ID: teacher, Role: platformauth.RoleTeacher}

	get := func(who uuid.UUID, caller grading.Caller) (*grading.Marking, error) {
		return svc.GetMarking(ctx, examID, who, caller)
	}
	mustGet := func(who uuid.UUID) *grading.Marking {
		m, err := get(who, owner)
		require.NoError(t, err)
		return m
	}
	count := func(table string) int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n))
		return n
	}

	// Alice's second answer already has a teacher mark of 7.5, with a comment.
	_, err := pool.Exec(ctx, `INSERT INTO answer_marks (session_id, question_id, marks_half, max_half, comment, version, marked_by)
	                          VALUES ($1, $2, 15, 20, 'clear and complete', 3, $3)`, aSess, q2, teacher)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO paper_gradings (session_id, exam_id, version) VALUES ($1, $2, 7)`, aSess, examID)
	require.NoError(t, err)
	before := reviewSeed{pool: pool, ctx: ctx, t: t}.snapshot()

	t.Run("a submitted paper: auto mark, teacher mark, and what is still unmarked", func(t *testing.T) {
		m := mustGet(a)
		assert.True(t, m.MarkingAllowed)
		require.NotNil(t, m.SessionID)
		assert.Equal(t, aSess, *m.SessionID)
		require.Len(t, m.Lines, 3)

		assert.Equal(t, grading.SourceAuto, m.Lines[0].Source)
		assert.Equal(t, 4, *m.Lines[0].EffectiveHalf)
		require.NotNil(t, m.Lines[0].CorrectOptionID)
		assert.Equal(t, right, *m.Lines[0].CorrectOptionID, "the teacher sees which option is correct while marking")

		assert.Equal(t, grading.SourceTeacher, m.Lines[1].Source)
		assert.Equal(t, 15, *m.Lines[1].EffectiveHalf)
		assert.Equal(t, 3, m.Lines[1].Version)
		require.NotNil(t, m.Lines[1].Comment)
		assert.Equal(t, "clear and complete", *m.Lines[1].Comment)

		assert.Equal(t, grading.SourceNone, m.Lines[2].Source, "an answered essay waits for the teacher")
		assert.True(t, m.Lines[2].Answered)

		assert.Equal(t, grading.StatusDraft, m.Paper.Status)
		assert.Equal(t, 7, m.Paper.Version)
		assert.Equal(t, 19, m.Paper.TotalHalf)
		assert.Equal(t, 4+20+10, m.Paper.MaxHalf)
		assert.Equal(t, 2, m.Paper.Marked)
		assert.Equal(t, 3, m.Paper.Questions)
	})

	t.Run("a paper with wrong and blank answers is complete at zero", func(t *testing.T) {
		m := mustGet(b)
		assert.Zero(t, m.Paper.TotalHalf)
		assert.Equal(t, 3, m.Paper.Marked, "blank answers get 0 automatically")
		assert.False(t, m.Lines[1].Answered)
		assert.Equal(t, grading.SourceAuto, m.Lines[1].Source)
		assert.Zero(t, m.Paper.Version, "nothing has been saved for this paper yet")
	})

	t.Run("an expired attempt can be marked", func(t *testing.T) {
		m := mustGet(e)
		assert.True(t, m.MarkingAllowed)
		assert.Equal(t, review.StatusExpired, m.SessionStatus)
		assert.Equal(t, 4, *m.Lines[0].EffectiveHalf)
	})

	t.Run("a student who has not started has nothing to mark", func(t *testing.T) {
		m := mustGet(c)
		assert.False(t, m.MarkingAllowed)
		assert.Equal(t, grading.ReasonNotStarted, m.Reason)
		assert.Nil(t, m.SessionID)
		assert.Empty(t, m.Lines)
	})

	t.Run("a student still sitting the exam: nothing to mark, and no answers or key sent", func(t *testing.T) {
		m := mustGet(d)
		assert.False(t, m.MarkingAllowed)
		assert.Equal(t, grading.ReasonSessionNotEnded, m.Reason)
		assert.Empty(t, m.Lines)
	})

	t.Run("a retake in progress does not hide the submitted attempt, exactly as in the review", func(t *testing.T) {
		m := mustGet(f)
		require.NotNil(t, m.SessionID)
		assert.Equal(t, fSub, *m.SessionID, "the submitted attempt, not the retake")
		assert.NotEqual(t, fRetake, *m.SessionID)
		assert.True(t, m.MarkingAllowed)

		// It is the attempt the review shows. If these ever disagreed, a teacher
		// would be marking a paper other than the one on their screen.
		responses, err := review.NewRepository(pool).GetResponses(ctx, examID, f, true)
		require.NoError(t, err)
		require.NotNil(t, responses.Session)
		assert.Equal(t, responses.Session.ID, *m.SessionID)
	})

	t.Run("the same attempt as the review for every kind of student", func(t *testing.T) {
		for name, who := range map[string]uuid.UUID{"submitted": a, "wrong": b, "expired": e, "in progress": d, "retake": f} {
			responses, err := review.NewRepository(pool).GetResponses(ctx, examID, who, true)
			require.NoError(t, err, name)
			m := mustGet(who)
			require.NotNil(t, m.SessionID, name)
			assert.Equal(t, responses.Session.ID, *m.SessionID, name)
		}
	})

	t.Run("who may ask", func(t *testing.T) {
		_, err := get(a, grading.Caller{ID: adminID, Role: platformauth.RoleAdmin})
		assert.NoError(t, err, "an administrator may")

		_, err = get(a, grading.Caller{ID: otherTeacher, Role: platformauth.RoleTeacher})
		assert.ErrorIs(t, err, grading.ErrExamNotFound, "another teacher cannot tell the exam exists")

		_, err = get(a, grading.Caller{ID: studentRole, Role: platformauth.RoleStudent})
		assert.ErrorIs(t, err, grading.ErrForbiddenRole)

		_, err = get(stranger, owner)
		assert.ErrorIs(t, err, grading.ErrExamNotFound, "a student not assigned to the exam")

		_, err = svc.GetMarking(ctx, uuid.New(), a, owner)
		assert.ErrorIs(t, err, grading.ErrExamNotFound, "an exam that does not exist")
	})

	t.Run("asking changes nothing", func(t *testing.T) {
		for _, who := range []uuid.UUID{a, b, c, d, e, f} {
			mustGet(who)
		}
		assert.Equal(t, before, reviewSeed{pool: pool, ctx: ctx, t: t}.snapshot())
		assert.Equal(t, 1, count("paper_gradings"), "no grading record is created by reading")
		assert.Equal(t, 1, count("answer_marks"))
	})

	t.Run("a draft follows the questions as they are now", func(t *testing.T) {
		// Bob chose B. If the key is corrected to B, his answer becomes right.
		_, err := pool.Exec(ctx, `UPDATE question_options SET is_correct = (id = $2) WHERE question_id = $1`, q1, wrong)
		require.NoError(t, err)
		defer func() {
			_, err := pool.Exec(ctx, `UPDATE question_options SET is_correct = (id = $2) WHERE question_id = $1`, q1, right)
			require.NoError(t, err)
		}()

		m := mustGet(b)
		assert.Equal(t, 4, *m.Lines[0].EffectiveHalf)
		assert.Equal(t, wrong, *m.Lines[0].CorrectOptionID)
	})

	t.Run("a final paper is read from its snapshot and does not follow later edits", func(t *testing.T) {
		snapshot, err := json.Marshal(grading.Snapshot{Version: 1, Lines: []grading.SnapshotLine{
			{QuestionID: q1, QuestionNumber: 1, Type: "MCQ", MaxHalf: 4, AwardedHalf: 4, Source: grading.SourceAuto},
			{QuestionID: q2, QuestionNumber: 2, Type: "ESSAY", MaxHalf: 20, AwardedHalf: 15, Source: grading.SourceTeacher},
			{QuestionID: q3, QuestionNumber: 3, Type: "ESSAY", MaxHalf: 10, AwardedHalf: 6, Source: grading.SourceTeacher},
		}})
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `UPDATE paper_gradings SET status = 'FINAL', version = 9, finalised_by = $2, finalised_at = NOW(),
		                         total_half = 25, max_half = 34, snapshot = $3::jsonb WHERE session_id = $1`, aSess, teacher, string(snapshot))
		require.NoError(t, err)

		// Afterwards the exam is edited: the key changes and the essay is worth more.
		_, err = pool.Exec(ctx, `UPDATE question_options SET is_correct = (id = $2) WHERE question_id = $1`, q1, wrong)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `UPDATE questions SET points = 50 WHERE id = $1`, q2)
		require.NoError(t, err)
		defer func() {
			_, _ = pool.Exec(ctx, `UPDATE question_options SET is_correct = (id = $2) WHERE question_id = $1`, q1, right)
			_, _ = pool.Exec(ctx, `UPDATE questions SET points = 10 WHERE id = $1`, q2)
		}()

		m := mustGet(a)
		assert.Equal(t, grading.StatusFinal, m.Paper.Status)
		assert.Equal(t, 9, m.Paper.Version)
		assert.Equal(t, 25, m.Paper.TotalHalf)
		assert.Equal(t, 34, m.Paper.MaxHalf, "the maximum at the time, not the 50 points it has now")
		require.Len(t, m.Lines, 3)
		assert.Equal(t, 4, *m.Lines[0].EffectiveHalf, "still marked right although the key now says otherwise")
		assert.Equal(t, 20, m.Lines[1].MaxHalf)
		for _, l := range m.Lines {
			assert.True(t, l.FromSnapshot)
			assert.Nil(t, l.CorrectOptionID, "no answer key for a final paper")
		}
		require.NotNil(t, m.Paper.FinalisedBy)
		assert.Equal(t, teacher, *m.Paper.FinalisedBy)
	})
}
