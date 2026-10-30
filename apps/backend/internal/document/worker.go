package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/pdf"
)

// errLostOwnership is the cause set on a job's context when its document was
// cancelled, deleted or restarted, or its lease was taken over. The job stops
// quietly: whoever did that owns the document now.
var errLostOwnership = errors.New("document no longer belongs to this job")

// heartbeatEvery is how often a running job renews its lease and checks that
// the document is still its own.
func (s *service) heartbeatEvery() time.Duration {
	d := s.opts.Lease / 3
	if d > 15*time.Second {
		d = 15 * time.Second
	}
	if d < time.Second {
		d = time.Second
	}
	return d
}

// Process runs the extraction job for one attempt of one document.
//
// It is safe to run twice, concurrently or after a crash. Taking the document
// is a conditional update that only one worker can win, and every write after
// that is conditional on the document still being this attempt's and still
// PROCESSING, so a worker that was cancelled, replaced or restarted cannot
// overwrite a newer result. No database transaction is open while the PDF is
// read or the model is called.
func (s *service) Process(ctx context.Context, documentID uuid.UUID, attempt int, final bool) error {
	res, d, err := s.repo.Claim(ctx, documentID, attempt, s.opts.Lease)
	if err != nil {
		return err // the database is unwell: let the queue try again
	}
	log := s.log.With().Str("document_id", documentID.String()).Int("attempt", attempt).Logger()
	switch res {
	case ClaimStale:
		log.Info().Msg("extraction job has nothing to do: the document is gone, finished, cancelled or restarted")
		return nil
	case ClaimLeaseHeld:
		// Another worker is on it. If that worker is dead its lease will run out
		// and a later delivery of this job takes over.
		return ErrLeaseHeld
	}

	workCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	stop := s.heartbeat(workCtx, cancel, documentID, attempt)
	defer stop()

	started := time.Now()
	err = s.run(workCtx, cancel, d)
	if err == nil {
		log.Info().Dur("took", time.Since(started)).Msg("extraction finished; draft is ready for review")
		return nil
	}
	if errors.Is(err, errLostOwnership) || errors.Is(context.Cause(workCtx), errLostOwnership) {
		log.Info().Msg("extraction stopped: the document is no longer this job's")
		return nil
	}
	return s.finish(ctx, d, err, final, log)
}

// heartbeat renews the lease until stopped, and cancels the job's context if it
// finds the document is no longer this attempt's to work on.
func (s *service) heartbeat(ctx context.Context, cancel context.CancelCauseFunc, id uuid.UUID, attempt int) (stop func()) {
	done := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		t := time.NewTicker(s.heartbeatEvery())
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				ok, err := s.repo.Heartbeat(ctx, id, attempt, s.opts.Lease)
				if err != nil {
					// A missed beat is not a reason to stop: the lease has room for more.
					s.log.Warn().Err(err).Str("document_id", id.String()).Msg("could not renew a document lease")
					continue
				}
				if !ok {
					cancel(errLostOwnership)
					return
				}
			}
		}
	}()
	return func() { close(done); <-exited }
}

// progress records where the job is. It reports false, and ends the job, when
// the document is no longer the job's.
func (s *service) progress(ctx context.Context, cancel context.CancelCauseFunc, id uuid.UUID, attempt int, stage Stage, done, total int) {
	ok, err := s.repo.SetProgress(ctx, id, attempt, stage, done, total)
	if err == nil && !ok {
		cancel(errLostOwnership)
	}
}

// transient marks a failure of our own infrastructure as worth retrying.
func transient(code string) error { return extract.NewError(code, true, "") }

func (s *service) run(ctx context.Context, cancel context.CancelCauseFunc, d *Document) error {

	data, err := s.loadFile(ctx, d)
	if err != nil {
		return err
	}

	s.progress(ctx, cancel, d.ID, d.Attempt, StageReadingPages, 0, d.PageCount)
	input, infos, err := s.readPaper(ctx, data)
	if err != nil {
		return mapReadError(err)
	}
	data = nil // the PDF is no longer needed; let it go before the model call

	extracted, err := s.extractor.Extract(ctx, input, func(done, total int) {
		s.progress(ctx, cancel, d.ID, d.Attempt, StageExtracting, done, total)
	})
	if err != nil {
		return err
	}

	s.progress(ctx, cancel, d.ID, d.Attempt, StageValidating, 0, 0)
	if len(extracted.Questions) > maxDraftQuestions {
		return extract.NewError(extract.CodeTooManyBatches, false, "more than %d questions", maxDraftQuestions)
	}

	s.progress(ctx, cancel, d.ID, d.Attempt, StageSaving, 0, 0)
	if err := ctx.Err(); err != nil {
		return err
	}
	ok, err := s.repo.SaveDraft(ctx, d.ID, d.Attempt, toDraft(d, s.extractor, infos, extracted))
	if err != nil {
		s.log.Error().Err(err).Str("document_id", d.ID.String()).Msg("saving the extracted draft failed")
		return transient(extract.CodeInternal)
	}
	if !ok {
		return errLostOwnership
	}
	return nil
}

