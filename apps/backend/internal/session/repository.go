package session

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Session, error)
	GetActiveByCandidateID(ctx context.Context, candidateID uuid.UUID) (*Session, error)
	Create(ctx context.Context, s *Session) error
	Update(ctx context.Context, s *Session) error
}

type pgRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{
		pool: pool,
	}
}

func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID) (*Session, error) {
	return nil, nil
}

func (r *pgRepository) GetActiveByCandidateID(ctx context.Context, candidateID uuid.UUID) (*Session, error) {
	return nil, nil
}

func (r *pgRepository) Create(ctx context.Context, s *Session) error {
	return nil
}

func (r *pgRepository) Update(ctx context.Context, s *Session) error {
	return nil
}
