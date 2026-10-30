package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const (
	// TaskExtractDocument reads an uploaded exam paper into draft questions.
	TaskExtractDocument = "document:extract"
	// QueueDocuments is the queue only the document server reads.
	QueueDocuments = "documents"
)

// ExtractDocumentPayload names the document and the attempt the job is for. The
// attempt is how a late or duplicate job is recognised: it does nothing if the
// document has moved on to a newer one.
type ExtractDocumentPayload struct {
	DocumentID uuid.UUID `json:"document_id"`
	Attempt    int       `json:"attempt"`
}

// DocumentProcessor is the document module as the job layer sees it.
type DocumentProcessor interface {
	// Process runs one attempt. final is true on the job's last delivery.
	Process(ctx context.Context, documentID uuid.UUID, attempt int, final bool) error
}

// SetDocumentProcessor says what runs extraction jobs. It must be called before
// Start.
func (j *JobService) SetDocumentProcessor(p DocumentProcessor) { j.documents = p }

// EnqueueExtract queues the extraction of one attempt of a document. Queueing
// the same attempt twice is harmless: the task id is derived from the document
// and the attempt, so the second is recognised and ignored.
func (j *JobService) EnqueueExtract(documentID uuid.UUID, attempt int) error {
	payload, err := json.Marshal(ExtractDocumentPayload{DocumentID: documentID, Attempt: attempt})
	if err != nil {
		return fmt.Errorf("failed to marshal extract document payload: %w", err)
	}
	maxRetry, timeout := j.docJob.MaxRetry, j.docJob.Timeout
	if maxRetry <= 0 {
		maxRetry = 3
	}
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	task := asynq.NewTask(TaskExtractDocument, payload)
	info, err := j.Client.Enqueue(task,
		asynq.Queue(QueueDocuments),
		asynq.TaskID(fmt.Sprintf("document-extract-%s-%d", documentID, attempt)),
		asynq.MaxRetry(maxRetry),
		asynq.Timeout(timeout),
	)
	if err != nil {
		if errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return fmt.Errorf("failed to enqueue document extraction: %w", err)
	}
	j.logger.Info().
		Str("task_id", info.ID).
		Str("document_id", documentID.String()).
		Int("attempt", attempt).
		Msg("enqueued document extraction")
	return nil
}

// documentRetryDelay waits 30 s, 1 min, 2 min... between tries, capped at 10
// minutes: long enough for a rate limit to clear or a dead worker's lease to run
// out, short enough that a teacher is not left waiting for nothing.
func documentRetryDelay(n int, _ error, _ *asynq.Task) time.Duration {
	d := 30 * time.Second
	for i := 0; i < n && d < 10*time.Minute; i++ {
		d *= 2
	}
	if d > 10*time.Minute {
		d = 10 * time.Minute
	}
	return d
}

func (j *JobService) handleExtractDocumentTask(ctx context.Context, t *asynq.Task) error {
	var p ExtractDocumentPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		// A payload that cannot be read will never be readable.
		return fmt.Errorf("bad extract document payload: %v: %w", err, asynq.SkipRetry)
	}
	if j.documents == nil {
		return errors.New("document processor not initialised")
	}

	// On the last delivery a failure that would otherwise be retried must be
	// recorded as the document's failure instead.
	retried, _ := asynq.GetRetryCount(ctx)
	maxRetry, ok := asynq.GetMaxRetry(ctx)
	final := ok && retried >= maxRetry

	return j.documents.Process(ctx, p.DocumentID, p.Attempt, final)
}
