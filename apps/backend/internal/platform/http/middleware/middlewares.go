package middleware

import (
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/logger"
)

type Middlewares struct {
	Global          *GlobalMiddlewares
	ContextEnhancer *ContextEnhancer
	Tracing         *TracingMiddleware
	RateLimit       *RateLimitMiddleware
}

func NewMiddlewares(cfg *config.Config, log *zerolog.Logger, loggerService *logger.LoggerService) *Middlewares {
	var nrApp *newrelic.Application
	if loggerService != nil {
		nrApp = loggerService.GetApplication()
	}

	return &Middlewares{
		Global:          NewGlobalMiddlewares(log, cfg.Server.CORSAllowedOrigins),
		ContextEnhancer: NewContextEnhancer(log),
		Tracing:         NewTracingMiddleware(nrApp),
		RateLimit:       NewRateLimitMiddleware(nrApp),
	}
}
