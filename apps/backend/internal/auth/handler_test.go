package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/auth"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/user"
)

type MockAuthService struct {
	mock.Mock
}

func (m *MockAuthService) Register(ctx context.Context, req auth.RegisterRequest) (*auth.UserResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*auth.UserResponse), args.Error(1)
}

func (m *MockAuthService) Login(ctx context.Context, req auth.LoginRequest) (*auth.LoginResponse, string, time.Time, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, "", time.Time{}, args.Error(3)
	}
	return args.Get(0).(*auth.LoginResponse), args.String(1), args.Get(2).(time.Time), args.Error(3)
}

func (m *MockAuthService) Refresh(ctx context.Context, rawRefreshToken string) (*auth.RefreshResponse, string, time.Time, error) {
	args := m.Called(ctx, rawRefreshToken)
	if args.Get(0) == nil {
		return nil, "", time.Time{}, args.Error(3)
	}
	return args.Get(0).(*auth.RefreshResponse), args.String(1), args.Get(2).(time.Time), args.Error(3)
}

func (m *MockAuthService) Logout(ctx context.Context, identity *platformauth.AuthenticatedIdentity, rawRefreshToken string) error {
	args := m.Called(ctx, identity, rawRefreshToken)
	return args.Error(0)
}

func (m *MockAuthService) GetMe(ctx context.Context, userID uuid.UUID) (*auth.UserResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*auth.UserResponse), args.Error(1)
}

func TestAuthHandler(t *testing.T) {
	mockSvc := new(MockAuthService)
	handler := auth.NewHandler(mockSvc, false)
	e := echo.New()

	t.Run("POST /register success", func(t *testing.T) {
		reqBody := auth.RegisterRequest{
			Email:     "teacher@school.edu",
			Password:  "SecurePassword123!",
			FirstName: "Grace",
			LastName:  "Hopper",
			Role:      platformauth.RoleTeacher,
		}
		jsonBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		mockSvc.On("Register", mock.Anything, reqBody).Return(&auth.UserResponse{
			ID:        uuid.New(),
			Email:     reqBody.Email,
			FirstName: reqBody.FirstName,
			LastName:  reqBody.LastName,
			Role:      platformauth.RoleTeacher,
		}, nil).Once()

		err := handler.Register(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)
		mockSvc.AssertExpectations(t)
	})

	t.Run("POST /register duplicate conflict", func(t *testing.T) {
		reqBody := auth.RegisterRequest{
			Email:     "exists@school.edu",
			Password:  "SecurePassword123!",
			FirstName: "Grace",
			LastName:  "Hopper",
			Role:      platformauth.RoleTeacher,
		}
		jsonBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		mockSvc.On("Register", mock.Anything, reqBody).Return(nil, user.ErrUserAlreadyExists).Once()

		err := handler.Register(c)
		assert.Error(t, err)
		mockSvc.AssertExpectations(t)
	})

	t.Run("POST /login success sets cookie", func(t *testing.T) {
		reqBody := auth.LoginRequest{
			Email:    "teacher@school.edu",
			Password: "Password123!",
		}
		jsonBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		loginResp := &auth.LoginResponse{
			AccessToken: "dummy.jwt.token",
			ExpiresIn:   900,
			User: auth.UserResponse{
				Email: reqBody.Email,
				Role:  platformauth.RoleTeacher,
			},
		}
		mockSvc.On("Login", mock.Anything, reqBody).Return(loginResp, "raw-refresh-token", time.Now().Add(7*24*time.Hour), nil).Once()

		err := handler.Login(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Check cookie
		cookies := rec.Result().Cookies()
		var refreshCookie *http.Cookie
		for _, ck := range cookies {
			if ck.Name == auth.RefreshTokenCookieName {
				refreshCookie = ck
				break
			}
		}
		require.NotNil(t, refreshCookie)
		assert.Equal(t, "raw-refresh-token", refreshCookie.Value)
		assert.True(t, refreshCookie.HttpOnly)
		mockSvc.AssertExpectations(t)
	})

	t.Run("POST /refresh success with header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		req.Header.Set("X-Refresh-Token", "old-refresh-token")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		refreshResp := &auth.RefreshResponse{
			AccessToken: "new.jwt.token",
			ExpiresIn:   900,
		}
		mockSvc.On("Refresh", mock.Anything, "old-refresh-token").Return(refreshResp, "new-refresh-token", time.Now().Add(7*24*time.Hour), nil).Once()

		err := handler.Refresh(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		mockSvc.AssertExpectations(t)
	})

	t.Run("POST /logout clears cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		req.AddCookie(&http.Cookie{
			Name:  auth.RefreshTokenCookieName,
			Value: "token-to-revoke",
		})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		mockSvc.On("Logout", mock.Anything, mock.Anything, "token-to-revoke").Return(nil).Once()

		err := handler.Logout(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		cookies := rec.Result().Cookies()
		var refreshCookie *http.Cookie
		for _, ck := range cookies {
			if ck.Name == auth.RefreshTokenCookieName {
				refreshCookie = ck
				break
			}
		}
		require.NotNil(t, refreshCookie)
		assert.Equal(t, -1, refreshCookie.MaxAge)
		mockSvc.AssertExpectations(t)
	})

	t.Run("GET /me returns caller profile", func(t *testing.T) {
		testUserID := uuid.New()
		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		platformauth.SetIdentity(c, &platformauth.AuthenticatedIdentity{
			UserID: testUserID,
			Role:   platformauth.RoleTeacher,
		})

		mockSvc.On("GetMe", mock.Anything, testUserID).Return(&auth.UserResponse{
			ID:        testUserID,
			Email:     "teacher@school.edu",
			FirstName: "Grace",
			LastName:  "Hopper",
			Role:      platformauth.RoleTeacher,
		}, nil).Once()

		err := handler.Me(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		mockSvc.AssertExpectations(t)
	})
}
