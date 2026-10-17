package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/grading"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
)

func TestGradingFinalise_Integration(t *testing.T) {
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

	examID := seed.exam(teacher, "Physics")
	mcq := seed.question(examID, 1, "MCQ", "Pick one", 2)
	right := seed.option(mcq, "A", "right", 1, true)
	essay := seed.question(examID, 2, "ESSAY", "Explain gravity", 10)

	newStudent := func(roll, status string) (student, session uuid.UUID) {
		student = seed.user("STUDENT", "S"+roll, "Student", roll)
		session = seed.session(seed.assign(examID, student, "ASSIGNED"), examID, student, status, time.Now().Add(-time.Hour))
		seed.answer(session, mcq, &right, "")
		seed.answer(session, essay, nil, "The student's own words about it")
		return
	}

	examSvc := exam.NewService(exam.NewRepository(pool), &nop)
	svc := grading.NewService(grading.NewRepository(pool), examSvc, &nop)
	owner := grading.Caller{ID: teacher, Role: platformauth.RoleTeacher}
	admin := grading.Caller{ID: adminID, Role: platformauth.RoleAdmin}
	stranger := grading.Caller{ID: otherTeacher, Role: platformauth.RoleTeacher}
	meta := grading.AccessMeta{RequestID: "req-fin", IP: "203.0.113.9"}
	half := func(i int) *int { return &i }

	save := func(student, session uuid.UUID, marks int, version int) (*grading.SaveMarkResult, error) {
		return svc.SaveMark(ctx, examID, student, essay, grading.SaveMarkRequest{
			SessionID: session, MarksHalf: half(marks), ExpectedVersion: version,
		}, owner, meta)
	}
	finalise := func(student, session uuid.UUID, version int, c grading.Caller) (*grading.Marking, error) {
		return svc.Finalise(ctx, examID, student, grading.FinaliseRequest{SessionID: session, ExpectedPaperVersion: version}, c, meta)
	}
	reopen := func(student, session uuid.UUID, reason string, c grading.Caller) (*grading.Marking, error) {
		return svc.Reopen(ctx, examID, student, grading.ReopenRequest{SessionID: session, Reason: reason}, c, meta)
	}
	count := func(sql string, args ...any) int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, sql, args...).Scan(&n))
		return n
	}
	audits := func(action string) int { return count(`SELECT count(*) FROM audit_events WHERE action = $1`, action) }
	events := func(session uuid.UUID, kind string) int {
		return count(`SELECT count(*) FROM paper_grading_events WHERE session_id = $1 AND event = $2`, session, kind)
	}
	marking := func(student uuid.UUID) *grading.Marking {
		m, err := svc.GetMarking(ctx, examID, student, owner)
		require.NoError(t, err)
		return m
	}

	t.Run("a paper with an unmarked essay cannot be finalised, and nothing is left behind", func(t *testing.T) {
		stu, sess := newStudent("R001", "SUBMITTED")
		_, err := finalise(stu, sess, 0, owner)
		assert.ErrorIs(t, err, grading.ErrPaperIncomplete)
		var inc *grading.IncompleteError
		require.ErrorAs(t, err, &inc)
		assert.Equal(t, []int{2}, inc.Questions)
		assert.Equal(t, 0, count(`SELECT count(*) FROM paper_gradings WHERE session_id = $1 AND status = 'FINAL'`, sess))
		assert.Equal(t, 0, events(sess, "FINALISED"))
		assert.Equal(t, 0, audits("grading.paper.finalise"))
	})

	var stu, sess uuid.UUID
	t.Run("finalising freezes the total and records the event and the audit entry", func(t *testing.T) {
		stu, sess = newStudent("R002", "SUBMITTED")
		res, err := save(stu, sess, 15, 0)
		require.NoError(t, err)

		m, err := finalise(stu, sess, res.Paper.Version, owner)
		require.NoError(t, err)
		assert.Equal(t, grading.StatusFinal, m.Paper.Status)
		assert.Equal(t, 19, m.Paper.TotalHalf, "the auto-marked MCQ (4) and the essay (15)")
		assert.Equal(t, 24, m.Paper.MaxHalf)

		var status string
		var total, maxHalf int
		var by uuid.UUID
		require.NoError(t, pool.QueryRow(ctx, `SELECT status, total_half, max_half, finalised_by FROM paper_gradings WHERE session_id = $1`, sess).
			Scan(&status, &total, &maxHalf, &by))
		assert.Equal(t, []any{"FINAL", 19, 24, teacher}, []any{status, total, maxHalf, by})
		assert.Equal(t, 1, events(sess, "FINALISED"))
		assert.Equal(t, 1, audits("grading.paper.finalise"))
	})

	t.Run("a final paper refuses new marks", func(t *testing.T) {
		_, err := save(stu, sess, 4, 1)
		assert.ErrorIs(t, err, grading.ErrPaperFinalised)
	})

	t.Run("later edits to the answer key and the points cannot change a final paper", func(t *testing.T) {
		before := marking(stu)
		_, err := pool.Exec(ctx, `UPDATE question_options SET is_correct = FALSE WHERE question_id = $1`, mcq)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `UPDATE questions SET points = 1 WHERE id IN ($1, $2)`, mcq, essay)
		require.NoError(t, err)
		defer func() {
			_, _ = pool.Exec(ctx, `UPDATE question_options SET is_correct = TRUE WHERE id = $1`, right)
			_, _ = pool.Exec(ctx, `UPDATE questions SET points = 2 WHERE id = $1`, mcq)
			_, _ = pool.Exec(ctx, `UPDATE questions SET points = 10 WHERE id = $1`, essay)
		}()

		after := marking(stu)
		assert.Equal(t, before.Paper.TotalHalf, after.Paper.TotalHalf)
		assert.Equal(t, before.Paper.MaxHalf, after.Paper.MaxHalf)
		assert.Equal(t, 19, after.Paper.TotalHalf)
		assert.Equal(t, 24, after.Paper.MaxHalf)
	})

	t.Run("finalising again is a harmless repeat", func(t *testing.T) {
		before := audits("grading.paper.finalise")
		m, err := finalise(stu, sess, 0, owner)
		require.NoError(t, err)
		assert.Equal(t, grading.StatusFinal, m.Paper.Status)
		assert.Equal(t, before, audits("grading.paper.finalise"))
		assert.Equal(t, 1, events(sess, "FINALISED"))
	})

	t.Run("another teacher cannot reopen, and cannot tell the paper exists", func(t *testing.T) {
		_, err := reopen(stu, sess, "let me in please", stranger)
		assert.ErrorIs(t, err, grading.ErrExamNotFound)
		assert.Equal(t, 0, events(sess, "REOPENED"))
	})

	t.Run("reopening needs a reason", func(t *testing.T) {
		_, err := reopen(stu, sess, "no", owner)
		assert.ErrorIs(t, err, grading.ErrInvalidRequest)
		assert.Equal(t, 0, events(sess, "REOPENED"))
	})

	t.Run("an administrator reopens it: event, reason and audit entry are kept, the old snapshot too", func(t *testing.T) {
		m, err := reopen(stu, sess, "  appeal upheld for question 2  ", admin)
		require.NoError(t, err)
		assert.Equal(t, grading.StatusDraft, m.Paper.Status)

		var reason string
		var actor uuid.UUID
		var total int
		var snaps int
		require.NoError(t, pool.QueryRow(ctx, `SELECT reason, actor_id, total_half, (snapshot IS NOT NULL)::int
			FROM paper_grading_events WHERE session_id = $1 AND event = 'REOPENED'`, sess).Scan(&reason, &actor, &total, &snaps))
		assert.Equal(t, "appeal upheld for question 2", reason)
		assert.Equal(t, adminID, actor)
		assert.Equal(t, 19, total, "what had been final is remembered")
		assert.Equal(t, 1, snaps)
		assert.Equal(t, 1, audits("grading.paper.reopen"))

		var leaked int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM audit_events
			WHERE action = 'grading.paper.reopen' AND metadata::text ILIKE '%appeal%'`).Scan(&leaked))
		assert.Equal(t, 0, leaked, "the reason is in the history table, not the audit log")

		var stored int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM paper_gradings
			WHERE session_id = $1 AND status = 'DRAFT' AND snapshot IS NULL AND finalised_at IS NULL`, sess).Scan(&stored))
		assert.Equal(t, 1, stored)
	})

	t.Run("a reopened paper can be marked and finalised again", func(t *testing.T) {
		m := marking(stu)
		_, err := save(stu, sess, 20, m.Lines[1].Version)
		require.NoError(t, err)
		m = marking(stu)
		fm, err := finalise(stu, sess, m.Paper.Version, owner)
		require.NoError(t, err)
		assert.Equal(t, 24, fm.Paper.TotalHalf)
		assert.Equal(t, 2, events(sess, "FINALISED"))
	})

	t.Run("a stale paper version cannot finalise", func(t *testing.T) {
		s2, sess2 := newStudent("R003", "SUBMITTED")
		res, err := save(s2, sess2, 8, 0)
		require.NoError(t, err)
		_, err = finalise(s2, sess2, res.Paper.Version-1, owner)
		assert.ErrorIs(t, err, grading.ErrStaleVersion)
		assert.Equal(t, 0, events(sess2, "FINALISED"))
	})

	t.Run("an expired attempt can be finalised, one still in progress cannot", func(t *testing.T) {
		exp, expSess := newStudent("R004", "EXPIRED")
		res, err := save(exp, expSess, 6, 0)
		require.NoError(t, err)
		_, err = finalise(exp, expSess, res.Paper.Version, owner)
		assert.NoError(t, err)

		live, liveSess := newStudent("R005", "IN_PROGRESS")
		_, err = finalise(live, liveSess, 0, owner)
		assert.ErrorIs(t, err, grading.ErrSessionNotEnded)
	})

	t.Run("a different attempt is refused", func(t *testing.T) {
		s3, sess3 := newStudent("R006", "SUBMITTED")
		_, err := finalise(s3, uuid.New(), 0, owner)
		assert.ErrorIs(t, err, grading.ErrSessionChanged)
		_ = sess3
	})

	t.Run("a save racing a finalise ends consistently: the mark is in the snapshot or the save is refused", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			rs, rsess := newStudent("R1"+string(rune('0'+i))+"0", "SUBMITTED")
			first, err := save(rs, rsess, 10, 0)
			require.NoError(t, err)

			var wg sync.WaitGroup
			var saveErr, finErr error
			start := make(chan struct{})
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				_, saveErr = save(rs, rsess, 20, first.Line.Version)
			}()
			go func() {
				defer wg.Done()
				<-start
				_, finErr = finalise(rs, rsess, first.Paper.Version, owner)
			}()
			close(start)
			wg.Wait()

			var status string
			var total int
			err = pool.QueryRow(ctx, `SELECT status, COALESCE(total_half, 0) FROM paper_gradings WHERE session_id = $1`, rsess).Scan(&status, &total)
			require.NoError(t, err)
			switch {
			case saveErr == nil && finErr == nil:
				// The save landed first, so the finalise (at the older paper version)
				// would have been refused. Both succeeding means the finalise ran
				// first and the save then found the paper final: that is an error.
				t.Fatalf("both succeeded: save and finalise cannot both win (status %s)", status)
			case saveErr == nil:
				assert.ErrorIs(t, finErr, grading.ErrStaleVersion)
				assert.Equal(t, "DRAFT", status)
			default:
				require.NoError(t, finErr)
				assert.ErrorIs(t, saveErr, grading.ErrPaperFinalised)
				assert.Equal(t, "FINAL", status)
				assert.Equal(t, 14, total, "the snapshot holds the mark that was there when it froze: 10 + the auto MCQ 4")
			}
		}
	})
}
