package exam

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Exam, error)
	List(ctx context.Context, limit, offset int) ([]Exam, int, error)
	Create(ctx context.Context, e *Exam) error
	Update(ctx context.Context, e *Exam) error
}

type pgRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{
		pool: pool,
	}
}

func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID) (*Exam, error) {
	// Architectural repository placeholder
	return nil, nil
}

func (r *pgRepository) List(ctx context.Context, limit, offset int) ([]Exam, int, error) {
	// Architectural repository placeholder
	return nil, 0, nil
}

func (r *pgRepository) Create(ctx context.Context, e *Exam) error {
	// Architectural repository placeholder
	return nil
}

func (r *pgRepository) Update(ctx context.Context, e *Exam) error {
	// Architectural repository placeholder
	return nil
}
