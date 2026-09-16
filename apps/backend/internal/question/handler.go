package question

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
	questions := g.Group("/questions")
	if authMiddleware != nil {
		questions.Use(authMiddleware)
	}

	questions.GET("/:id", h.GetQuestion)
	questions.POST("", h.CreateQuestion)
}

func (h *Handler) GetQuestion(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid question ID format", false, nil, nil, nil)
	}

	q, err := h.service.GetQuestion(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, ErrQuestionNotFound) {
			return httperrs.NewNotFoundError("Question not found", true, nil)
		}
		return httperrs.NewInternalServerError()
	}

	return c.JSON(http.StatusOK, ToQuestionResponse(q))
}

func (h *Handler) CreateQuestion(c echo.Context) error {
	var req CreateQuestionRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	q, err := h.service.CreateQuestion(c.Request().Context(), req)
	if err != nil {
		return httperrs.NewInternalServerError()
	}

	return c.JSON(http.StatusCreated, ToQuestionResponse(q))
}
