package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
)

type MockAuthProvider struct {
	mock.Mock
}

func (m *MockAuthProvider) Authenticate(ctx context.Context, rawToken string) (*platformauth.AuthenticatedIdentity, error) {
	args := m.Called(ctx, rawToken)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*platformauth.AuthenticatedIdentity), args.Error(1)
}

func TestMiddleware_RequireAuth(t *testing.T) {
	logger := zerolog.Nop()
	mockProvider := new(MockAuthProvider)
	mw := platformauth.NewMiddleware(mockProvider, &logger)

	e := echo.New()

	t.Run("Missing Authorization header returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw.RequireAuth(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})

		err := handler(c)
		assert.Error(t, err)
	})

	t.Run("Invalid Authorization format returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Basic some-credentials")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw.RequireAuth(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})

		err := handler(c)
		assert.Error(t, err)
	})

	t.Run("Provider authentication error returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		mockProvider.On("Authenticate", mock.Anything, "invalid-token").
			Return(nil, errors.New("bad token")).Once()

		handler := mw.RequireAuth(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})

		err := handler(c)
		assert.Error(t, err)
		mockProvider.AssertExpectations(t)
	})

	t.Run("Valid token succeeds and sets identity in context", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		testID := uuid.New()
		expectedIdentity := &platformauth.AuthenticatedIdentity{
			UserID:    testID,
			Role:      platformauth.RoleTeacher,
			Email:     "teacher@school.edu",
			Provider:  "jwt",
			ExpiresAt: time.Now().Add(15 * time.Minute),
		}

		mockProvider.On("Authenticate", mock.Anything, "valid-token").
			Return(expectedIdentity, nil).Once()

		called := false
		handler := mw.RequireAuth(func(c echo.Context) error {
			called = true
			identity, ok := platformauth.GetIdentity(c)
			require.True(t, ok)
			assert.Equal(t, testID, identity.UserID)
			assert.Equal(t, platformauth.RoleTeacher, identity.Role)
			assert.Equal(t, testID.String(), c.Get("user_id"))
			return c.String(http.StatusOK, "ok")
		})

		err := handler(c)
		assert.NoError(t, err)
		assert.True(t, called)
		mockProvider.AssertExpectations(t)
	})
}

func TestMiddleware_RequireRole(t *testing.T) {
	logger := zerolog.Nop()
	mockProvider := new(MockAuthProvider)
	mw := platformauth.NewMiddleware(mockProvider, &logger)

	e := echo.New()

	t.Run("Allowed role passes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		platformauth.SetIdentity(c, &platformauth.AuthenticatedIdentity{
			UserID: uuid.New(),
			Role:   platformauth.RoleTeacher,
		})

		called := false
		handler := mw.RequireRole(platformauth.RoleTeacher)(func(c echo.Context) error {
			called = true
			return c.String(http.StatusOK, "ok")
		})

		err := handler(c)
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("Admin always passes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		platformauth.SetIdentity(c, &platformauth.AuthenticatedIdentity{
			UserID: uuid.New(),
			Role:   platformauth.RoleAdmin,
		})

		called := false
		handler := mw.RequireRole(platformauth.RoleStudent)(func(c echo.Context) error {
			called = true
			return c.String(http.StatusOK, "ok")
		})

		err := handler(c)
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("Insufficient role returns 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		platformauth.SetIdentity(c, &platformauth.AuthenticatedIdentity{
			UserID: uuid.New(),
			Role:   platformauth.RoleStudent,
		})

		called := false
		handler := mw.RequireRole(platformauth.RoleTeacher)(func(c echo.Context) error {
			called = true
			return c.String(http.StatusOK, "ok")
		})

		err := handler(c)
		assert.Error(t, err)
		assert.False(t, called)
	})
}
