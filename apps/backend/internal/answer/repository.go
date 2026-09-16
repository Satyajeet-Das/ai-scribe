package answer

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Answer, error)
	ListBySessionID(ctx context.Context, sessionID uuid.UUID) ([]Answer, error)
	Create(ctx context.Context, a *Answer) error
}

type pgRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{
		pool: pool,
	}
}

func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID) (*Answer, error) {
	return nil, nil
}

func (r *pgRepository) ListBySessionID(ctx context.Context, sessionID uuid.UUID) ([]Answer, error) {
	return nil, nil
}

func (r *pgRepository) Create(ctx context.Context, a *Answer) error {
	return nil
}
