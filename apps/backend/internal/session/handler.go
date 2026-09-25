package session

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/validation"
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

	sessions.POST("", h.StartSession)
	sessions.GET("/:id", h.GetSession)
	sessions.POST("/:id/submit", h.SubmitSession)
}

func (h *Handler) StartSession(c echo.Context) error {
	var req StartSessionRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	userIDStr, _ := c.Get("user_id").(string)
	callerID, _ := uuid.Parse(userIDStr)

	sess, durationMins, err := h.service.StartSession(c.Request().Context(), req, callerID)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusCreated, ToSessionResponse(sess, durationMins))
}

func (h *Handler) GetSession(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid session ID format", false, nil, nil, nil)
	}

	userIDStr, _ := c.Get("user_id").(string)
	callerID, _ := uuid.Parse(userIDStr)

	sess, durationMins, err := h.service.GetSession(c.Request().Context(), id, callerID)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, ToSessionResponse(sess, durationMins))
}

func (h *Handler) SubmitSession(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid session ID format", false, nil, nil, nil)
	}

	userIDStr, _ := c.Get("user_id").(string)
	callerID, _ := uuid.Parse(userIDStr)

	sess, err := h.service.SubmitSession(c.Request().Context(), id, callerID)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, SubmitSessionResponse{
		ID:          sess.ID,
		Status:      sess.Status,
		SubmittedAt: *sess.SubmittedAt,
		Message:     "Exam session submitted successfully",
	})
}

func (h *Handler) mapError(err error) error {
	if errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrInvalidAssignment) || errors.Is(err, exam.ErrExamNotFound) {
		return httperrs.NewNotFoundError(err.Error(), true, nil)
	}
	if errors.Is(err, ErrUnauthorizedStudent) {
		return httperrs.NewForbiddenError("Forbidden: only the assigned student can access this session", false)
	}
	if errors.Is(err, ErrSessionExpired) ||
		errors.Is(err, ErrSessionAlreadySubmitted) ||
		errors.Is(err, ErrSessionNotInProgress) ||
		errors.Is(err, ErrAssignmentRevoked) ||
		errors.Is(err, ErrExamArchived) ||
		errors.Is(err, ErrExamNotPublished) ||
		errors.Is(err, ErrActiveSessionAlreadyExists) {
		return httperrs.NewBadRequestError(err.Error(), false, nil, nil, nil)
	}
	return httperrs.NewInternalServerError()
}
