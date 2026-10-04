package user

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(g *echo.Group, authMiddleware echo.MiddlewareFunc, requireRoleMiddleware echo.MiddlewareFunc) {
	students := g.Group("/students")
	if authMiddleware != nil {
		students.Use(authMiddleware)
	}
	if requireRoleMiddleware != nil {
		students.Use(requireRoleMiddleware)
	}

	students.GET("/search", h.SearchStudents)
}

func (h *Handler) SearchStudents(c echo.Context) error {
	query := c.QueryParam("q")
	limit := 10
	if l := c.QueryParam("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	results, err := h.service.SearchStudents(c.Request().Context(), query, limit)
	if err != nil {
		return httperrs.NewInternalServerError()
	}

	return c.JSON(http.StatusOK, results)
}
