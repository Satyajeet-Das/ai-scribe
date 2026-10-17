package recording

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/audit"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
)

// JanitorOptions configure housekeeping.
type JanitorOptions struct {
	// Interval is the time between runs.
	Interval time.Duration
	// BatchSize bounds how many recordings are purged per database round trip.
	BatchSize int
	// UnconfirmedGrace is how long a dictation the student never confirmed is
	// kept before its audio is deleted. It must be longer than the time a student
	// has to confirm. Such audio supports no answer, so keeping it serves no one.
	UnconfirmedGrace time.Duration
	// OrphanGrace is how old an object must be before it can count as an orphan.
	// It must comfortably exceed the time between storing audio and writing its
	// database row, or audio being saved right now would be taken for an orphan.
	OrphanGrace time.Duration
	// DeleteOrphans removes orphaned audio. When false they are only reported.
	DeleteOrphans bool
}

// Report is what one run did.
type Report struct {
	// Skipped is true when another instance was already cleaning.
	Skipped bool
	// Purged is the number of recordings whose retention ended and whose audio
	// was deleted. PurgeFailed were left for the next run.
	Purged      int
	PurgeFailed int
	// PurgedUnconfirmed counts dictations that never became an answer and whose
	// audio was deleted. They are not included in Purged.
	PurgedUnconfirmed int
	// Orphans are stored objects no recording refers to; OrphansRemoved were deleted.
	Orphans        int
	OrphansRemoved int
	// MissingAudio are recordings that should have audio but whose object is not
	// in the store. They are reported and never changed: this is lost data and
	// needs a person.
	MissingAudio int
}

// Janitor deletes audio when its retention ends and tidies up objects that no
// recording refers to. It only ever acts on dates and rows that already exist;
// it decides no policy of its own.
type Janitor struct {
	repo   JanitorRepository
	store  audiostore.Store
	audit  audit.Recorder
	opts   JanitorOptions
	logger *zerolog.Logger
	now    func() time.Time
}

func NewJanitor(repo JanitorRepository, store audiostore.Store, rec audit.Recorder, opts JanitorOptions, logger *zerolog.Logger) *Janitor {
	if opts.Interval <= 0 {
		opts.Interval = time.Hour
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 100
	}
	if opts.OrphanGrace <= 0 {
		opts.OrphanGrace = time.Hour
	}
	if opts.UnconfirmedGrace <= 0 {
		opts.UnconfirmedGrace = time.Hour
	}
	nop := zerolog.Nop()
	if logger == nil {
		logger = &nop
	}
	return &Janitor{repo: repo, store: store, audit: rec, opts: opts, logger: logger, now: time.Now}
}

// maxBatchesPerRun stops a very large backlog from holding the lock for ever;
// the rest is done on the next run.
const maxBatchesPerRun = 50

// Run does housekeeping now and then every Interval until ctx ends.
func (j *Janitor) Run(ctx context.Context) {
	j.runAndLog(ctx)
	ticker := time.NewTicker(j.opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.runAndLog(ctx)
		}
	}
}

func (j *Janitor) runAndLog(ctx context.Context) {
	report, err := j.RunOnce(ctx)
	switch {
	case err != nil && ctx.Err() == nil:
		j.logger.Error().Err(err).Str("event", "recording.cleanup_failed").Msg("recording cleanup failed")
	case report.Skipped:
		j.logger.Debug().Str("event", "recording.cleanup_skipped").Msg("another instance is cleaning recordings")
	default:
		ev := j.logger.Info()
		if report.MissingAudio > 0 || report.PurgeFailed > 0 {
			ev = j.logger.Warn()
		}
		ev.Str("event", "recording.cleanup").
			Int("purged", report.Purged).Int("purged_unconfirmed", report.PurgedUnconfirmed).
			Int("purge_failed", report.PurgeFailed).
			Int("orphans", report.Orphans).Int("orphans_removed", report.OrphansRemoved).
			Int("missing_audio", report.MissingAudio).Msg("recording cleanup finished")
	}
}

// RunOnce does one round of housekeeping. It is safe to run on several
// instances at once (only one proceeds) and safe to repeat or interrupt: every
// step can be retried and leaves nothing half-done that the next run cannot
// finish.
func (j *Janitor) RunOnce(ctx context.Context) (Report, error) {
	var report Report
	release, ok, err := j.repo.TryLock(ctx)
	if err != nil {
		return report, err
	}
	if !ok {
		report.Skipped = true
		return report, nil
	}
	defer release()

	if err := j.purgeExpired(ctx, &report); err != nil {
		return report, err
	}
	if err := j.purgeUnconfirmed(ctx, &report); err != nil {
		return report, err
	}
	if err := j.tidyStore(ctx, &report); err != nil {
		return report, err
	}
	return report, nil
}

