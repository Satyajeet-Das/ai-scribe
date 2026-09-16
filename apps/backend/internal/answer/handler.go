package answer

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

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
	answers := g.Group("/answers")
	if authMiddleware != nil {
		answers.Use(authMiddleware)
	}

	answers.GET("/:id", h.GetAnswer)
	answers.POST("", h.SubmitAnswer)
}

func (h *Handler) GetAnswer(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid answer ID format", false, nil, nil, nil)
	}

	a, err := h.service.GetAnswer(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, ErrAnswerNotFound) {
			return httperrs.NewNotFoundError("Answer not found", true, nil)
		}
		return httperrs.NewInternalServerError()
	}

	return c.JSON(http.StatusOK, ToAnswerResponse(a))
}

func (h *Handler) SubmitAnswer(c echo.Context) error {
	var req SubmitAnswerRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	userIDStr, _ := c.Get("user_id").(string)
	candidateID, _ := uuid.Parse(userIDStr)

	a, err := h.service.SubmitAnswer(c.Request().Context(), req, candidateID)
	if err != nil {
		return httperrs.NewInternalServerError()
	}

	return c.JSON(http.StatusCreated, ToAnswerResponse(a))
}