// loadFile reads the stored PDF, bounded by the size limit, and checks it is the
// file that was uploaded.
func (s *service) loadFile(ctx context.Context, d *Document) ([]byte, error) {
	rc, _, err := s.store.Open(ctx, d.StorageKey)
	if err != nil {
		if errors.Is(err, audiostore.ErrNotFound) {
			return nil, extract.NewError(codeFileMissing, false, "")
		}
		return nil, transient(extract.CodeStorageDown)
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, s.opts.MaxBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, transient(extract.CodeStorageDown)
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != d.SizeBytes || hex.EncodeToString(sum[:]) != d.SHA256 {
		return nil, extract.NewError(extract.CodeInvalidPDF, false, "stored file does not match the upload")
	}
	return data, nil
}

// mapReadError classifies a failure to read the PDF.
func mapReadError(err error) error {
	switch {
	case errors.Is(err, pdf.ErrBusy), errors.Is(err, context.DeadlineExceeded):
		return extract.NewError(extract.CodeTimeout, true, "")
	case errors.Is(err, context.Canceled):
		return err
	}
	return extract.NewError(extract.CodeInvalidPDF, false, "")
}

func toDraft(d *Document, ex Extractor, infos []PageInfo, r *extract.Result) DraftData {
	now := time.Now().UTC()
	qs := make([]DraftQuestion, len(r.Questions))
	for i, q := range r.Questions {
		qs[i] = DraftQuestion{
			ID: uuid.New(), DocumentID: d.ID, Position: i + 1,
			NumberLabel: q.NumberLabel, Type: q.Type, Instructions: q.Instructions, Text: q.Text, Points: q.Points,
			Options: q.Options, Tables: q.Tables, Diagrams: q.Diagrams, SourcePages: q.SourcePages,
			Confidence: q.Confidence, Warnings: q.Warnings, Origin: OriginExtracted, CreatedAt: now, UpdatedAt: now,
		}
	}
	return DraftData{
		Provider: ex.Name(), PromptVersion: ex.PromptVersion(), PageSummary: infos,
		DocumentInstructions: r.DocumentInstructions, Unassigned: r.Unassigned, Questions: qs,
	}
}

// finish records how a job ended badly. A failure that may pass is put back in
// the queue (and the document shows QUEUED again) until the job's last delivery;
// after that, and for any failure that cannot pass, the document is FAILED with
// a code and a message that this application wrote.
func (s *service) finish(ctx context.Context, d *Document, err error, final bool, log zerolog.Logger) error {
	code, retry := classify(err)
	// The request that started the job may be over (shutdown, timeout); recording
	// the outcome must not be.
	bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	if retry && !final {
		if _, rerr := s.repo.Requeue(bg, d.ID, d.Attempt); rerr != nil {
			log.Error().Err(rerr).Msg("could not put the document back in the queue")
		}
		log.Warn().Str("code", code).Msg("extraction failed for now; it will be tried again")
		return err
	}
	if _, ferr := s.repo.Fail(bg, d.ID, d.Attempt, code, failureMessage(code)); ferr != nil {
		log.Error().Err(ferr).Msg("could not record that extraction failed")
		// Without a record the document would sit in PROCESSING until its lease
		// ran out; let the queue deliver the job again.
		return ferr
	}
	log.Warn().Str("code", code).Bool("final_attempt", final).Msg("extraction failed")
	return nil
}

// classify maps an error to a stable failure code and whether trying again
// could help.
func classify(err error) (code string, retry bool) {
	var e *extract.Error
	if errors.As(err, &e) {
		return e.Code, e.Transient
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return extract.CodeTimeout, true
	case errors.Is(err, context.Canceled):
		return codeInterrupted, true
	case errors.Is(err, ErrLeaseHeld):
		return codeInterrupted, true
	}
	return extract.CodeInternal, false
}
