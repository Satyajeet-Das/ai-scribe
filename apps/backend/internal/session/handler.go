package session

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
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

	sessions.GET("/:id", h.GetSession)
}

func (h *Handler) GetSession(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid session ID format", false, nil, nil, nil)
	}

	sess, err := h.service.GetSession(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return httperrs.NewNotFoundError("Session not found", true, nil)
		}
		return httperrs.NewInternalServerError()
	}

	return c.JSON(http.StatusOK, ToSessionResponse(sess))
}
