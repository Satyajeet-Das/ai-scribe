package assignment

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Assignment, error)
	GetByExamAndCandidate(ctx context.Context, examID, candidateID uuid.UUID) (*Assignment, error)
	Create(ctx context.Context, a *Assignment) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status Status) error
}

type pgRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{
		pool: pool,
	}
}

func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID) (*Assignment, error) {
	return nil, nil
}

func (r *pgRepository) GetByExamAndCandidate(ctx context.Context, examID, candidateID uuid.UUID) (*Assignment, error) {
	return nil, nil
}

func (r *pgRepository) Create(ctx context.Context, a *Assignment) error {
	return nil
}

func (r *pgRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status Status) error {
	return nil
}
