package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/Satyajeet-Das/ai-scribe/internal/auth"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	jwtadapter "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth/adapters/jwt"
	"github.com/Satyajeet-Das/ai-scribe/internal/user"
)

type MockUserRepo struct {
	mock.Mock
}

func (m *MockUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockUserRepo) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockUserRepo) GetByEmailWithPassword(ctx context.Context, email string) (*user.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockUserRepo) GetByClerkID(ctx context.Context, clerkID string) (*user.User, error) {
	args := m.Called(ctx, clerkID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockUserRepo) Create(ctx context.Context, u *user.User) error {
	args := m.Called(ctx, u)
	return args.Error(0)
}

type MockAuthRepo struct {
	mock.Mock
}

func (m *MockAuthRepo) CreateRefreshToken(ctx context.Context, token *auth.RefreshToken) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockAuthRepo) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*auth.RefreshToken, error) {
	args := m.Called(ctx, tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*auth.RefreshToken), args.Error(1)
}

func (m *MockAuthRepo) UpdateRefreshTokenStatus(ctx context.Context, id uuid.UUID, status auth.RefreshTokenStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *MockAuthRepo) RevokeAllUserTokens(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func TestAuthService_Register(t *testing.T) {
	logger := zerolog.Nop()
	mockUserRepo := new(MockUserRepo)
	mockAuthRepo := new(MockAuthRepo)
	cfg := jwtadapter.Config{
		SecretKey:            "super-secret-test-key-32-bytes-long!",
		AccessTokenDuration:  15 * time.Minute,
		RefreshTokenDuration: 7 * 24 * time.Hour,
		Issuer:               "ai-scribe-test",
	}

	svc := auth.NewService(mockUserRepo, mockAuthRepo, nil, cfg, &logger)
	ctx := context.Background()

	t.Run("Valid registration creates user with bcrypt hash", func(t *testing.T) {
		req := auth.RegisterRequest{
			Email:     "newuser@school.edu",
			Password:  "SecurePassword123!",
			FirstName: "Jane",
			LastName:  "Doe",
			Role:      platformauth.RoleTeacher,
		}

		mockUserRepo.On("Create", ctx, mock.MatchedBy(func(u *user.User) bool {
			return u.Email == req.Email &&
				u.Role == string(platformauth.RoleTeacher) &&
				bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) == nil
		})).Return(nil).Once()

		resp, err := svc.Register(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, req.Email, resp.Email)
		assert.Equal(t, platformauth.RoleTeacher, resp.Role)
		mockUserRepo.AssertExpectations(t)
	})

	t.Run("Password too short returns error", func(t *testing.T) {
		req := auth.RegisterRequest{
			Email:     "short@school.edu",
			Password:  "short",
			FirstName: "Jane",
			LastName:  "Doe",
			Role:      platformauth.RoleTeacher,
		}

		_, err := svc.Register(ctx, req)
		assert.ErrorIs(t, err, auth.ErrPasswordTooShort)
	})

	t.Run("Invalid role returns error", func(t *testing.T) {
		req := auth.RegisterRequest{
			Email:     "role@school.edu",
			Password:  "SecurePassword123!",
			FirstName: "Jane",
			LastName:  "Doe",
			Role:      platformauth.Role("INVALID_ROLE"),
		}

		_, err := svc.Register(ctx, req)
		assert.ErrorIs(t, err, auth.ErrInvalidRole)
	})
}

