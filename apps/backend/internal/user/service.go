package user

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type Service interface {
	GetUser(ctx context.Context, id uuid.UUID) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByClerkID(ctx context.Context, clerkID string) (*User, error)
	CreateUser(ctx context.Context, u *User) error
}

type userService struct {
	repo   Repository
	logger *zerolog.Logger
}

func NewService(repo Repository, logger *zerolog.Logger) Service {
	return &userService{
		repo:   repo,
		logger: logger,
	}
}

func (s *userService) GetUser(ctx context.Context, id uuid.UUID) (*User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *userService) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return s.repo.GetByEmail(ctx, email)
}

func (s *userService) GetUserByClerkID(ctx context.Context, clerkID string) (*User, error) {
	return s.repo.GetByClerkID(ctx, clerkID)
}

func (s *userService) CreateUser(ctx context.Context, u *User) error {
	return s.repo.Create(ctx, u)
}
