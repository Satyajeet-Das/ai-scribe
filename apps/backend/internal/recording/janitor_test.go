package recording

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/audit"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
)

var epoch = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

// world is a store, a database and an audit log that share one ordered record
// of what happened, so a test can say not just what was done but in what order.
type world struct {
	mu    sync.Mutex
	steps []string

	store *spyStore
	rows  map[uuid.UUID]*row
	audit *spyAudit

	events []audit.Event

	locked     bool // another instance holds the cleanup lock
	markErr    error
	deleteErr  error
	auditErr   error
	markCalled int
}

type row struct {
	id, session uuid.UUID
	key         string
	retention   *time.Time
	deleted     bool
	// confirmed means the recording supports a confirmed answer.
	confirmed bool
	madeAt    time.Time
}

type spyStore struct {
	*audiostore.Memory
	w *world
}

func (s *spyStore) Delete(ctx context.Context, key string) error {
	if s.w.deleteErr != nil {
		return s.w.deleteErr
	}
	s.w.step("delete-object")
	return s.Memory.Delete(ctx, key)
}

type spyAudit struct{ w *world }

func (a *spyAudit) Record(_ context.Context, e audit.Event) error {
	if a.w.auditErr != nil {
		return a.w.auditErr
	}
	a.w.step("audit:" + e.Action)
	a.w.events = append(a.w.events, e)
	return nil
}

func (w *world) step(s string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.steps = append(w.steps, s)
}

type repoView struct{ w *world }

func (r repoView) TryLock(context.Context) (func(), bool, error) {
	if r.w.locked {
		return nil, false, nil
	}
	return func() {}, true, nil
}

