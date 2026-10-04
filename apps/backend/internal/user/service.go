package user

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type StudentSearchResult struct {
	ID     uuid.UUID `json:"id"`
	RollNo string    `json:"rollNo"`
	Name   string    `json:"name"`
	Email  string    `json:"email"`
}

func (s StudentSearchResult) MarshalJSON() ([]byte, error) {
	type Alias StudentSearchResult
	return json.Marshal(&struct {
		Alias
		RollNoSnake string `json:"roll_no"`
	}{
		Alias:       (Alias)(s),
		RollNoSnake: s.RollNo,
	})
}

type Service interface {
	GetUser(ctx context.Context, id uuid.UUID) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByClerkID(ctx context.Context, clerkID string) (*User, error)
	GetUserByRollNo(ctx context.Context, rollNo string) (*User, error)
	SearchStudents(ctx context.Context, query string, limit int) ([]StudentSearchResult, error)
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

func (s *userService) GetUserByRollNo(ctx context.Context, rollNo string) (*User, error) {
	return s.repo.GetByRollNo(ctx, rollNo)
}

func (s *userService) SearchStudents(ctx context.Context, query string, limit int) ([]StudentSearchResult, error) {
	users, err := s.repo.SearchStudents(ctx, query, limit)
	if err != nil {
		s.logger.Error().Err(err).Str("query", query).Msg("failed to search students")
		return nil, err
	}

	results := make([]StudentSearchResult, len(users))
	for i, u := range users {
		rollNo := ""
		if u.RollNo != nil {
			rollNo = *u.RollNo
		}
		name := strings.TrimSpace(u.FirstName + " " + u.LastName)
		if name == "" {
			name = u.Email
		}
		results[i] = StudentSearchResult{
			ID:     u.ID,
			RollNo: rollNo,
			Name:   name,
			Email:  u.Email,
		}
	}
	return results, nil
}

func (s *userService) CreateUser(ctx context.Context, u *User) error {
	return s.repo.Create(ctx, u)
}
