package auth

import (
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"

	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/middleware"
)

type Middleware struct {
	provider AuthProvider
	logger   *zerolog.Logger
}

func NewMiddleware(provider AuthProvider, logger *zerolog.Logger) *Middleware {
	return &Middleware{
		provider: provider,
		logger:   logger,
	}
}

// RequireAuth authenticates the incoming HTTP request by extracting the Bearer token
// and delegating verification to the injected AuthProvider.
func (m *Middleware) RequireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()
		reqID := middleware.GetRequestID(c)

		authHeader := c.Request().Header.Get("Authorization")
		if authHeader == "" {
			m.logger.Warn().
				Str("request_id", reqID).
				Str("path", c.Path()).
				Dur("duration", time.Since(start)).
				Msg("missing Authorization header")
			return httperrs.NewUnauthorizedError("Unauthorized: missing authorization header", false)
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			m.logger.Warn().
				Str("request_id", reqID).
				Str("path", c.Path()).
				Dur("duration", time.Since(start)).
				Msg("invalid Authorization header format")
			return httperrs.NewUnauthorizedError("Unauthorized: invalid bearer token format", false)
		}

		rawToken := strings.TrimSpace(parts[1])
		identity, err := m.provider.Authenticate(c.Request().Context(), rawToken)
		if err != nil {
			m.logger.Warn().
				Err(err).
				Str("request_id", reqID).
				Str("path", c.Path()).
				Dur("duration", time.Since(start)).
				Msg("token authentication failed")
			return httperrs.NewUnauthorizedError("Unauthorized", false)
		}

		// Populate identity in context
		SetIdentity(c, identity)

		m.logger.Info().
			Str("request_id", reqID).
			Str("user_id", identity.UserID.String()).
			Str("role", string(identity.Role)).
			Str("provider", identity.Provider).
			Dur("duration", time.Since(start)).
			Msg("user authenticated successfully")

		return next(c)
	}
}

// Authenticate is an alias for RequireAuth.
func (m *Middleware) Authenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return m.RequireAuth(next)
}

// RequireRole ensures the authenticated user has at least one of the specified roles.
// ADMIN role is always permitted.
func (m *Middleware) RequireRole(allowedRoles ...Role) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			role := GetCallerRole(c)
			if role == "" {
				userID, _ := GetCallerUserID(c)
				m.logger.Warn().
					Str("user_id", userID.String()).
					Str("path", c.Path()).
					Msg("role check failed: unauthenticated or missing role")
				return httperrs.NewForbiddenError("Forbidden: insufficient role permissions", false)
			}

			// Admin always has access
			if strings.EqualFold(string(role), string(RoleAdmin)) {
				return next(c)
			}

			for _, allowed := range allowedRoles {
				if strings.EqualFold(string(role), string(allowed)) {
					return next(c)
				}
			}

			userID, _ := GetCallerUserID(c)
			m.logger.Warn().
				Str("user_id", userID.String()).
				Str("current_role", string(role)).
				Str("path", c.Path()).
				Msg("user forbidden: insufficient role permissions")

			return httperrs.NewForbiddenError("Forbidden: insufficient role permissions", false)
		}
	}
}