func (r repoView) Expired(_ context.Context, before time.Time, limit int) ([]Expired, error) {
	var out []Expired
	for _, x := range r.w.rows {
		if !x.deleted && x.retention != nil && !x.retention.After(before) {
			out = append(out, Expired{ID: x.id, SessionID: x.session, StorageKey: x.key})
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r repoView) Unconfirmed(_ context.Context, madeBefore time.Time, limit int) ([]Expired, error) {
	var out []Expired
	for _, x := range r.w.rows {
		if !x.deleted && !x.confirmed && !x.madeAt.After(madeBefore) {
			out = append(out, Expired{ID: x.id, SessionID: x.session, StorageKey: x.key})
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r repoView) MarkDeleted(_ context.Context, id uuid.UUID, _ time.Time) (bool, error) {
	r.w.markCalled++
	if r.w.markErr != nil {
		return false, r.w.markErr
	}
	x := r.w.rows[id]
	if x == nil || x.deleted {
		return false, nil
	}
	r.w.step("mark-deleted")
	x.deleted = true
	return true, nil
}

func (r repoView) StoredKeys(context.Context) (map[string]uuid.UUID, error) {
	out := map[string]uuid.UUID{}
	for _, x := range r.w.rows {
		if !x.deleted {
			out[x.key] = x.id
		}
	}
	return out, nil
}

func newWorld() *world {
	w := &world{rows: map[uuid.UUID]*row{}}
	w.store = &spyStore{Memory: audiostore.NewMemory(), w: w}
	w.audit = &spyAudit{w: w}
	return w
}

// events is kept outside the struct literal above only to keep it short.
func (w *world) eventsOf(action string) []audit.Event {
	var out []audit.Event
	for _, e := range w.events {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

// add stores audio and, unless orphan is true, the row that refers to it.
func (w *world) add(t *testing.T, retention *time.Time, withRow bool) *row {
	t.Helper()
	key := audiostore.NewKey()
	require.NoError(t, w.store.Memory.Put(context.Background(), key, bytes.NewReader([]byte("audio")), 5))
	// By default a recording supports an answer the student confirmed, and was
	// made long ago, so only the rules under test apply to it.
	r := &row{id: uuid.New(), session: uuid.New(), key: key, retention: retention, confirmed: true, madeAt: epoch.Add(-48 * time.Hour)}
	if withRow {
		w.rows[r.id] = r
	}
	return r
}

func at(d time.Duration) *time.Time { t := epoch.Add(d); return &t }

func (w *world) janitor(opts JanitorOptions) *Janitor {
	j := NewJanitor(repoView{w}, w.store, w.audit, opts, nil)
	j.now = func() time.Time { return epoch }
	return j
}

func (w *world) has(key string) bool {
	_, _, err := w.store.Memory.Open(context.Background(), key)
	return err == nil
}

func (w *world) age(key string, by time.Duration) {
	w.store.Memory.SetModified(key, epoch.Add(-by))
}

// ---------------------------------------------------------------------------

func TestPurge_DeletesOnlyWhatIsPastItsRetentionDate(t *testing.T) {
	w := newWorld()
	expired := w.add(t, at(-time.Hour), true)
	justNow := w.add(t, at(0), true) // the date is today's instant: due
	future := w.add(t, at(time.Hour), true)
	noDate := w.add(t, nil, true) // kept until someone decides otherwise

	report, err := w.janitor(JanitorOptions{}).RunOnce(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 2, report.Purged)
	assert.False(t, w.has(expired.key))
	assert.False(t, w.has(justNow.key))
	assert.True(t, w.has(future.key), "not due yet")
	assert.True(t, w.has(noDate.key), "a recording with no retention date is never deleted")
	assert.True(t, expired.deleted && justNow.deleted)
	assert.False(t, future.deleted || noDate.deleted)
}

func TestPurge_IsAuditedBeforeAndRecordedAfterTheAudioIsGone(t *testing.T) {
	w := newWorld()
	r := w.add(t, at(-time.Hour), true)

	_, err := w.janitor(JanitorOptions{}).RunOnce(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{"audit:audio.purged", "delete-object", "mark-deleted"}, w.steps,
		"a deletion is on record before it happens, and the row never claims it before it is true")
	events := w.eventsOf(audit.ActionAudioPurged)
	require.Len(t, events, 1)
	assert.Equal(t, r.id, events[0].ResourceID)
	assert.Equal(t, r.session, events[0].SessionID)
	assert.Equal(t, "SYSTEM", events[0].ActorRole)
	assert.Equal(t, uuid.Nil, events[0].ActorID, "the system did this, not a person")
	assert.Equal(t, "retention_expired", events[0].Metadata["reason"])
}

func TestPurge_AudioIsKeptWhenTheDeletionCannotBeRecorded(t *testing.T) {
	w := newWorld()
	r := w.add(t, at(-time.Hour), true)
	w.auditErr = errors.New("audit database down")

	report, err := w.janitor(JanitorOptions{}).RunOnce(context.Background())
	require.NoError(t, err)
	assert.Zero(t, report.Purged)
	assert.Equal(t, 1, report.PurgeFailed)
	assert.True(t, w.has(r.key), "no unrecorded deletion")
	assert.False(t, r.deleted)

	// Once the audit log is back, the next run finishes the job.
	w.auditErr = nil
	report, err = w.janitor(JanitorOptions{}).RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, report.Purged)
	assert.False(t, w.has(r.key))
}

func TestPurge_AStoreFailureLeavesTheRowSoTheNextRunRetries(t *testing.T) {
	w := newWorld()
	r := w.add(t, at(-time.Hour), true)
	w.deleteErr = audiostore.ErrUnavailable

	report, err := w.janitor(JanitorOptions{}).RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, report.PurgeFailed)
	assert.False(t, r.deleted, "the row must not say the audio is gone while it is not")
	assert.True(t, w.has(r.key))

	w.deleteErr = nil
	report, _ = w.janitor(JanitorOptions{}).RunOnce(context.Background())
	assert.Equal(t, 1, report.Purged)
	assert.True(t, r.deleted)
}

func TestPurge_ACrashBetweenDeletingAndMarkingIsFinishedNextTime(t *testing.T) {
	w := newWorld()
	r := w.add(t, at(-time.Hour), true)
	w.markErr = errors.New("connection lost") // the object is deleted, then the update fails

	report, _ := w.janitor(JanitorOptions{}).RunOnce(context.Background())
	assert.Equal(t, 1, report.PurgeFailed)
	assert.False(t, w.has(r.key))
	assert.False(t, r.deleted)

	w.markErr = nil
	report, _ = w.janitor(JanitorOptions{}).RunOnce(context.Background())
	assert.Equal(t, 1, report.Purged, "deleting audio that is already gone is not an error")
	assert.True(t, r.deleted)
}

func TestPurge_WorksThroughABacklogInBatches(t *testing.T) {
	w := newWorld()
	for i := 0; i < 250; i++ {
		w.add(t, at(-time.Hour), true)
	}
	report, err := w.janitor(JanitorOptions{BatchSize: 100}).RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 250, report.Purged)
	assert.Empty(t, w.store.Memory.Keys(), "every expired recording's audio is gone")
	for _, r := range w.rows {
		assert.True(t, r.deleted)
	}
}

func TestPurge_AStuckBatchDoesNotSpinForever(t *testing.T) {
	w := newWorld()
	for i := 0; i < 5; i++ {
		w.add(t, at(-time.Hour), true)
	}
	w.deleteErr = audiostore.ErrUnavailable

	done := make(chan Report, 1)
	go func() {
		r, _ := w.janitor(JanitorOptions{BatchSize: 5}).RunOnce(context.Background())
		done <- r
	}()
	select {
	case report := <-done:
		assert.Equal(t, 5, report.PurgeFailed)
	case <-time.After(5 * time.Second):
		t.Fatal("a batch that cannot make progress kept being retried within one run")
	}
}

// unconfirmed adds a dictation that never became part of a confirmed answer.
func (w *world) unconfirmed(t *testing.T, madeAgo time.Duration) *row {
	t.Helper()
	r := w.add(t, nil, true)
	r.confirmed = false
	r.madeAt = epoch.Add(-madeAgo)
	return r
}

func TestUnconfirmed_DictationsThatNeverBecameAnAnswerAreDeletedAfterTheGracePeriod(t *testing.T) {
	w := newWorld()
	denied := w.unconfirmed(t, 3*time.Hour)   // "no, that's wrong", then dictated again
	justMade := w.unconfirmed(t, time.Minute) // may still be confirmed
	kept := w.add(t, nil, true)               // supports a confirmed answer, has no retention date

	report, err := w.janitor(JanitorOptions{UnconfirmedGrace: time.Hour}).RunOnce(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, report.PurgedUnconfirmed)
	assert.Zero(t, report.Purged, "these are not retention expiries and are counted apart")
	assert.False(t, w.has(denied.key))
	assert.True(t, denied.deleted)
	assert.True(t, w.has(justMade.key), "a student may still be about to confirm it")
	assert.True(t, w.has(kept.key), "audio behind a confirmed answer is never touched by this rule")

	events := w.eventsOf(audit.ActionAudioPurged)
	require.Len(t, events, 1)
	assert.Equal(t, "never_confirmed", events[0].Metadata["reason"])
}

func TestUnconfirmed_AreAuditedBeforeTheyAreDeletedAndKeptIfThatFails(t *testing.T) {
	w := newWorld()
	r := w.unconfirmed(t, 3*time.Hour)

	w.auditErr = errors.New("audit down")
	report, err := w.janitor(JanitorOptions{UnconfirmedGrace: time.Hour}).RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, report.PurgeFailed)
	assert.True(t, w.has(r.key))

	w.auditErr = nil
	w.steps = nil
	_, err = w.janitor(JanitorOptions{UnconfirmedGrace: time.Hour}).RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"audit:audio.purged", "delete-object", "mark-deleted"}, w.steps)
}