func TestAuthService_Login(t *testing.T) {
	logger := zerolog.Nop()
	mockUserRepo := new(MockUserRepo)
	mockAuthRepo := new(MockAuthRepo)
	cfg := jwtadapter.Config{
		SecretKey:            "super-secret-test-key-32-bytes-long!",
		AccessTokenDuration:  15 * time.Minute,
		RefreshTokenDuration: 7 * 24 * time.Hour,
		Issuer:               "ai-scribe-test",
	}

	svc := auth.NewService(mockUserRepo, mockAuthRepo, nil, cfg, &logger)
	ctx := context.Background()

	hash, _ := bcrypt.GenerateFromPassword([]byte("Password123!"), bcrypt.DefaultCost)
	testUserID := uuid.New()
	testUser := &user.User{
		Email:        "teacher@school.edu",
		PasswordHash: string(hash),
		FirstName:    "Ada",
		LastName:     "Lovelace",
		Role:         string(platformauth.RoleTeacher),
		IsActive:     true,
	}
	testUser.ID = testUserID

	t.Run("Valid login returns access token and persists refresh token", func(t *testing.T) {
		mockUserRepo.On("GetByEmail", ctx, "teacher@school.edu").Return(testUser, nil).Once()
		mockAuthRepo.On("CreateRefreshToken", ctx, mock.MatchedBy(func(tok *auth.RefreshToken) bool {
			return tok.UserID == testUserID && tok.Status == auth.RefreshTokenStatusActive
		})).Return(nil).Once()

		resp, rawRefresh, expiry, err := svc.Login(ctx, auth.LoginRequest{
			Email:    "teacher@school.edu",
			Password: "Password123!",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, rawRefresh)
		assert.True(t, expiry.After(time.Now()))
		assert.Equal(t, testUser.Email, resp.User.Email)
		mockUserRepo.AssertExpectations(t)
		mockAuthRepo.AssertExpectations(t)
	})

	t.Run("Invalid password returns ErrInvalidCredentials", func(t *testing.T) {
		mockUserRepo.On("GetByEmail", ctx, "teacher@school.edu").Return(testUser, nil).Once()

		_, _, _, err := svc.Login(ctx, auth.LoginRequest{
			Email:    "teacher@school.edu",
			Password: "WrongPassword!",
		})
		assert.ErrorIs(t, err, platformauth.ErrInvalidCredentials)
		mockUserRepo.AssertExpectations(t)
	})

	t.Run("Non-existent email returns ErrInvalidCredentials", func(t *testing.T) {
		mockUserRepo.On("GetByEmail", ctx, "unknown@school.edu").Return(nil, user.ErrUserNotFound).Once()

		_, _, _, err := svc.Login(ctx, auth.LoginRequest{
			Email:    "unknown@school.edu",
			Password: "Password123!",
		})
		assert.ErrorIs(t, err, platformauth.ErrInvalidCredentials)
		mockUserRepo.AssertExpectations(t)
	})
}

func TestAuthService_Refresh(t *testing.T) {
	logger := zerolog.Nop()
	mockUserRepo := new(MockUserRepo)
	mockAuthRepo := new(MockAuthRepo)
	cfg := jwtadapter.Config{
		SecretKey:            "super-secret-test-key-32-bytes-long!",
		AccessTokenDuration:  15 * time.Minute,
		RefreshTokenDuration: 7 * 24 * time.Hour,
		Issuer:               "ai-scribe-test",
	}

	svc := auth.NewService(mockUserRepo, mockAuthRepo, nil, cfg, &logger)
	ctx := context.Background()

	testUserID := uuid.New()
	testTokenID := uuid.New()
	rawToken := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tokenHash := jwtadapter.HashRefreshToken(rawToken)

	activeToken := &auth.RefreshToken{
		ID:        testTokenID,
		UserID:    testUserID,
		TokenHash: tokenHash,
		Status:    auth.RefreshTokenStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	testUser := &user.User{
		Email:    "student@school.edu",
		Role:     string(platformauth.RoleStudent),
		IsActive: true,
	}
	testUser.ID = testUserID

	t.Run("Valid active refresh token rotates to new tokens", func(t *testing.T) {
		mockAuthRepo.On("GetRefreshTokenByHash", ctx, tokenHash).Return(activeToken, nil).Once()
		mockUserRepo.On("GetByID", ctx, testUserID).Return(testUser, nil).Once()
		mockAuthRepo.On("UpdateRefreshTokenStatus", ctx, testTokenID, auth.RefreshTokenStatusConsumed).Return(nil).Once()
		mockAuthRepo.On("CreateRefreshToken", ctx, mock.Anything).Return(nil).Once()

		resp, newRaw, expiry, err := svc.Refresh(ctx, rawToken)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, newRaw)
		assert.NotEqual(t, rawToken, newRaw)
		assert.True(t, expiry.After(time.Now()))
		mockAuthRepo.AssertExpectations(t)
		mockUserRepo.AssertExpectations(t)
	})

	t.Run("Replay of consumed token revokes all user sessions", func(t *testing.T) {
		consumedToken := &auth.RefreshToken{
			ID:        uuid.New(),
			UserID:    testUserID,
			TokenHash: tokenHash,
			Status:    auth.RefreshTokenStatusConsumed,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		mockAuthRepo.On("GetRefreshTokenByHash", ctx, tokenHash).Return(consumedToken, nil).Once()
		mockAuthRepo.On("RevokeAllUserTokens", ctx, testUserID).Return(nil).Once()

		_, _, _, err := svc.Refresh(ctx, rawToken)
		assert.ErrorIs(t, err, platformauth.ErrRevokedToken)
		mockAuthRepo.AssertExpectations(t)
	})
}
