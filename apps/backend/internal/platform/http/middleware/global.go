package middleware

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database/sqlerr"
	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
)

type GlobalMiddlewares struct {
	logger             *zerolog.Logger
	corsAllowedOrigins []string
}

func NewGlobalMiddlewares(logger *zerolog.Logger, corsAllowedOrigins []string) *GlobalMiddlewares {
	return &GlobalMiddlewares{
		logger:             logger,
		corsAllowedOrigins: corsAllowedOrigins,
	}
}

func (global *GlobalMiddlewares) CORS() echo.MiddlewareFunc {
	return middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: global.corsAllowedOrigins,
	})
}

func (global *GlobalMiddlewares) RequestLogger() echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:     true,
		LogStatus:  true,
		LogError:   true,
		LogLatency: true,
		LogHost:    true,
		LogMethod:  true,
		LogURIPath: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			statusCode := v.Status

			if v.Error != nil {
				var httpErr *httperrs.HTTPError
				var echoErr *echo.HTTPError
				if errors.As(v.Error, &httpErr) {
					statusCode = httpErr.Status
				} else if errors.As(v.Error, &echoErr) {
					statusCode = echoErr.Code
				}
			}

			logger := GetLogger(c)
			var e *zerolog.Event

			switch {
			case statusCode >= 500:
				e = logger.Error().Err(v.Error)
			case statusCode >= 400:
				e = logger.Warn()
			default:
				e = logger.Info()
			}

			if requestID := GetRequestID(c); requestID != "" {
				e = e.Str("request_id", requestID)
			}

			if userID := GetUserID(c); userID != "" {
				e = e.Str("user_id", userID)
			}

			e.
				Dur("latency", v.Latency).
				Int("status", statusCode).
				Str("method", v.Method).
				Str("uri", v.URI).
				Str("host", v.Host).
				Str("ip", c.RealIP()).
				Str("user_agent", c.Request().UserAgent()).
				Msg("HTTP_REQUEST")

			return nil
		},
	})
}

func (global *GlobalMiddlewares) Recover() echo.MiddlewareFunc {
	return middleware.Recover()
}

func (global *GlobalMiddlewares) Secure() echo.MiddlewareFunc {
	return middleware.Secure()
}

func (global *GlobalMiddlewares) GlobalErrorHandler(err error, c echo.Context) {
	originalErr := err

	var httpErr *httperrs.HTTPError
	if !errors.As(err, &httpErr) {
		var echoErr *echo.HTTPError
		if errors.As(err, &echoErr) {
			if echoErr.Code == http.StatusNotFound {
				err = httperrs.NewNotFoundError("Route not found", false, nil)
			}
		} else {
			err = sqlerr.HandleError(err)
		}
	}

	var echoErr *echo.HTTPError
	var status int
	var code string
	var message string
	var fieldErrors []httperrs.FieldError
	var action *httperrs.Action

	switch {
	case errors.As(err, &httpErr):
		status = httpErr.Status
		code = httpErr.Code
		message = httpErr.Message
		fieldErrors = httpErr.Errors
		action = httpErr.Action

	case errors.As(err, &echoErr):
		status = echoErr.Code
		code = httperrs.MakeUpperCaseWithUnderscores(http.StatusText(status))
		if msg, ok := echoErr.Message.(string); ok {
			message = msg
		} else {
			message = http.StatusText(echoErr.Code)
		}

	default:
		status = http.StatusInternalServerError
		code = httperrs.MakeUpperCaseWithUnderscores(http.StatusText(http.StatusInternalServerError))
		message = http.StatusText(http.StatusInternalServerError)
	}

	logger := *GetLogger(c)
	logger.Error().Stack().
		Err(originalErr).
		Int("status", status).
		Str("error_code", code).
		Msg(message)

	if !c.Response().Committed {
		_ = c.JSON(status, httperrs.HTTPError{
			Code:     code,
			Message:  message,
			Status:   status,
			Override: httpErr != nil && httpErr.Override,
			Errors:   fieldErrors,
			Action:   action,
		})
	}
}
