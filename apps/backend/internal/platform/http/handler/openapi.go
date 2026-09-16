package handler

import (
	"fmt"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
)

type OpenAPIHandler struct{}

func NewOpenAPIHandler() *OpenAPIHandler {
	return &OpenAPIHandler{}
}

func (h *OpenAPIHandler) ServeOpenAPIUI(c echo.Context) error {
	templateBytes, err := os.ReadFile("static/openapi.html")
	if err != nil {
		return fmt.Errorf("failed to read OpenAPI UI template: %w", err)
	}

	c.Response().Header().Set("Cache-Control", "no-cache")
	if err := c.HTML(http.StatusOK, string(templateBytes)); err != nil {
		return fmt.Errorf("failed to write HTML response: %w", err)
	}

	return nil
}
