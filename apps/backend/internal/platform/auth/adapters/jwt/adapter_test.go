package jwt_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

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

func TestJWTAdapter_Authenticate(t *testing.T) {
	cfg := jwtadapter.Config{
		SecretKey:            "super-secret-test-key-32-bytes-long!",
		AccessTokenDuration:  15 * time.Minute,
		RefreshTokenDuration: 7 * 24 * time.Hour,
		Issuer:               "ai-scribe-test",
	}
	logger := zerolog.Nop()
	mockRepo := new(MockUserRepo)
	adapter := jwtadapter.NewAdapter(cfg, mockRepo, nil, &logger)

	testUserID := uuid.New()
	testUser := &user.User{
		Email:     "teacher@school.edu",
		FirstName: "Ada",
		LastName:  "Lovelace",
		Role:      string(platformauth.RoleTeacher),
		IsActive:  true,
	}
	testUser.ID = testUserID

	ctx := context.Background()

	t.Run("Valid token returns AuthenticatedIdentity", func(t *testing.T) {
		tokenStr, _, _, err := jwtadapter.GenerateAccessToken(cfg, testUserID, platformauth.RoleTeacher, testUser.Email)
		require.NoError(t, err)

		mockRepo.On("GetByID", ctx, testUserID).Return(testUser, nil).Once()

		identity, err := adapter.Authenticate(ctx, tokenStr)
		require.NoError(t, err)
		assert.Equal(t, testUserID, identity.UserID)
		assert.Equal(t, platformauth.RoleTeacher, identity.Role)
		assert.Equal(t, testUser.Email, identity.Email)
		assert.Equal(t, "jwt", identity.Provider)
		mockRepo.AssertExpectations(t)
	})

	t.Run("Expired token returns ErrExpiredToken", func(t *testing.T) {
		expiredClaims := jwtadapter.Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-10 * time.Minute)),
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(-20 * time.Minute)),
				Issuer:    cfg.Issuer,
			},
			UserID: testUserID,
			Role:   platformauth.RoleTeacher,
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims)
		tokenStr, err := tok.SignedString([]byte(cfg.SecretKey))
		require.NoError(t, err)

		identity, err := adapter.Authenticate(ctx, tokenStr)
		assert.ErrorIs(t, err, platformauth.ErrExpiredToken)
		assert.Nil(t, identity)
	})

	t.Run("Invalid signature returns ErrInvalidToken", func(t *testing.T) {
		tokenStr, _, _, err := jwtadapter.GenerateAccessToken(cfg, testUserID, platformauth.RoleTeacher, testUser.Email)
		require.NoError(t, err)

		tamperedAdapter := jwtadapter.NewAdapter(jwtadapter.Config{SecretKey: "wrong-key-different-secret-key!!"}, mockRepo, nil, &logger)
		identity, err := tamperedAdapter.Authenticate(ctx, tokenStr)
		assert.ErrorIs(t, err, platformauth.ErrInvalidToken)
		assert.Nil(t, identity)
	})

	t.Run("Empty or malformed token returns ErrInvalidToken", func(t *testing.T) {
		_, err := adapter.Authenticate(ctx, "")
		assert.ErrorIs(t, err, platformauth.ErrInvalidToken)

		_, err = adapter.Authenticate(ctx, "not-a-jwt.token.here")
		assert.ErrorIs(t, err, platformauth.ErrInvalidToken)
	})

	t.Run("Deactivated user returns ErrUserDeactivated", func(t *testing.T) {
		inactiveUserID := uuid.New()
		inactiveUser := &user.User{
			Email:    "banned@school.edu",
			Role:     string(platformauth.RoleStudent),
			IsActive: false,
		}
		inactiveUser.ID = inactiveUserID

		tokenStr, _, _, err := jwtadapter.GenerateAccessToken(cfg, inactiveUserID, platformauth.RoleStudent, inactiveUser.Email)
		require.NoError(t, err)

		mockRepo.On("GetByID", ctx, inactiveUserID).Return(inactiveUser, nil).Once()

		identity, err := adapter.Authenticate(ctx, tokenStr)
		assert.ErrorIs(t, err, platformauth.ErrUserDeactivated)
		assert.Nil(t, identity)
		mockRepo.AssertExpectations(t)
	})

	t.Run("Non-existent user returns ErrUserNotFound", func(t *testing.T) {
		missingUserID := uuid.New()
		tokenStr, _, _, err := jwtadapter.GenerateAccessToken(cfg, missingUserID, platformauth.RoleStudent, "missing@test.com")
		require.NoError(t, err)

		mockRepo.On("GetByID", ctx, missingUserID).Return(nil, user.ErrUserNotFound).Once()

		identity, err := adapter.Authenticate(ctx, tokenStr)
		assert.ErrorIs(t, err, platformauth.ErrUserNotFound)
		assert.Nil(t, identity)
		mockRepo.AssertExpectations(t)
	})
}

func TestRefreshTokenGeneration(t *testing.T) {
	cfg := jwtadapter.Config{
		RefreshTokenDuration: 7 * 24 * time.Hour,
	}

	raw, hash, expiresAt, err := jwtadapter.GenerateRefreshToken(cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, raw)
	assert.NotEmpty(t, hash)
	assert.True(t, expiresAt.After(time.Now()))
	assert.Equal(t, hash, jwtadapter.HashRefreshToken(raw))
}
