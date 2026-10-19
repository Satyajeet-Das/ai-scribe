package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/middleware"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/logger"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/redis"
)

// The playback route is limited per reviewer, so one teacher cannot bulk-copy
// recordings by spreading requests over addresses, and colleagues sharing an
// address do not use up each other's allowance.
func TestRateLimit_PerUser_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	testDB, cleanup := SetupTestDB(t)
	defer cleanup()

	logSvc := logger.NewLoggerService(testDB.Config.Observability)
	l := logger.NewLoggerWithService(testDB.Config.Observability, logSvc)
	client := redis.New(testDB.Config, &l, logSvc)
	defer client.Close()

	limit := middleware.NewRateLimitMiddleware(nil).WithRedis(client, &l).LimitPerUser(2, time.Minute)
	e := echo.New()
	e.GET("/audio", func(c echo.Context) error { return c.NoContent(http.StatusOK) }, func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_id", c.Request().Header.Get("X-Test-User"))
			return next(c)
		}
	}, limit)

	play := func(user, ip string) int {
		req := httptest.NewRequest(http.MethodGet, "/audio", nil)
		req.Header.Set("X-Test-User", user)
		req.Header.Set("X-Real-IP", ip)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}

	assert.Equal(t, http.StatusOK, play("teacher-a", "198.51.100.1"))
	assert.Equal(t, http.StatusOK, play("teacher-a", "198.51.100.2"), "a new address is the same reviewer")
	assert.Equal(t, http.StatusTooManyRequests, play("teacher-a", "198.51.100.3"))
	assert.Equal(t, http.StatusOK, play("teacher-b", "198.51.100.1"), "a colleague on the same address is not limited")
}
