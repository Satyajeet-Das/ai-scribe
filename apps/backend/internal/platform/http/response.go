package http

import (
	"time"

	"github.com/labstack/echo/v4"
	"github.com/newrelic/go-agent/v3/integrations/nrpkgerrors"
	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/middleware"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/validation"
)

type HandlerFunc[Req validation.Validatable, Res any] func(c echo.Context, req Req) (Res, error)
type HandlerFuncNoContent[Req validation.Validatable] func(c echo.Context, req Req) error

type ResponseHandler interface {
	Handle(c echo.Context, result interface{}) error
	GetOperation() string
	AddAttributes(txn *newrelic.Transaction, result interface{})
}

type JSONResponseHandler struct {
	status int
}

func (h JSONResponseHandler) Handle(c echo.Context, result interface{}) error {
	return c.JSON(h.status, result)
}

func (h JSONResponseHandler) GetOperation() string {
	return "handler"
}

func (h JSONResponseHandler) AddAttributes(txn *newrelic.Transaction, result interface{}) {}

type NoContentResponseHandler struct {
	status int
}

func (h NoContentResponseHandler) Handle(c echo.Context, result interface{}) error {
	return c.NoContent(h.status)
}

func (h NoContentResponseHandler) GetOperation() string {
	return "handler_no_content"
}

func (h NoContentResponseHandler) AddAttributes(txn *newrelic.Transaction, result interface{}) {}

type FileResponseHandler struct {
	status      int
	filename    string
	contentType string
}

func (h FileResponseHandler) Handle(c echo.Context, result interface{}) error {
	data := result.([]byte)
	c.Response().Header().Set("Content-Disposition", "attachment; filename="+h.filename)
	return c.Blob(h.status, h.contentType, data)
}

func (h FileResponseHandler) GetOperation() string {
	return "handler_file"
}

func (h FileResponseHandler) AddAttributes(txn *newrelic.Transaction, result interface{}) {
	if txn != nil {
		txn.AddAttribute("file.name", h.filename)
		txn.AddAttribute("file.content_type", h.contentType)
		if data, ok := result.([]byte); ok {
			txn.AddAttribute("file.size_bytes", len(data))
		}
	}
}

func handleRequest[Req validation.Validatable](
	c echo.Context,
	req Req,
	handler func(c echo.Context, req Req) (interface{}, error),
	responseHandler ResponseHandler,
) error {
	start := time.Now()
	method := c.Request().Method
	path := c.Path()
	route := path

	txn := newrelic.FromContext(c.Request().Context())
	if txn != nil {
		txn.AddAttribute("handler.name", route)
		responseHandler.AddAttributes(txn, nil)
	}

	loggerBuilder := middleware.GetLogger(c).With().
		Str("operation", responseHandler.GetOperation()).
		Str("method", method).
		Str("path", path).
		Str("route", route)

	if fileHandler, ok := responseHandler.(FileResponseHandler); ok {
		loggerBuilder = loggerBuilder.
			Str("filename", fileHandler.filename).
			Str("content_type", fileHandler.contentType)
	}

	logger := loggerBuilder.Logger()
	logger.Info().Msg("handling request")

	validationStart := time.Now()
	if err := validation.BindAndValidate(c, req); err != nil {
		validationDuration := time.Since(validationStart)
		logger.Error().
			Err(err).
			Dur("validation_duration", validationDuration).
			Msg("request validation failed")

		if txn != nil {
			txn.NoticeError(nrpkgerrors.Wrap(err))
			txn.AddAttribute("validation.status", "failed")
			txn.AddAttribute("validation.duration_ms", validationDuration.Milliseconds())
		}
		return err
	}

	validationDuration := time.Since(validationStart)
	if txn != nil {
		txn.AddAttribute("validation.status", "success")
		txn.AddAttribute("validation.duration_ms", validationDuration.Milliseconds())
	}

	handlerStart := time.Now()
	result, err := handler(c, req)
	handlerDuration := time.Since(handlerStart)

	if err != nil {
		totalDuration := time.Since(start)
		logger.Error().
			Err(err).
			Dur("handler_duration", handlerDuration).
			Dur("total_duration", totalDuration).
			Msg("handler execution failed")

		if txn != nil {
			txn.NoticeError(nrpkgerrors.Wrap(err))
			txn.AddAttribute("handler.status", "error")
			txn.AddAttribute("handler.duration_ms", handlerDuration.Milliseconds())
			txn.AddAttribute("total.duration_ms", totalDuration.Milliseconds())
		}
		return err
	}

	totalDuration := time.Since(start)
	if txn != nil {
		txn.AddAttribute("handler.status", "success")
		txn.AddAttribute("handler.duration_ms", handlerDuration.Milliseconds())
		txn.AddAttribute("total.duration_ms", totalDuration.Milliseconds())
		responseHandler.AddAttributes(txn, result)
	}

	logger.Info().
		Dur("handler_duration", handlerDuration).
		Dur("validation_duration", validationDuration).
		Dur("total_duration", totalDuration).
		Msg("request completed successfully")

	return responseHandler.Handle(c, result)
}

func Handle[Req validation.Validatable, Res any](
	handler HandlerFunc[Req, Res],
	status int,
	req Req,
) echo.HandlerFunc {
	return func(c echo.Context) error {
		return handleRequest(c, req, func(c echo.Context, req Req) (interface{}, error) {
			return handler(c, req)
		}, JSONResponseHandler{status: status})
	}
}

func HandleFile[Req validation.Validatable](
	handler HandlerFunc[Req, []byte],
	status int,
	req Req,
	filename string,
	contentType string,
) echo.HandlerFunc {
	return func(c echo.Context) error {
		return handleRequest(c, req, func(c echo.Context, req Req) (interface{}, error) {
			return handler(c, req)
		}, FileResponseHandler{
			status:      status,
			filename:    filename,
			contentType: contentType,
		})
	}
}

func HandleNoContent[Req validation.Validatable](
	handler HandlerFuncNoContent[Req],
	status int,
	req Req,
) echo.HandlerFunc {
	return func(c echo.Context) error {
		return handleRequest(c, req, func(c echo.Context, req Req) (interface{}, error) {
			err := handler(c, req)
			return nil, err
		}, NoContentResponseHandler{status: status})
	}
}
