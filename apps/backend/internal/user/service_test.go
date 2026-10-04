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

func (m *MockRepository) GetByRollNo(ctx context.Context, rollNo string) (*user.User, error) {
	args := m.Called(ctx, rollNo)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func (m *MockRepository) SearchStudents(ctx context.Context, query string, limit int) ([]user.User, error) {
	args := m.Called(ctx, query, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]user.User), args.Error(1)
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
	roll := "23CS001"
	testUser := &user.User{
		Email:     "test@example.com",
		FirstName: "John",
		LastName:  "Doe",
		Role:      "STUDENT",
		RollNo:    &roll,
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

	t.Run("GetUserByRollNo", func(t *testing.T) {
		mockRepo.On("GetByRollNo", ctx, "23CS001").Return(testUser, nil).Once()
		u, err := svc.GetUserByRollNo(ctx, "23CS001")
		assert.NoError(t, err)
		assert.Equal(t, "23CS001", *u.RollNo)
		mockRepo.AssertExpectations(t)
	})

	t.Run("SearchStudents", func(t *testing.T) {
		mockRepo.On("SearchStudents", ctx, "23CS", 10).Return([]user.User{*testUser}, nil).Once()
		results, err := svc.SearchStudents(ctx, "23CS", 10)
		assert.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "23CS001", results[0].RollNo)
		assert.Equal(t, "John Doe", results[0].Name)
		assert.Equal(t, "test@example.com", results[0].Email)
		mockRepo.AssertExpectations(t)
	})
}