func TestUnconfirmed_ARecordingBothExpiredAndUnconfirmedIsPurgedOnce(t *testing.T) {
	w := newWorld()
	r := w.unconfirmed(t, 3*time.Hour)
	r.retention = at(-time.Hour)

	report, err := w.janitor(JanitorOptions{UnconfirmedGrace: time.Hour}).RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, report.Purged+report.PurgedUnconfirmed)
	assert.Len(t, w.eventsOf(audit.ActionAudioPurged), 1, "one deletion, one audit entry")
}

func TestUnconfirmed_DefaultGracePeriodIsOneHour(t *testing.T) {
	w := newWorld()
	old := w.unconfirmed(t, 61*time.Minute)
	recent := w.unconfirmed(t, 59*time.Minute)

	_, err := w.janitor(JanitorOptions{}).RunOnce(context.Background())
	require.NoError(t, err)
	assert.False(t, w.has(old.key))
	assert.True(t, w.has(recent.key))
}

func TestRun_OnlyOneInstanceCleansAtATime(t *testing.T) {
	w := newWorld()
	r := w.add(t, at(-time.Hour), true)
	w.locked = true

	report, err := w.janitor(JanitorOptions{}).RunOnce(context.Background())
	require.NoError(t, err)
	assert.True(t, report.Skipped)
	assert.True(t, w.has(r.key), "an instance that does not hold the lock touches nothing")
}

