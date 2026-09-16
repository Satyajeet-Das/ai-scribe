package assignment

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
	assignments := g.Group("/assignments")
	if authMiddleware != nil {
		assignments.Use(authMiddleware)
	}

	assignments.GET("/:id", h.GetAssignment)
	assignments.POST("", h.CreateAssignment)
}

func (h *Handler) GetAssignment(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid assignment ID format", false, nil, nil, nil)
	}

	a, err := h.service.GetAssignment(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, ErrAssignmentNotFound) {
			return httperrs.NewNotFoundError("Assignment not found", true, nil)
		}
		return httperrs.NewInternalServerError()
	}

	return c.JSON(http.StatusOK, ToAssignmentResponse(a))
}

func (h *Handler) CreateAssignment(c echo.Context) error {
	var req CreateAssignmentRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	a, err := h.service.CreateAssignment(c.Request().Context(), req)
	if err != nil {
		return httperrs.NewInternalServerError()
	}

	return c.JSON(http.StatusCreated, ToAssignmentResponse(a))
}
