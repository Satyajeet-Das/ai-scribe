package answer

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/validation"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) RegisterRoutes(g *echo.Group, authMiddleware echo.MiddlewareFunc) {
	sessions := g.Group("/sessions")
	if authMiddleware != nil {
		sessions.Use(authMiddleware)
	}

	sessions.PUT("/:session_id/questions/:question_id/answer", h.SubmitAnswer)
	sessions.GET("/:session_id/answers", h.ListAnswers)
}

func (h *Handler) SubmitAnswer(c echo.Context) error {
	sessionIDParam := c.Param("session_id")
	sessionID, err := uuid.Parse(sessionIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid session ID format", false, nil, nil, nil)
	}

	questionIDParam := c.Param("question_id")
	questionID, err := uuid.Parse(questionIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid question ID format", false, nil, nil, nil)
	}

	var req SubmitAnswerRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	userIDStr, _ := c.Get("user_id").(string)
	callerID, _ := uuid.Parse(userIDStr)

	ans, err := h.service.SubmitAnswer(c.Request().Context(), sessionID, questionID, req, callerID)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, ToAnswerResponse(ans))
}

func (h *Handler) ListAnswers(c echo.Context) error {
	sessionIDParam := c.Param("session_id")
	sessionID, err := uuid.Parse(sessionIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid session ID format", false, nil, nil, nil)
	}

	userIDStr, _ := c.Get("user_id").(string)
	callerID, _ := uuid.Parse(userIDStr)

	answers, err := h.service.ListAnswers(c.Request().Context(), sessionID, callerID)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, ToAnswerResponseList(answers))
}

func (h *Handler) mapError(err error) error {
	if errors.Is(err, session.ErrSessionNotFound) ||
		errors.Is(err, question.ErrQuestionNotFound) ||
		errors.Is(err, question.ErrOptionNotFound) ||
		errors.Is(err, ErrAnswerNotFound) {
		return httperrs.NewNotFoundError(err.Error(), true, nil)
	}

	if errors.Is(err, ErrUnauthorizedStudent) {
		return httperrs.NewForbiddenError("Forbidden: only the assigned student can access answers for this session", false)
	}

	if errors.Is(err, ErrSessionNotActive) ||
		errors.Is(err, ErrQuestionNotForExam) ||
		errors.Is(err, ErrOptionNotForQuestion) ||
		errors.Is(err, ErrInvalidAnswer) {
		return httperrs.NewBadRequestError(err.Error(), false, nil, nil, nil)
	}

	return httperrs.NewInternalServerError()
}
