package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/audit"
)

func TestAuditRecordTx_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	testDB, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pool := testDB.Pool
	seed := reviewSeed{pool: pool, ctx: ctx, t: t}
	teacher := seed.user("TEACHER", "Terry", "Teacher", "")

	countBy := func(action string) int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action = $1`, action).Scan(&n))
		return n
	}
	event := func() audit.Event {
		return audit.Event{
			ActorID: teacher, ActorRole: "TEACHER", Action: audit.ActionMarkSave, ResourceType: audit.ResourceAnswerMark,
			ResourceID: uuid.New(), SessionID: uuid.New(), RequestID: "req-1", IP: "203.0.113.5",
			Metadata: map[string]any{"marks_half": 15, "max_half": 20, "comment_changed": true},
		}
	}

	t.Run("an event written in a transaction that commits is kept", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, audit.RecordTx(ctx, tx, event()))
		require.NoError(t, tx.Commit(ctx))
		assert.Equal(t, 1, countBy("grading.mark.save"))

		var actor uuid.UUID
		var resourceType, ip, meta string
		require.NoError(t, pool.QueryRow(ctx, `
			SELECT actor_id, resource_type, host(ip), metadata::text FROM audit_events WHERE action = 'grading.mark.save'`,
		).Scan(&actor, &resourceType, &ip, &meta))
		assert.Equal(t, teacher, actor)
		assert.Equal(t, "answer_mark", resourceType)
		assert.Equal(t, "203.0.113.5", ip)
		assert.JSONEq(t, `{"marks_half":15,"max_half":20,"comment_changed":true}`, meta)
	})

	t.Run("an event written in a transaction that rolls back leaves no trace", func(t *testing.T) {
		before := countBy("grading.mark.save")
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, audit.RecordTx(ctx, tx, event()))
		require.NoError(t, tx.Rollback(ctx))
		assert.Equal(t, before, countBy("grading.mark.save"), "the change and its record come apart never")
	})

	t.Run("a failed event poisons the transaction so the change cannot commit without its record", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		bad := event()
		bad.Action = strings.Repeat("x", 61) // longer than the 60-character column
		require.Error(t, audit.RecordTx(ctx, tx, bad))
		assert.Error(t, tx.Commit(ctx), "PostgreSQL refuses to commit a transaction that hit an error")
	})

	t.Run("the pool-based recorder behaves as before", func(t *testing.T) {
		before := countBy("grading.paper.finalise")
		e := event()
		e.Action = audit.ActionPaperFinalise
		e.ResourceType = audit.ResourcePaper
		require.NoError(t, audit.NewRecorder(pool).Record(ctx, e))
		assert.Equal(t, before+1, countBy("grading.paper.finalise"))
	})

	t.Run("an unusable address and a long request id are handled the same way in both", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		e := event()
		e.Action, e.IP, e.RequestID = audit.ActionPaperReopen, "not-an-ip", strings.Repeat("r", 200)
		require.NoError(t, audit.RecordTx(ctx, tx, e))
		require.NoError(t, tx.Commit(ctx))

		var ip *string
		var rid string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT host(ip), request_id FROM audit_events WHERE action = 'grading.paper.reopen'`).Scan(&ip, &rid))
		assert.Nil(t, ip)
		assert.Len(t, rid, 64)
	})
}
