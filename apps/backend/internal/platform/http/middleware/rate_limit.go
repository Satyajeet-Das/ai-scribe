package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

type RateLimitMiddleware struct {
	nrApp  *newrelic.Application
	redis  *redis.Client
	logger *zerolog.Logger
}

func NewRateLimitMiddleware(nrApp *newrelic.Application) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		nrApp: nrApp,
	}
}

func (r *RateLimitMiddleware) WithRedis(client *redis.Client, logger *zerolog.Logger) *RateLimitMiddleware {
	r.redis = client
	r.logger = logger
	return r
}

func (r *RateLimitMiddleware) RecordRateLimitHit(endpoint string) {
	if r.nrApp != nil {
		r.nrApp.RecordCustomEvent("RateLimitHit", map[string]interface{}{
			"endpoint": endpoint,
		})
	}
}

// Limit returns an IP-based rate limiting echo middleware.
func (r *RateLimitMiddleware) Limit(maxRequests int64, window time.Duration) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if r.redis == nil {
				return next(c)
			}

			ip := c.RealIP()
			if ip == "" {
				ip = "unknown"
			}

			key := fmt.Sprintf("ratelimit:%s:%s", c.Path(), ip)
			ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
			defer cancel()

			count, err := r.redis.Incr(ctx, key).Result()
			if err != nil {
				if r.logger != nil {
					r.logger.Warn().Err(err).Msg("redis rate limit increment failed, allowing request")
				}
				return next(c)
			}

			if count == 1 {
				_ = r.redis.Expire(ctx, key, window).Err()
			}

			if count > maxRequests {
				r.RecordRateLimitHit(c.Path())
				ttl, _ := r.redis.TTL(ctx, key).Result()
				retryAfter := int(ttl.Seconds())
				if retryAfter <= 0 {
					retryAfter = int(window.Seconds())
				}
				c.Response().Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
					"code":     "TOO_MANY_REQUESTS",
					"message":  "Too many requests. Please try again later.",
					"status":   http.StatusTooManyRequests,
					"override": true,
				})
			}

			return next(c)
		}
	}
}
