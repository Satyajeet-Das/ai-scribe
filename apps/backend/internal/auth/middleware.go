package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"

	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/middleware"
)

type Middleware struct {
	logger *zerolog.Logger
}

func NewMiddleware(logger *zerolog.Logger) *Middleware {
	return &Middleware{
		logger: logger,
	}
}

func (m *Middleware) RequireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return echo.WrapMiddleware(
		clerkhttp.WithHeaderAuthorization(
			clerkhttp.AuthorizationFailureHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				start := time.Now()

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)

				response := map[string]string{
					"code":     "UNAUTHORIZED",
					"message":  "Unauthorized",
					"override": "false",
					"status":   "401",
				}

				if err := json.NewEncoder(w).Encode(response); err != nil {
					m.logger.Error().Err(err).Str("function", "RequireAuth").Dur("duration", time.Since(start)).Msg("failed to write JSON response")
				} else {
					m.logger.Error().Str("function", "RequireAuth").Dur("duration", time.Since(start)).Msg("could not get session claims from context")
				}
			}))))(func(c echo.Context) error {
		start := time.Now()
		claims, ok := clerk.SessionClaimsFromContext(c.Request().Context())

		if !ok {
			m.logger.Error().
				Str("function", "RequireAuth").
				Str("request_id", middleware.GetRequestID(c)).
				Dur("duration", time.Since(start)).
				Msg("could not get session claims from context")
			return httperrs.NewUnauthorizedError("Unauthorized", false)
		}

		c.Set("user_id", claims.Subject)
		c.Set("user_role", claims.ActiveOrganizationRole)
		c.Set("permissions", claims.Claims.ActiveOrganizationPermissions)

		m.logger.Info().
			Str("function", "RequireAuth").
			Str("user_id", claims.Subject).
			Str("request_id", middleware.GetRequestID(c)).
			Dur("duration", time.Since(start)).
			Msg("user authenticated successfully")

		return next(c)
	})
}

func (m *Middleware) RequireRole(allowedRoles ...Role) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			roleVal := c.Get("user_role")
			roleStr, _ := roleVal.(string)

			for _, allowed := range allowedRoles {
				if roleStr != "" && (roleStr == string(allowed) || roleStr == "admin" || roleStr == "ADMIN") {
					return next(c)
				}
			}

			userID, _ := c.Get("user_id").(string)
			m.logger.Warn().
				Str("function", "RequireRole").
				Str("user_id", userID).
				Str("current_role", roleStr).
				Msg("user forbidden: insufficient role permissions")

			return httperrs.NewForbiddenError("Forbidden: insufficient role permissions", false)
		}
	}
}

