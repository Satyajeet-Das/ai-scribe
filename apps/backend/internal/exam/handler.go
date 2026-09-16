package exam

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
	exams := g.Group("/exams")
	if authMiddleware != nil {
		exams.Use(authMiddleware)
	}

	exams.GET("/:id", h.GetExam)
	exams.POST("", h.CreateExam)
}

func (h *Handler) GetExam(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	exam, err := h.service.GetExam(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, ErrExamNotFound) {
			return httperrs.NewNotFoundError("Exam not found", true, nil)
		}
		return httperrs.NewInternalServerError()
	}

	return c.JSON(http.StatusOK, ToExamResponse(exam))
}

func (h *Handler) CreateExam(c echo.Context) error {
	var req CreateExamRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	userIDStr, _ := c.Get("user_id").(string)
	createdBy, _ := uuid.Parse(userIDStr)

	exam, err := h.service.CreateExam(c.Request().Context(), req, createdBy)
	if err != nil {
		return httperrs.NewInternalServerError()
	}

	return c.JSON(http.StatusCreated, ToExamResponse(exam))
}
