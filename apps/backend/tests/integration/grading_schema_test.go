package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/migrations"
)

// rejectedBy asserts that err is a check-constraint violation of the named
// constraint, so a test cannot pass because of some unrelated error.
func rejectedBy(t *testing.T, err error, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a database error, got %v", err)
	assert.Equal(t, "23514", pgErr.Code, "expected a check violation: %s", pgErr.Message)
	assert.Equal(t, constraint, pgErr.ConstraintName)
}

func TestGradingSchema_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	testDB, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	seed := reviewSeed{pool: testDB.Pool, ctx: ctx, t: t}
	pool := testDB.Pool

	teacher := seed.user("TEACHER", "Terry", "Teacher", "")
	student := seed.user("STUDENT", "Sam", "Student", "R100")
	examID := seed.exam(teacher, "Biology")
	essay := seed.question(examID, 1, "ESSAY", "Explain photosynthesis", 10)
	assignment := seed.assign(examID, student, "ASSIGNED")
	sessionID := seed.session(assignment, examID, student, "SUBMITTED", time.Now())

	exec := func(sql string, args ...any) error {
		_, err := pool.Exec(ctx, sql, args...)
		return err
	}
	count := func(table string, where string, args ...any) int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE "+where, args...).Scan(&n))
		return n
	}

	t.Run("a paper starts as a draft with version 0", func(t *testing.T) {
		require.NoError(t, exec(`INSERT INTO paper_gradings (session_id, exam_id) VALUES ($1, $2)`, sessionID, examID))
		var status string
		var version int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT status, version FROM paper_gradings WHERE session_id = $1`, sessionID).Scan(&status, &version))
		assert.Equal(t, "DRAFT", status)
		assert.Zero(t, version)
		assert.Error(t, exec(`INSERT INTO paper_gradings (session_id, exam_id) VALUES ($1, $2)`, sessionID, examID),
			"one row per session")
	})

	t.Run("a paper is final only when everything a final paper needs is there", func(t *testing.T) {
		finalise := `UPDATE paper_gradings SET status = 'FINAL', finalised_by = $2, finalised_at = NOW(),
		             total_half = $3, max_half = $4, snapshot = $5::jsonb WHERE session_id = $1`

		rejectedBy(t, exec(finalise, sessionID, teacher, 10, 20, nil), "chk_final_is_complete") // no snapshot
		rejectedBy(t, exec(`UPDATE paper_gradings SET status = 'FINAL' WHERE session_id = $1`, sessionID), "chk_final_is_complete")
		rejectedBy(t, exec(finalise, sessionID, teacher, 25, 20, `{"lines":[]}`), "chk_total_within_max")

		require.NoError(t, exec(finalise, sessionID, teacher, 15, 20, `{"lines":[]}`))
		assert.Equal(t, 1, count("paper_gradings", "session_id = $1 AND status = 'FINAL'", sessionID))

		// Reopening clears all of it at once; a draft cannot keep a half-finished finalisation.
		rejectedBy(t, exec(`UPDATE paper_gradings SET status = 'DRAFT' WHERE session_id = $1`, sessionID), "chk_final_is_complete")
		require.NoError(t, exec(`UPDATE paper_gradings SET status = 'DRAFT', finalised_by = NULL, finalised_at = NULL,
		                         total_half = NULL, max_half = NULL, snapshot = NULL WHERE session_id = $1`, sessionID))
	})

	t.Run("an unknown status is refused", func(t *testing.T) {
		assert.Error(t, exec(`UPDATE paper_gradings SET status = 'PUBLISHED' WHERE session_id = $1`, sessionID))
	})

	mark := func(marks any, max int, comment any, version int) error {
		return exec(`INSERT INTO answer_marks (session_id, question_id, marks_half, max_half, comment, version, marked_by)
		             VALUES ($1, $2, $3, $4, $5, $6, $7)
		             ON CONFLICT (session_id, question_id) DO UPDATE
		             SET marks_half = EXCLUDED.marks_half, max_half = EXCLUDED.max_half,
		                 comment = EXCLUDED.comment, version = EXCLUDED.version`,
			sessionID, essay, marks, max, comment, version, teacher)
	}

	t.Run("half marks from zero up to the maximum are accepted", func(t *testing.T) {
		for _, half := range []int{0, 1, 15, 20} { // 0, 0.5, 7.5, 10 marks
			require.NoError(t, mark(half, 20, nil, 1), half)
		}
		var stored int
		require.NoError(t, pool.QueryRow(ctx, `SELECT marks_half FROM answer_marks WHERE session_id = $1`, sessionID).Scan(&stored))
		assert.Equal(t, 20, stored)
	})

	t.Run("a mark can be cleared without losing its place in the version sequence", func(t *testing.T) {
		require.NoError(t, mark(nil, 20, "comment only", 2))
		var marks *int
		var version int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT marks_half, version FROM answer_marks WHERE session_id = $1`, sessionID).Scan(&marks, &version))
		assert.Nil(t, marks)
		assert.Equal(t, 2, version)
	})

	t.Run("marks outside the allowed range are refused", func(t *testing.T) {
		rejectedBy(t, mark(21, 20, nil, 3), "chk_marks_within_max") // 10.5 out of 10
		rejectedBy(t, mark(-1, 20, nil, 3), "chk_marks_within_max")
		rejectedBy(t, mark(1, 0, nil, 3), "chk_marks_within_max") // a 0-point question takes only 0
	})

	t.Run("a mark of zero is fine for a zero-point question", func(t *testing.T) {
		require.NoError(t, mark(0, 0, nil, 3))
	})

	t.Run("comments are bounded", func(t *testing.T) {
		require.NoError(t, mark(10, 20, strings.Repeat("x", 2000), 4))
		assert.Error(t, mark(10, 20, strings.Repeat("x", 2001), 5))
	})

	t.Run("a version below 1 is refused", func(t *testing.T) {
		assert.Error(t, mark(10, 20, nil, 0))
	})

	t.Run("marks are stored as whole numbers of half-marks", func(t *testing.T) {
		// PostgreSQL rounds a fraction into an integer column instead of refusing
		// it, so the guarantee is the column type plus the application only ever
		// passing integers (and the API parser refusing 7.25 before it gets here).
		for table, column := range map[string]string{
			"answer_marks": "marks_half", "paper_gradings": "total_half", "paper_grading_events": "total_half",
		} {
			var dataType string
			require.NoError(t, pool.QueryRow(ctx, `SELECT data_type FROM information_schema.columns
				WHERE table_name = $1 AND column_name = $2`, table, column).Scan(&dataType))
			assert.Equal(t, "integer", dataType, "%s.%s", table, column)
		}
	})

	t.Run("every reopening carries a reason", func(t *testing.T) {
		event := func(kind string, reason any) error {
			return exec(`INSERT INTO paper_grading_events (session_id, exam_id, event, actor_id, actor_role, reason, total_half, max_half)
			             VALUES ($1, $2, $3, $4, 'TEACHER', $5, 15, 20)`, sessionID, examID, kind, teacher, reason)
		}
		require.NoError(t, event("FINALISED", nil))
		rejectedBy(t, event("REOPENED", nil), "chk_reopen_has_reason")
		rejectedBy(t, event("REOPENED", "  "), "chk_reopen_has_reason")
		rejectedBy(t, event("REOPENED", "ok"), "chk_reopen_has_reason")
		require.NoError(t, event("REOPENED", "regrade after appeal"))
		assert.Error(t, event("REOPENED", strings.Repeat("x", 501)))
		assert.Error(t, event("DELETED", "reason"), "only the two known events exist")
		assert.Equal(t, 2, count("paper_grading_events", "session_id = $1", sessionID))
	})

	t.Run("the marks of a session go when the session goes", func(t *testing.T) {
		other := seed.user("STUDENT", "Cam", "Other", "R101")
		otherSession := seed.session(seed.assign(examID, other, "ASSIGNED"), examID, other, "SUBMITTED", time.Now())
		require.NoError(t, exec(`INSERT INTO paper_gradings (session_id, exam_id) VALUES ($1, $2)`, otherSession, examID))
		require.NoError(t, exec(`INSERT INTO answer_marks (session_id, question_id, marks_half, max_half, version, marked_by)
		                         VALUES ($1, $2, 4, 20, 1, $3)`, otherSession, essay, teacher))
		require.NoError(t, exec(`INSERT INTO paper_grading_events (session_id, exam_id, event, actor_id) VALUES ($1, $2, 'FINALISED', $3)`,
			otherSession, examID, teacher))

		require.NoError(t, exec(`DELETE FROM sessions WHERE id = $1`, otherSession))
		assert.Zero(t, count("paper_gradings", "session_id = $1", otherSession))
		assert.Zero(t, count("answer_marks", "session_id = $1", otherSession))
		assert.Zero(t, count("paper_grading_events", "session_id = $1", otherSession))
		assert.Equal(t, 1, count("paper_gradings", "session_id = $1", sessionID), "other papers are untouched")
	})

	t.Run("a mark cannot point at a student who does not exist", func(t *testing.T) {
		assert.Error(t, exec(`INSERT INTO answer_marks (session_id, question_id, marks_half, max_half, version, marked_by)
		                      VALUES ($1, $2, 2, 20, 1, $3)`, sessionID, uuid.New(), teacher), "unknown question")
		assert.Error(t, exec(`INSERT INTO answer_marks (session_id, question_id, marks_half, max_half, version, marked_by)
		                      VALUES ($1, $2, 2, 20, 1, $3)`, sessionID, essay, uuid.New()), "unknown marker")
	})

	t.Run("the migration can be undone and applied again", func(t *testing.T) {
		raw, err := migrations.FS.ReadFile("008_grading.sql")
		require.NoError(t, err)
		up, down, found := strings.Cut(string(raw), "---- create above / drop below ----")
		require.True(t, found, "the migration has both halves")

		exists := func(table string) bool {
			var name *string
			require.NoError(t, pool.QueryRow(ctx, `SELECT to_regclass($1)::text`, "public."+table).Scan(&name))
			return name != nil
		}
		tables := []string{"paper_gradings", "answer_marks", "paper_grading_events"}

		require.NoError(t, exec(down))
		for _, table := range tables {
			assert.False(t, exists(table), "%s is gone after the down migration", table)
		}
		require.NoError(t, exec(up))
		for _, table := range tables {
			assert.True(t, exists(table), "%s is back after applying the migration again", table)
		}
		assert.NoError(t, exec(up), "applying it twice is harmless")
	})
}
