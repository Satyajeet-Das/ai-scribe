package http

import (
	"net/http"

	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/handler"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/middleware"
)

type RouterParams struct {
	Middlewares    *middleware.Middlewares
	HealthHandler  *handler.HealthHandler
	OpenAPIHandler *handler.OpenAPIHandler
	Logger         *zerolog.Logger
}

func NewRouter(params RouterParams) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	e.HTTPErrorHandler = params.Middlewares.Global.GlobalErrorHandler

	e.Use(
		echoMiddleware.RateLimiterWithConfig(echoMiddleware.RateLimiterConfig{
			Store: echoMiddleware.NewRateLimiterMemoryStore(rate.Limit(20)),
			DenyHandler: func(c echo.Context, identifier string, err error) error {
				if params.Middlewares.RateLimit != nil {
					params.Middlewares.RateLimit.RecordRateLimitHit(c.Path())
				}

				params.Logger.Warn().
					Str("request_id", middleware.GetRequestID(c)).
					Str("identifier", identifier).
					Str("path", c.Path()).
					Str("method", c.Request().Method).
					Str("ip", c.RealIP()).
					Msg("rate limit exceeded")

				return echo.NewHTTPError(http.StatusTooManyRequests, "Rate limit exceeded")
			},
		}),
		params.Middlewares.Global.CORS(),
		params.Middlewares.Global.Secure(),
		middleware.RequestID(),
		params.Middlewares.Tracing.NewRelicMiddleware(),
		params.Middlewares.Tracing.EnhanceTracing(),
		params.Middlewares.ContextEnhancer.EnhanceContext(),
		params.Middlewares.Global.RequestLogger(),
		params.Middlewares.Global.Recover(),
	)

	// Register system routes
	registerSystemRoutes(e, params.HealthHandler, params.OpenAPIHandler)

	return e
}

func registerSystemRoutes(e *echo.Echo, health *handler.HealthHandler, openapi *handler.OpenAPIHandler) {
	if health != nil {
		e.GET("/status", health.CheckHealth)
	}

	e.Static("/static", "static")

	if openapi != nil {
		e.GET("/docs", openapi.ServeOpenAPIUI)
	}
}
