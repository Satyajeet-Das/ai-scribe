package question

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Question, error)
	ListByExamID(ctx context.Context, examID uuid.UUID) ([]Question, error)
	Create(ctx context.Context, q *Question) error
}

type pgRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{
		pool: pool,
	}
}

func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID) (*Question, error) {
	return nil, nil
}

func (r *pgRepository) ListByExamID(ctx context.Context, examID uuid.UUID) ([]Question, error) {
	return nil, nil
}

func (r *pgRepository) Create(ctx context.Context, q *Question) error {
	return nil
}
