package redis

import (
	"context"
	"time"

	"github.com/newrelic/go-agent/v3/integrations/nrredis-v9"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	loggerPkg "github.com/Satyajeet-Das/ai-scribe/internal/platform/logger"
)

func New(cfg *config.Config, logger *zerolog.Logger, loggerService *loggerPkg.LoggerService) *redis.Client {
	opts, err := redis.ParseURL(cfg.Redis.Address)
	if err != nil {
		opts = &redis.Options{
			Addr: cfg.Redis.Address,
		}
	}

	redisClient := redis.NewClient(opts)

	if loggerService != nil && loggerService.GetApplication() != nil {
		redisClient.AddHook(nrredis.NewHook(redisClient.Options()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Warn().Err(err).Str("address", cfg.Redis.Address).Msg("unable to connect to Redis, continuing without Redis")
	} else {
		logger.Info().Str("address", cfg.Redis.Address).Msg("connected to Redis")
	}

	return redisClient
}
