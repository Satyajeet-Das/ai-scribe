package main

import (
	"context"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/logger"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		panic("failed to load configuration: " + err.Error())
	}

	loggerService := logger.NewLoggerService(cfg.Observability)
	defer loggerService.Shutdown()

	log := logger.NewLoggerWithService(cfg.Observability, loggerService)
	log.Info().Msg("starting database migration")

	if err := database.Migrate(context.Background(), &log, cfg); err != nil {
		log.Fatal().Err(err).Msg("failed to apply database migrations")
	}

	log.Info().Msg("database migration completed successfully")
}
