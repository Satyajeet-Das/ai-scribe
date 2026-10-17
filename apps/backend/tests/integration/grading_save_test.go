package integration

import (
	"context"
	"errors"
	"strings"
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

func TestGradingSave_Integration(t *testing.T) {
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
	mcq := seed.question(examID, 1, "MCQ", "Pick one", 2)
	right := seed.option(mcq, "A", "right", 1, true)
	essays := []uuid.UUID{
		seed.question(examID, 2, "ESSAY", "Explain gravity", 10),
		seed.question(examID, 3, "ESSAY", "Explain light", 5),
		seed.question(examID, 4, "ESSAY", "Explain heat", 4),
	}
	otherExam := seed.exam(teacher, "Chemistry")
	foreignQuestion := seed.question(otherExam, 1, "ESSAY", "Explain bonding", 10)

	newStudent := func(roll, status string) (student, session uuid.UUID) {
		student = seed.user("STUDENT", "S"+roll, "Student", roll)
		session = seed.session(seed.assign(examID, student, "ASSIGNED"), examID, student, status, time.Now().Add(-time.Hour))
		seed.answer(session, mcq, &right, "")
		for _, q := range essays {
			seed.answer(session, q, nil, "The student's own words about it")
		}
		return
	}
	alice, aSess := newStudent("R001", "SUBMITTED")

	examSvc := exam.NewService(exam.NewRepository(pool), &nop)
	svc := grading.NewService(grading.NewRepository(pool), examSvc, &nop)
	owner := grading.Caller{ID: teacher, Role: platformauth.RoleTeacher}
	meta := grading.AccessMeta{RequestID: "req-int", IP: "203.0.113.8"}

	save := func(student, session, question uuid.UUID, marks *int, comment *string, version int, caller grading.Caller) (*grading.SaveMarkResult, error) {
		return svc.SaveMark(ctx, examID, student, question, grading.SaveMarkRequest{
			SessionID: session, MarksHalf: marks, Comment: comment, ExpectedVersion: version,
		}, caller, meta)
	}
	half := func(i int) *int { return &i }
	str := func(s string) *string { return &s }

	count := func(sql string, args ...any) int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, sql, args...).Scan(&n))
		return n
	}
	paperVersion := func(session uuid.UUID) int {
		var v *int
		require.NoError(t, pool.QueryRow(ctx, `SELECT (SELECT version FROM paper_gradings WHERE session_id = $1)`, session).Scan(&v))
		if v == nil {
			return -1
		}
		return *v
	}
	markVersion := func(session, q uuid.UUID) int {
		return count(`SELECT COALESCE((SELECT version FROM answer_marks WHERE session_id = $1 AND question_id = $2), 0)`, session, q)
	}
	audits := func(action string) int { return count(`SELECT count(*) FROM audit_events WHERE action = $1`, action) }

	t.Run("a first save creates the mark, the paper's record and an audit entry together", func(t *testing.T) {
		require.Equal(t, -1, paperVersion(aSess), "no grading record before the first write")

		res, err := save(alice, aSess, essays[0], half(15), str("Clear and complete."), 0, owner)
		require.NoError(t, err)
		assert.Equal(t, 15, *res.Line.EffectiveHalf)
		assert.Equal(t, 1, res.Line.Version)
		assert.Equal(t, 1, res.Paper.Version)

		var marks, maxHalf, version int
		var comment string
		var by uuid.UUID
		require.NoError(t, pool.QueryRow(ctx, `SELECT marks_half, max_half, comment, version, marked_by FROM answer_marks
			WHERE session_id = $1 AND question_id = $2`, aSess, essays[0]).Scan(&marks, &maxHalf, &comment, &version, &by))
		assert.Equal(t, []any{15, 20, "Clear and complete.", 1, teacher}, []any{marks, maxHalf, comment, version, by})
		assert.Equal(t, 1, paperVersion(aSess))
		assert.Equal(t, 1, audits("grading.mark.save"))
	})

	t.Run("later saves raise the version; the paper's version moves with every change", func(t *testing.T) {
		res, err := save(alice, aSess, essays[0], half(17), nil, 1, owner)
		require.NoError(t, err)
		assert.Equal(t, 2, res.Line.Version)
		assert.Nil(t, res.Line.Comment, "the replacement had no comment, so the comment is cleared")
		assert.Equal(t, 2, paperVersion(aSess))
	})

	t.Run("a save based on an old version is refused and changes nothing", func(t *testing.T) {
		_, err := save(alice, aSess, essays[0], half(3), nil, 1, owner)
		assert.ErrorIs(t, err, grading.ErrStaleVersion)
		assert.Equal(t, 2, markVersion(aSess, essays[0]))
		assert.Equal(t, 2, paperVersion(aSess))
		assert.Equal(t, 2, audits("grading.mark.save"), "a refused save is not a change")
	})

	t.Run("repeating a save that already went through is harmless", func(t *testing.T) {
		before := audits("grading.mark.save")
		res, err := save(alice, aSess, essays[0], half(17), nil, 1, owner) // the lost reply was for version 1 -> 2
		require.NoError(t, err)
		assert.Equal(t, 2, res.Line.Version)
		assert.Equal(t, before, audits("grading.mark.save"))
		assert.Equal(t, 2, paperVersion(aSess))
	})

	t.Run("eight teachers saving the same answer at once: one wins, seven are told it changed", func(t *testing.T) {
		const n = 8
		var wg sync.WaitGroup
		errs := make([]error, n)
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, errs[i] = save(alice, aSess, essays[0], half(i+1), nil, 2, owner)
			}()
		}
		close(start)
		wg.Wait()

		won, stale := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				won++
			case errors.Is(err, grading.ErrStaleVersion):
				stale++
			default:
				t.Fatalf("unexpected error: %v", err)
			}
		}
		assert.Equal(t, 1, won)
		assert.Equal(t, n-1, stale)
		assert.Equal(t, 3, markVersion(aSess, essays[0]), "the version rose once")
		assert.Equal(t, 3, paperVersion(aSess))
		assert.Equal(t, 3, audits("grading.mark.save"))
	})

	t.Run("the very first writes to a paper, racing, do not collide", func(t *testing.T) {
		bob, bSess := newStudent("R002", "SUBMITTED")
		require.Equal(t, -1, paperVersion(bSess))

		// Two teachers per question, three questions, nothing saved yet.
		const n = 6
		var wg sync.WaitGroup
		errs := make([]error, n)
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, errs[i] = save(bob, bSess, essays[i%3], half(i+1), nil, 0, owner)
			}()
		}
		close(start)
		wg.Wait()

		won := 0
		for _, err := range errs {
			if err == nil {
				won++
			} else {
				assert.ErrorIs(t, err, grading.ErrStaleVersion)
			}
		}
		assert.Equal(t, 3, won, "exactly one save per question")
		assert.Equal(t, 3, paperVersion(bSess), "and the paper's record exists once, at the number of changes")
		assert.Equal(t, 1, count(`SELECT count(*) FROM paper_gradings WHERE session_id = $1`, bSess))
	})

	t.Run("an audit entry that cannot be written takes the mark with it", func(t *testing.T) {
		cara, cSess := newStudent("R003", "SUBMITTED")
		_, err := pool.Exec(ctx, `ALTER TABLE audit_events RENAME TO audit_events_away`)
		require.NoError(t, err)
		defer func() {
			_, err := pool.Exec(ctx, `ALTER TABLE audit_events_away RENAME TO audit_events`)
			require.NoError(t, err)
		}()

		_, err = save(cara, cSess, essays[0], half(10), str("never saved"), 0, owner)
		assert.ErrorIs(t, err, grading.ErrAuditUnavailable)

		assert.Zero(t, count(`SELECT count(*) FROM answer_marks WHERE session_id = $1`, cSess), "no mark without its record")
		assert.Equal(t, -1, paperVersion(cSess), "not even the paper's grading record")
	})

	t.Run("nothing invalid is written", func(t *testing.T) {
		dan, dSess := newStudent("R004", "SUBMITTED")
		before := audits("grading.mark.save")

		_, err := save(dan, dSess, essays[1], half(11), nil, 0, owner) // 5.5 of 5
		assert.ErrorIs(t, err, grading.ErrInvalidRequest)
		_, err = save(dan, dSess, essays[1], half(-1), nil, 0, owner)
		assert.ErrorIs(t, err, grading.ErrInvalidRequest)
		_, err = save(dan, dSess, essays[1], half(4), str(strings.Repeat("x", 2001)), 0, owner)
		assert.ErrorIs(t, err, grading.ErrInvalidRequest)
		_, err = save(dan, dSess, essays[1], half(4), str("bad\x00text"), 0, owner)
		assert.ErrorIs(t, err, grading.ErrInvalidRequest)

		assert.Zero(t, count(`SELECT count(*) FROM answer_marks WHERE session_id = $1`, dSess))
		assert.Equal(t, before, audits("grading.mark.save"))

		res, err := save(dan, dSess, essays[1], half(10), nil, 0, owner) // the maximum, 5 of 5
		require.NoError(t, err)
		assert.Equal(t, 10, *res.Line.EffectiveHalf)
	})

	t.Run("a multiple-choice mark can be overridden and cleared", func(t *testing.T) {
		eve, eSess := newStudent("R005", "SUBMITTED")
		res, err := save(eve, eSess, mcq, half(1), str("half credit for the working"), 0, owner)
		require.NoError(t, err)
		assert.Equal(t, 1, *res.Line.EffectiveHalf)
		assert.Equal(t, grading.SourceTeacher, res.Line.Source)

		res, err = save(eve, eSess, mcq, nil, nil, 1, owner)
		require.NoError(t, err)
		assert.Equal(t, 4, *res.Line.EffectiveHalf, "back to the automatic mark")
		assert.Equal(t, grading.SourceAuto, res.Line.Source)
		assert.Equal(t, 2, markVersion(eSess, mcq), "the cleared mark keeps its place in the version sequence")
	})

	t.Run("only a paper that has ended can be marked", func(t *testing.T) {
		fay, fSess := newStudent("R006", "IN_PROGRESS")
		_, err := save(fay, fSess, essays[0], half(4), nil, 0, owner)
		assert.ErrorIs(t, err, grading.ErrSessionNotEnded)

		gus, gSess := newStudent("R007", "EXPIRED")
		_, err = save(gus, gSess, essays[0], half(4), nil, 0, owner)
		assert.NoError(t, err, "an expired attempt can be marked")

		never := seed.user("STUDENT", "Nora", "Never", "R008")
		seed.assign(examID, never, "ASSIGNED")
		_, err = save(never, uuid.New(), essays[0], half(4), nil, 0, owner)
		assert.ErrorIs(t, err, grading.ErrSessionNotEnded)

		assert.Zero(t, count(`SELECT count(*) FROM answer_marks WHERE session_id = $1`, fSess))
	})

	t.Run("a mark goes to the attempt the teacher was looking at, and to no other", func(t *testing.T) {
		hal, hSess := newStudent("R009", "SUBMITTED")
		var assignment uuid.UUID
		require.NoError(t, pool.QueryRow(ctx, `SELECT assignment_id FROM sessions WHERE id = $1`, hSess).Scan(&assignment))
		retake := seed.session(assignment, examID, hal, "IN_PROGRESS", time.Now())

		_, err := save(hal, retake, essays[0], half(4), nil, 0, owner)
		assert.ErrorIs(t, err, grading.ErrSessionChanged, "the retake is not the paper being shown")
		_, err = save(hal, uuid.New(), essays[0], half(4), nil, 0, owner)
		assert.ErrorIs(t, err, grading.ErrSessionChanged)
		assert.Zero(t, count(`SELECT count(*) FROM answer_marks`+" WHERE session_id IN ($1, $2)", hSess, retake))

		_, err = save(hal, hSess, essays[0], half(4), nil, 0, owner)
		assert.NoError(t, err)
	})

	t.Run("a question from another exam is not found", func(t *testing.T) {
		ian, iSess := newStudent("R010", "SUBMITTED")
		_, err := save(ian, iSess, foreignQuestion, half(4), nil, 0, owner)
		assert.ErrorIs(t, err, grading.ErrExamNotFound)
		_, err = save(ian, iSess, uuid.New(), half(4), nil, 0, owner)
		assert.ErrorIs(t, err, grading.ErrExamNotFound)
	})

	t.Run("who may mark", func(t *testing.T) {
		jo, jSess := newStudent("R011", "SUBMITTED")
		_, err := save(jo, jSess, essays[0], half(4), nil, 0, grading.Caller{ID: otherTeacher, Role: platformauth.RoleTeacher})
		assert.ErrorIs(t, err, grading.ErrExamNotFound)
		_, err = save(jo, jSess, essays[0], half(4), nil, 0, grading.Caller{ID: studentRole, Role: platformauth.RoleStudent})
		assert.ErrorIs(t, err, grading.ErrForbiddenRole)
		assert.Zero(t, count(`SELECT count(*) FROM answer_marks WHERE session_id = $1`, jSess))

		res, err := save(jo, jSess, essays[0], half(4), nil, 0, grading.Caller{ID: adminID, Role: platformauth.RoleAdmin})
		require.NoError(t, err, "an administrator may")
		assert.Equal(t, 4, *res.Line.EffectiveHalf)
		var role string
		require.NoError(t, pool.QueryRow(ctx, `SELECT actor_role FROM audit_events WHERE actor_id = $1`, adminID).Scan(&role))
		assert.Equal(t, "ADMIN", role)
	})

	t.Run("a finalised paper cannot be changed", func(t *testing.T) {
		kim, kSess := newStudent("R012", "SUBMITTED")
		_, err := save(kim, kSess, essays[0], half(8), nil, 0, owner)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `UPDATE paper_gradings SET status = 'FINAL', finalised_by = $2, finalised_at = NOW(),
			total_half = 8, max_half = 8, snapshot = '{"version":1,"lines":[]}'::jsonb WHERE session_id = $1`, kSess, teacher)
		require.NoError(t, err)
		before := paperVersion(kSess)

		_, err = save(kim, kSess, essays[0], half(2), nil, 1, owner)
		assert.ErrorIs(t, err, grading.ErrPaperFinalised)
		_, err = save(kim, kSess, essays[1], half(2), nil, 0, owner)
		assert.ErrorIs(t, err, grading.ErrPaperFinalised)
		assert.Equal(t, 1, markVersion(kSess, essays[0]))
		assert.Equal(t, before, paperVersion(kSess))
	})

	t.Run("the audit trail records who and what, never the comment or the answer", func(t *testing.T) {
		secret := "plagiarised from the textbook"
		lee, lSess := newStudent("R013", "SUBMITTED")
		_, err := save(lee, lSess, essays[0], half(6), str(secret), 0, owner)
		require.NoError(t, err)

		var actor, resource, session uuid.UUID
		var requestID, ip, metadata string
		require.NoError(t, pool.QueryRow(ctx, `
			SELECT actor_id, resource_id, session_id, request_id, host(ip), metadata::text FROM audit_events
			WHERE action = 'grading.mark.save' AND session_id = $1`, lSess).Scan(&actor, &resource, &session, &requestID, &ip, &metadata))
		assert.Equal(t, teacher, actor)
		assert.Equal(t, essays[0], resource)
		assert.Equal(t, "req-int", requestID)
		assert.Equal(t, "203.0.113.8", ip)
		assert.Contains(t, metadata, `"marks_half": 6`)
		assert.Contains(t, metadata, `"comment_changed": true`)
		assert.NotContains(t, metadata, "plagiarised")
		assert.NotContains(t, metadata, "student's own words")
	})

	t.Run("marking reads back as saved, and the review of answers is unchanged by it", func(t *testing.T) {
		m, err := svc.GetMarking(ctx, examID, alice, owner)
		require.NoError(t, err)
		assert.Equal(t, grading.StatusDraft, m.Paper.Status)
		var answers int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM answers WHERE session_id = $1 AND text_answer <> ''`, aSess).Scan(&answers))
		assert.Equal(t, 3, answers, "marking never touches the student's answers")
	})
}
