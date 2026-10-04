package job

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/email"
)

type SessionExpirer interface {
	ExpireSession(ctx context.Context, sessionID uuid.UUID) error
}

var (
	emailClient    *email.Client
	sessionExpirer SessionExpirer
)

func (j *JobService) InitHandlers(config *config.Config, logger *zerolog.Logger, expirer SessionExpirer) {
	emailClient = email.NewClient(config, logger)
	sessionExpirer = expirer
}

func (j *JobService) handleWelcomeEmailTask(ctx context.Context, t *asynq.Task) error {
	var p WelcomeEmailPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("failed to unmarshal welcome email payload: %w", err)
	}

	j.logger.Info().
		Str("type", "welcome").
		Str("to", p.To).
		Msg("processing welcome email task")

	if emailClient == nil {
		j.logger.Warn().Msg("email client not initialized, skipping email delivery")
		return nil
	}

	err := emailClient.SendWelcomeEmail(p.To, p.FirstName)
	if err != nil {
		j.logger.Error().
			Str("type", "welcome").
			Str("to", p.To).
			Err(err).
			Msg("failed to send welcome email")
		return err
	}

	j.logger.Info().
		Str("type", "welcome").
		Str("to", p.To).
		Msg("successfully sent welcome email")
	return nil
}

func (j *JobService) handleExpireSessionTask(ctx context.Context, t *asynq.Task) error {
	var p ExpireSessionPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("failed to unmarshal expire session payload: %w", err)
	}

	j.logger.Info().
		Str("type", "expire_session").
		Str("session_id", p.SessionID.String()).
		Msg("processing session expiry task")

	if sessionExpirer == nil {
		j.logger.Warn().Msg("session expirer not initialized, skipping session expiry")
		return nil
	}

	err := sessionExpirer.ExpireSession(ctx, p.SessionID)
	if err != nil {
		j.logger.Error().
			Str("type", "expire_session").
			Str("session_id", p.SessionID.String()).
			Err(err).
			Msg("failed to execute session expiry")
		return err
	}

	j.logger.Info().
		Str("type", "expire_session").
		Str("session_id", p.SessionID.String()).
		Msg("successfully executed session expiry")
	return nil
}