func TestRun_StopsWhenToldTo(t *testing.T) {
	w := newWorld()
	ctx, cancel := context.WithCancel(context.Background())
	j := w.janitor(JanitorOptions{Interval: time.Hour})
	done := make(chan struct{})
	go func() { j.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not stop after its context ended")
	}
}

// ---------------------------------------------------------------------------

func TestOrphans_AreFoundButOnlyRemovedWhenAskedAndOldEnough(t *testing.T) {
	w := newWorld()
	referenced := w.add(t, nil, true)
	oldOrphan := w.add(t, nil, false)
	youngOrphan := w.add(t, nil, false) // may be mid-save right now
	w.age(referenced.key, 48*time.Hour)
	w.age(oldOrphan.key, 48*time.Hour)
	w.age(youngOrphan.key, time.Minute)
	grace := JanitorOptions{OrphanGrace: time.Hour}

	t.Run("reporting only, by default", func(t *testing.T) {
		report, err := w.janitor(grace).RunOnce(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, report.Orphans)
		assert.Zero(t, report.OrphansRemoved)
		assert.True(t, w.has(oldOrphan.key), "nothing is deleted unless that was asked for")
	})

	t.Run("removed when asked, and recorded", func(t *testing.T) {
		grace.DeleteOrphans = true
		report, err := w.janitor(grace).RunOnce(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, report.OrphansRemoved)
		assert.False(t, w.has(oldOrphan.key))
		assert.True(t, w.has(youngOrphan.key), "audio that is possibly being saved is left alone")
		assert.True(t, w.has(referenced.key), "audio a recording refers to is never touched")

		events := w.eventsOf(audit.ActionOrphanRemoved)
		require.Len(t, events, 1)
		assert.Equal(t, oldOrphan.key, events[0].Metadata["storage_key"])
	})
}

func TestOrphans_AreNotRemovedWhenTheRemovalCannotBeRecorded(t *testing.T) {
	w := newWorld()
	orphan := w.add(t, nil, false)
	w.age(orphan.key, 48*time.Hour)
	w.auditErr = errors.New("audit down")

	report, err := w.janitor(JanitorOptions{DeleteOrphans: true}).RunOnce(context.Background())
	require.NoError(t, err)
	assert.Zero(t, report.OrphansRemoved)
	assert.True(t, w.has(orphan.key))
}

func TestOrphans_AudioLeftBehindForADeletedRecordingIsCleanedUp(t *testing.T) {
	w := newWorld()
	r := w.add(t, at(-time.Hour), true)
	r.deleted = true // marked deleted, but the object is still there (an earlier delete failed)
	w.age(r.key, 48*time.Hour)

	report, err := w.janitor(JanitorOptions{DeleteOrphans: true}).RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, report.OrphansRemoved)
	assert.False(t, w.has(r.key), "deleted recordings must not keep their audio")
}

func TestMissingAudio_IsReportedAndNothingIsChanged(t *testing.T) {
	w := newWorld()
	lost := w.add(t, at(time.Hour), true)
	require.NoError(t, w.store.Memory.Delete(context.Background(), lost.key)) // vanished from the store
	w.steps = nil

	report, err := w.janitor(JanitorOptions{DeleteOrphans: true}).RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, report.MissingAudio)
	assert.False(t, lost.deleted, "lost audio needs a person; the row is left as evidence")
	assert.Empty(t, w.steps)
}

func TestMissingAudio_AFreshRecordingIsNotMistakenForLostAudio(t *testing.T) {
	w := newWorld()
	for i := 0; i < 20; i++ {
		r := w.add(t, nil, true)
		w.age(r.key, time.Duration(i)*time.Second)
	}
	report, err := w.janitor(JanitorOptions{DeleteOrphans: true}).RunOnce(context.Background())
	require.NoError(t, err)
	assert.Zero(t, report.MissingAudio)
	assert.Zero(t, report.Orphans)
}

func TestJanitor_Defaults(t *testing.T) {
	j := NewJanitor(repoView{newWorld()}, audiostore.NewMemory(), &spyAudit{w: newWorld()}, JanitorOptions{}, nil)
	assert.Equal(t, time.Hour, j.opts.Interval)
	assert.Equal(t, 100, j.opts.BatchSize)
	assert.Equal(t, time.Hour, j.opts.OrphanGrace)
	assert.False(t, j.opts.DeleteOrphans, "removing orphans is opt-in")
}
