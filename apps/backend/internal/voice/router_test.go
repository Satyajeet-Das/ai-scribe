package voice

import (
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	httpPlatform "github.com/Satyajeet-Das/ai-scribe/internal/platform/http"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/middleware"
)

// TestConnect_ThroughProductionMiddlewareChain upgrades through the same
// router and global middleware stack the server uses (rate limiter, CORS,
// secure headers, request ID, tracing, context enhancer, request logger,
// recover), to prove none of them breaks the connection hijack.
func TestConnect_ThroughProductionMiddlewareChain(t *testing.T) {
	h := newHarness(t, withoutTTS())

	logger := zerolog.Nop()
	cfg := &config.Config{Server: config.ServerConfig{CORSAllowedOrigins: []string{"https://exam.example.com"}}}
	router := httpPlatform.NewRouter(httpPlatform.RouterParams{
		Middlewares: middleware.NewMiddlewares(cfg, &logger, nil),
		Logger:      &logger,
	})
	h.svc.RegisterRoutes(router.Group("/api/v1"))
	prod := httptest.NewServer(router)
	defer prod.Close()
	h.server.Close()
	h.server = prod

	_, resp, err := h.dialRaw(dialOptions{})
	require.Error(t, err)
	assert.Equal(t, 401, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("X-Request-ID"))

	c, ready := h.connect()
	assert.Equal(t, h.sessionID, ready.SessionID)
	_, r := command(t, c, CommandPayload{Intent: "next_question"})
	assert.Contains(t, r.Text, "Question 2 of 3")
}
