package job

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/email"
)

var emailClient *email.Client

func (j *JobService) InitHandlers(config *config.Config, logger *zerolog.Logger) {
	emailClient = email.NewClient(config, logger)
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
