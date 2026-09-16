package middleware

import (
	"context"

	"github.com/labstack/echo/v4"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/logger"
)

const (
	UserIDKey   = "user_id"
	UserRoleKey = "user_role"
	LoggerKey   = "logger"
)

type ContextEnhancer struct {
	logger *zerolog.Logger
}

func NewContextEnhancer(logger *zerolog.Logger) *ContextEnhancer {
	return &ContextEnhancer{logger: logger}
}

func (ce *ContextEnhancer) EnhanceContext() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			requestID := GetRequestID(c)

			contextLogger := ce.logger.With().
				Str("request_id", requestID).
				Str("method", c.Request().Method).
				Str("path", c.Path()).
				Str("ip", c.RealIP()).
				Logger()

			if txn := newrelic.FromContext(c.Request().Context()); txn != nil {
				contextLogger = logger.WithTraceContext(contextLogger, txn)
			}

			if userID := ce.extractUserID(c); userID != "" {
				contextLogger = contextLogger.With().Str("user_id", userID).Logger()
			}

			if userRole := ce.extractUserRole(c); userRole != "" {
				contextLogger = contextLogger.With().Str("user_role", userRole).Logger()
			}

			c.Set(LoggerKey, &contextLogger)

			ctx := context.WithValue(c.Request().Context(), LoggerKey, &contextLogger)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}

func (ce *ContextEnhancer) extractUserID(c echo.Context) string {
	if userID, ok := c.Get(UserIDKey).(string); ok && userID != "" {
		return userID
	}
	return ""
}

func (ce *ContextEnhancer) extractUserRole(c echo.Context) string {
	if userRole, ok := c.Get(UserRoleKey).(string); ok && userRole != "" {
		return userRole
	}
	return ""
}

func GetUserID(c echo.Context) string {
	if userID, ok := c.Get(UserIDKey).(string); ok {
		return userID
	}
	return ""
}

func GetLogger(c echo.Context) *zerolog.Logger {
	if log, ok := c.Get(LoggerKey).(*zerolog.Logger); ok {
		return log
	}
	fallback := zerolog.Nop()
	return &fallback
}
