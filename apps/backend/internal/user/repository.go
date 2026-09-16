package user

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByClerkID(ctx context.Context, clerkID string) (*User, error)
	Create(ctx context.Context, u *User) error
}

type pgRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{
		pool: pool,
	}
}

func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	// Architectural placeholder for future query execution
	return nil, nil
}

func (r *pgRepository) GetByClerkID(ctx context.Context, clerkID string) (*User, error) {
	// Architectural placeholder for future query execution
	return nil, nil
}

func (r *pgRepository) Create(ctx context.Context, u *User) error {
	// Architectural placeholder for future query execution
	return nil
}
