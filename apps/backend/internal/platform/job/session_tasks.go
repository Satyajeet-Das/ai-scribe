package job

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const (
	TaskExpireSession = "session:expire"
)

// ExpireSessionPayload is the data passed to the session expiry background job
type ExpireSessionPayload struct {
	SessionID uuid.UUID `json:"session_id"`
}

// EnqueueExpireSessionTask enqueues a background job to forcefully expire a session
// after the given duration.
func (j *JobService) EnqueueExpireSessionTask(sessionID uuid.UUID, delay time.Duration) error {
	payload := ExpireSessionPayload{
		SessionID: sessionID,
	}
	
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal expire session payload: %w", err)
	}

	task := asynq.NewTask(TaskExpireSession, payloadBytes)
	
	// Enqueue the task to be processed after the given delay
	info, err := j.Client.Enqueue(task, asynq.ProcessIn(delay), asynq.Queue("critical"))
	if err != nil {
		return fmt.Errorf("failed to enqueue session expiry task: %w", err)
	}
	
	j.logger.Info().
		Str("task_id", info.ID).
		Str("queue", info.Queue).
		Str("session_id", sessionID.String()).
		Dur("delay", delay).
		Msg("enqueued session expiry task")
		
	return nil
}
