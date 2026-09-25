package exam

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Exam, error)
	List(ctx context.Context, limit, offset int, status *Status) ([]Exam, int, error)
	Create(ctx context.Context, e *Exam) error
	Update(ctx context.Context, e *Exam) error
	Publish(ctx context.Context, id uuid.UUID, publishedAt time.Time) error
	Archive(ctx context.Context, id uuid.UUID) error
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
	query := `
		SELECT id, title, subject, description, duration_mins, status, created_by, published_at, created_at, updated_at
		FROM exams
		WHERE id = $1
	`
	var e Exam
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&e.ID,
		&e.Title,
		&e.Subject,
		&e.Description,
		&e.DurationMins,
		&e.Status,
		&e.CreatedBy,
		&e.PublishedAt,
		&e.CreatedAt,
		&e.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrExamNotFound
		}
		return nil, err
	}
	return &e, nil
}

func (r *pgRepository) List(ctx context.Context, limit, offset int, status *Status) ([]Exam, int, error) {
	var statusFilter *string
	if status != nil {
		s := string(*status)
		statusFilter = &s
	}

	countQuery := `
		SELECT COUNT(*)
		FROM exams
		WHERE ($1::text IS NULL OR status = $1)
	`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, statusFilter).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, title, subject, description, duration_mins, status, created_by, published_at, created_at, updated_at
		FROM exams
		WHERE ($1::text IS NULL OR status = $1)
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.pool.Query(ctx, query, statusFilter, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var exams []Exam
	for rows.Next() {
		var e Exam
		if err := rows.Scan(
			&e.ID,
			&e.Title,
			&e.Subject,
			&e.Description,
			&e.DurationMins,
			&e.Status,
			&e.CreatedBy,
			&e.PublishedAt,
			&e.CreatedAt,
			&e.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		exams = append(exams, e)
	}

	return exams, total, rows.Err()
}

func (r *pgRepository) Create(ctx context.Context, e *Exam) error {
	query := `
		INSERT INTO exams (id, title, subject, description, duration_mins, status, created_by, published_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	now := time.Now().UTC()
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	e.CreatedAt = now
	e.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		e.ID,
		e.Title,
		e.Subject,
		e.Description,
		e.DurationMins,
		e.Status,
		e.CreatedBy,
		e.PublishedAt,
		e.CreatedAt,
		e.UpdatedAt,
	)
	return err
}

func (r *pgRepository) Update(ctx context.Context, e *Exam) error {
	query := `
		UPDATE exams
		SET title = $2, subject = $3, description = $4, duration_mins = $5, updated_at = NOW()
		WHERE id = $1
	`
	res, err := r.pool.Exec(ctx, query, e.ID, e.Title, e.Subject, e.Description, e.DurationMins)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrExamNotFound
	}
	return nil
}

func (r *pgRepository) Publish(ctx context.Context, id uuid.UUID, publishedAt time.Time) error {
	query := `
		UPDATE exams
		SET status = 'PUBLISHED', published_at = $2, updated_at = NOW()
		WHERE id = $1 AND status = 'DRAFT'
	`
	res, err := r.pool.Exec(ctx, query, id, publishedAt)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrInvalidExamState
	}
	return nil
}

func (r *pgRepository) Archive(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE exams
		SET status = 'ARCHIVED', updated_at = NOW()
		WHERE id = $1
	`
	res, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrExamNotFound
	}
	return nil
}
