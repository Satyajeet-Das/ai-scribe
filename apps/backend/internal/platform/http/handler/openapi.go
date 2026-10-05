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
	specBytes, err := os.ReadFile("static/openapi.json")
	if err != nil {
		// Fallback to static template if json reading fails
		templateBytes, readErr := os.ReadFile("static/openapi.html")
		if readErr != nil {
			return fmt.Errorf("failed to read OpenAPI UI template: %w", readErr)
		}
		c.Response().Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		return c.HTML(http.StatusOK, string(templateBytes))
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
  <head>
    <title>AI Exam Scribe API Reference</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
  </head>
  <body>
    <script id="api-reference" type="application/json">
%s
    </script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  </body>
</html>`, string(specBytes))

	c.Response().Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Response().Header().Set("Pragma", "no-cache")
	c.Response().Header().Set("Expires", "0")

	return c.HTML(http.StatusOK, html)
}
