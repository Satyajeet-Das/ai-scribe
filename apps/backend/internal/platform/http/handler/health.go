package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/middleware"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/logger"
)

type HealthHandler struct {
	env           string
	db            *database.Database
	redis         *redis.Client
	loggerService *logger.LoggerService
}

func NewHealthHandler(env string, db *database.Database, redisClient *redis.Client, loggerService *logger.LoggerService) *HealthHandler {
	return &HealthHandler{
		env:           env,
		db:            db,
		redis:         redisClient,
		loggerService: loggerService,
	}
}

func (h *HealthHandler) CheckHealth(c echo.Context) error {
	start := time.Now()
	log := middleware.GetLogger(c).With().
		Str("operation", "health_check").
		Logger()

	response := map[string]interface{}{
		"status":      "healthy",
		"timestamp":   time.Now().UTC(),
		"environment": h.env,
		"checks":      make(map[string]interface{}),
	}

	checks := response["checks"].(map[string]interface{})
	isHealthy := true

	// Check database connectivity
	if h.db != nil && h.db.Pool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		dbStart := time.Now()
		if err := h.db.Pool.Ping(ctx); err != nil {
			checks["database"] = map[string]interface{}{
				"status":        "unhealthy",
				"response_time": time.Since(dbStart).String(),
				"error":         err.Error(),
			}
			isHealthy = false
			log.Error().Err(err).Dur("response_time", time.Since(dbStart)).Msg("database health check failed")
			if h.loggerService != nil && h.loggerService.GetApplication() != nil {
				h.loggerService.GetApplication().RecordCustomEvent(
					"HealthCheckError", map[string]interface{}{
						"check_type":       "database",
						"operation":        "health_check",
						"error_type":       "database_unhealthy",
						"response_time_ms": time.Since(dbStart).Milliseconds(),
						"error_message":    err.Error(),
					})
			}
		} else {
			checks["database"] = map[string]interface{}{
				"status":        "healthy",
				"response_time": time.Since(dbStart).String(),
			}
			log.Info().Dur("response_time", time.Since(dbStart)).Msg("database health check passed")
		}
	} else {
		checks["database"] = map[string]interface{}{
			"status": "not_configured",
		}
	}

	// Check Redis connectivity
	if h.redis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		redisStart := time.Now()
		if err := h.redis.Ping(ctx).Err(); err != nil {
			checks["redis"] = map[string]interface{}{
				"status":        "unhealthy",
				"response_time": time.Since(redisStart).String(),
				"error":         err.Error(),
			}
			log.Error().Err(err).Dur("response_time", time.Since(redisStart)).Msg("redis health check failed")
			if h.loggerService != nil && h.loggerService.GetApplication() != nil {
				h.loggerService.GetApplication().RecordCustomEvent(
					"HealthCheckError", map[string]interface{}{
						"check_type":       "redis",
						"operation":        "health_check",
						"error_type":       "redis_unhealthy",
						"response_time_ms": time.Since(redisStart).Milliseconds(),
						"error_message":    err.Error(),
					})
			}
		} else {
			checks["redis"] = map[string]interface{}{
				"status":        "healthy",
				"response_time": time.Since(redisStart).String(),
			}
			log.Info().Dur("response_time", time.Since(redisStart)).Msg("redis health check passed")
		}
	}

	if !isHealthy {
		response["status"] = "unhealthy"
		log.Warn().
			Dur("total_duration", time.Since(start)).
			Msg("health check failed")
		if h.loggerService != nil && h.loggerService.GetApplication() != nil {
			h.loggerService.GetApplication().RecordCustomEvent(
				"HealthCheckError", map[string]interface{}{
					"check_type":        "overall",
					"operation":         "health_check",
					"error_type":        "overall_unhealthy",
					"total_duration_ms": time.Since(start).Milliseconds(),
				})
		}
		return c.JSON(http.StatusServiceUnavailable, response)
	}

	log.Info().
		Dur("total_duration", time.Since(start)).
		Msg("health check passed")

	if err := c.JSON(http.StatusOK, response); err != nil {
		log.Error().Err(err).Msg("failed to write JSON response")
		return fmt.Errorf("failed to write JSON response: %w", err)
	}

	return nil
}