// purgeExpired deletes the audio of recordings whose retention ended. For each:
// the access is audited first (if that cannot be done the audio stays), then
// the object is deleted, then the row is marked. A crash between the last two
// leaves a row that still looks stored, and the next run finishes the job,
// because deleting a missing object is not an error.
func (j *Janitor) purgeExpired(ctx context.Context, report *Report) error {
	return j.purgeBatches(ctx, "retention_expired", &report.Purged, &report.PurgeFailed,
		func(limit int) ([]Expired, error) { return j.repo.Expired(ctx, j.now(), limit) })
}

// purgeUnconfirmed deletes audio of dictations that never became an answer: the
// student denied the transcript, dictated again, or never confirmed. It does
// not wait for a retention date, because it supports no answer and the notice
// the student saw is about keeping answers for review, not every attempt.
func (j *Janitor) purgeUnconfirmed(ctx context.Context, report *Report) error {
	return j.purgeBatches(ctx, "never_confirmed", &report.PurgedUnconfirmed, &report.PurgeFailed,
		func(limit int) ([]Expired, error) {
			return j.repo.Unconfirmed(ctx, j.now().Add(-j.opts.UnconfirmedGrace), limit)
		})
}

func (j *Janitor) purgeBatches(ctx context.Context, reason string, purged, failed *int, next func(limit int) ([]Expired, error)) error {
	for batch := 0; batch < maxBatchesPerRun; batch++ {
		due, err := next(j.opts.BatchSize)
		if err != nil {
			return err
		}
		if len(due) == 0 {
			return nil
		}
		progressed := false
		for _, e := range due {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := j.purgeOne(ctx, e, reason); err != nil {
				*failed++
				j.logger.Error().Err(err).Str("event", "recording.purge_failed").
					Str("recording_id", e.ID.String()).Str("reason", reason).Msg("recording not purged; will retry")
				continue
			}
			*purged++
			progressed = true
		}
		// If nothing in a whole batch could be purged, the same rows would come
		// back for ever. Stop and let the next run try again.
		if !progressed || len(due) < j.opts.BatchSize {
			return nil
		}
	}
	return nil
}

func (j *Janitor) purgeOne(ctx context.Context, e Expired, reason string) error {
	if err := j.audit.Record(ctx, audit.Event{
		ActorRole: "SYSTEM", Action: audit.ActionAudioPurged, ResourceType: audit.ResourceRecording,
		ResourceID: e.ID, SessionID: e.SessionID, Outcome: audit.OutcomeOK,
		Metadata: map[string]any{"reason": reason},
	}); err != nil {
		return err
	}
	if err := j.store.Delete(ctx, e.StorageKey); err != nil {
		return err
	}
	_, err := j.repo.MarkDeleted(ctx, e.ID, j.now().UTC())
	return err
}

// tidyStore compares the store with the database. Objects no recording refers
// to are orphans (a crash between storing audio and writing its row, or a failed
// delete). Recordings whose object is gone are reported, never altered.
func (j *Janitor) tidyStore(ctx context.Context, report *Report) error {
	// The database is read first, then the store. Audio is stored before its row
	// is written, so every row seen here already has its object by the time the
	// store is listed (no false "missing audio"), and audio saved in between is
	// young enough to be left alone (no false "orphan").
	known, err := j.repo.StoredKeys(ctx)
	if err != nil {
		return err
	}
	objects, err := j.store.List(ctx)
	if err != nil {
		return err
	}

	present := make(map[string]struct{}, len(objects))
	cutoff := j.now().Add(-j.opts.OrphanGrace)
	for _, o := range objects {
		present[o.Key] = struct{}{}
		if _, referenced := known[o.Key]; referenced {
			continue
		}
		if o.ModifiedAt.After(cutoff) {
			continue // possibly being saved right now
		}
		report.Orphans++
		if !j.opts.DeleteOrphans {
			j.logger.Warn().Str("event", "recording.orphan").Str("storage_key", o.Key).
				Msg("audio no recording refers to (not deleted: orphan removal is off)")
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := j.removeOrphan(ctx, o.Key); err != nil {
			j.logger.Error().Err(err).Str("event", "recording.orphan_remove_failed").
				Str("storage_key", o.Key).Msg("orphaned audio not removed; will retry")
			continue
		}
		report.OrphansRemoved++
	}

	for key, id := range known {
		if _, ok := present[key]; ok {
			continue
		}
		report.MissingAudio++
		j.logger.Error().Str("event", "recording.audio_missing").
			Str("recording_id", id.String()).Msg("a recording's audio is not in the store")
	}
	return nil
}

func (j *Janitor) removeOrphan(ctx context.Context, key string) error {
	if err := j.audit.Record(ctx, audit.Event{
		ActorRole: "SYSTEM", Action: audit.ActionOrphanRemoved, ResourceType: audit.ResourceObject,
		Outcome: audit.OutcomeOK, Metadata: map[string]any{"storage_key": key},
	}); err != nil {
		return err
	}
	if err := j.store.Delete(ctx, key); err != nil && !errors.Is(err, audiostore.ErrNotFound) {
		return err
	}
	return nil
}
