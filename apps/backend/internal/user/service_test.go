package user_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/Satyajeet-Das/ai-scribe/internal/user"
)

type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockRepository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockRepository) GetByEmailWithPassword(ctx context.Context, email string) (*user.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockRepository) GetByClerkID(ctx context.Context, clerkID string) (*user.User, error) {
	args := m.Called(ctx, clerkID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockRepository) Create(ctx context.Context, u *user.User) error {
	args := m.Called(ctx, u)
	return args.Error(0)
}

func TestUserService(t *testing.T) {
	logger := zerolog.Nop()
	mockRepo := new(MockRepository)
	svc := user.NewService(mockRepo, &logger)

	ctx := context.Background()
	testUser := &user.User{
		Email:     "test@example.com",
		FirstName: "John",
		LastName:  "Doe",
		Role:      "TEACHER",
		IsActive:  true,
	}

	t.Run("CreateUser", func(t *testing.T) {
		mockRepo.On("Create", ctx, testUser).Return(nil).Once()
		err := svc.CreateUser(ctx, testUser)
		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("GetUserByEmail", func(t *testing.T) {
		mockRepo.On("GetByEmail", ctx, "test@example.com").Return(testUser, nil).Once()
		u, err := svc.GetUserByEmail(ctx, "test@example.com")
		assert.NoError(t, err)
		assert.Equal(t, "test@example.com", u.Email)
		mockRepo.AssertExpectations(t)
	})
}
