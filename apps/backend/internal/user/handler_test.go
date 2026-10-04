package user_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/user"
)

type MockUserService struct {
	mock.Mock
}

func (m *MockUserService) GetUser(ctx context.Context, id uuid.UUID) (*user.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockUserService) GetUserByEmail(ctx context.Context, email string) (*user.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockUserService) GetUserByClerkID(ctx context.Context, clerkID string) (*user.User, error) {
	args := m.Called(ctx, clerkID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockUserService) GetUserByRollNo(ctx context.Context, rollNo string) (*user.User, error) {
	args := m.Called(ctx, rollNo)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockUserService) SearchStudents(ctx context.Context, query string, limit int) ([]user.StudentSearchResult, error) {
	args := m.Called(ctx, query, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]user.StudentSearchResult), args.Error(1)
}

func (m *MockUserService) CreateUser(ctx context.Context, u *user.User) error {
	args := m.Called(ctx, u)
	return args.Error(0)
}

func TestHandler_SearchStudents(t *testing.T) {
	e := echo.New()
	mockSvc := new(MockUserService)
	handler := user.NewHandler(mockSvc)

	studentUUID := uuid.New()
	mockResults := []user.StudentSearchResult{
		{
			ID:     studentUUID,
			RollNo: "23CS001",
			Name:   "Rahul Sharma",
			Email:  "rahul@example.com",
		},
	}

	t.Run("successful search returns students list", func(t *testing.T) {
		mockSvc.On("SearchStudents", mock.Anything, "23CS", 10).Return(mockResults, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/students/search?q=23CS&limit=10", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.SearchStudents(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var res []user.StudentSearchResult
		err = json.Unmarshal(rec.Body.Bytes(), &res)
		require.NoError(t, err)
		assert.Len(t, res, 1)
		assert.Equal(t, "23CS001", res[0].RollNo)
		assert.Equal(t, "Rahul Sharma", res[0].Name)
		assert.Equal(t, studentUUID, res[0].ID)

		mockSvc.AssertExpectations(t)
	})

	t.Run("default limit is 10 when invalid or missing", func(t *testing.T) {
		mockSvc.On("SearchStudents", mock.Anything, "Priya", 10).Return([]user.StudentSearchResult{}, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/students/search?q=Priya", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.SearchStudents(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		mockSvc.AssertExpectations(t)
	})
}

func TestHandler_RoleMiddlewareGuards(t *testing.T) {
	e := echo.New()
	mockSvc := new(MockUserService)
	handler := user.NewHandler(mockSvc)

	// Setup fake auth provider/middleware
	authMiddleware := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return next(c)
		}
	}
	requireRole := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			role := c.Request().Header.Get("X-Role")
			if role != string(platformauth.RoleTeacher) && role != string(platformauth.RoleAdmin) {
				return echo.NewHTTPError(http.StatusForbidden, "Forbidden")
			}
			return next(c)
		}
	}

	g := e.Group("/api/v1")
	handler.RegisterRoutes(g, authMiddleware, requireRole)

	t.Run("forbidden for student", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/students/search?q=23CS", nil)
		req.Header.Set("X-Role", "STUDENT")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("allowed for teacher", func(t *testing.T) {
		mockSvc.On("SearchStudents", mock.Anything, "23CS", 10).Return([]user.StudentSearchResult{}, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/students/search?q=23CS", nil)
		req.Header.Set("X-Role", "TEACHER")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		mockSvc.AssertExpectations(t)
	})
}
