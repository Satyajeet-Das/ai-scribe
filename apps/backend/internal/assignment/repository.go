package assignment

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Assignment, error)
	GetActiveByExamAndStudent(ctx context.Context, examID, studentID uuid.UUID) (*Assignment, error)
	List(ctx context.Context, limit, offset int, examID, studentID *uuid.UUID, status *Status) ([]Assignment, int, error)
	Create(ctx context.Context, a *Assignment) error
	Revoke(ctx context.Context, id uuid.UUID) error
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
	query := `
		SELECT id, exam_id, student_id, assigned_at, status, created_at, updated_at
		FROM assignments
		WHERE id = $1
	`
	var a Assignment
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&a.ID,
		&a.ExamID,
		&a.StudentID,
		&a.AssignedAt,
		&a.Status,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAssignmentNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (r *pgRepository) GetActiveByExamAndStudent(ctx context.Context, examID, studentID uuid.UUID) (*Assignment, error) {
	query := `
		SELECT id, exam_id, student_id, assigned_at, status, created_at, updated_at
		FROM assignments
		WHERE exam_id = $1 AND student_id = $2 AND status = 'ASSIGNED'
	`
	var a Assignment
	err := r.pool.QueryRow(ctx, query, examID, studentID).Scan(
		&a.ID,
		&a.ExamID,
		&a.StudentID,
		&a.AssignedAt,
		&a.Status,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

func (r *pgRepository) List(ctx context.Context, limit, offset int, examID, studentID *uuid.UUID, status *Status) ([]Assignment, int, error) {
	var statusFilter *string
	if status != nil {
		s := string(*status)
		statusFilter = &s
	}

	countQuery := `
		SELECT COUNT(*)
		FROM assignments
		WHERE ($1::uuid IS NULL OR exam_id = $1)
		  AND ($2::uuid IS NULL OR student_id = $2)
		  AND ($3::text IS NULL OR status = $3)
	`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, examID, studentID, statusFilter).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, exam_id, student_id, assigned_at, status, created_at, updated_at
		FROM assignments
		WHERE ($1::uuid IS NULL OR exam_id = $1)
		  AND ($2::uuid IS NULL OR student_id = $2)
		  AND ($3::text IS NULL OR status = $3)
		ORDER BY assigned_at DESC
		LIMIT $4 OFFSET $5
	`
	rows, err := r.pool.Query(ctx, query, examID, studentID, statusFilter, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var assignments []Assignment
	for rows.Next() {
		var a Assignment
		if err := rows.Scan(
			&a.ID,
			&a.ExamID,
			&a.StudentID,
			&a.AssignedAt,
			&a.Status,
			&a.CreatedAt,
			&a.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		assignments = append(assignments, a)
	}

	return assignments, total, rows.Err()
}

func (r *pgRepository) Create(ctx context.Context, a *Assignment) error {
	query := `
		INSERT INTO assignments (id, exam_id, student_id, assigned_at, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	now := time.Now().UTC()
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.AssignedAt.IsZero() {
		a.AssignedAt = now
	}
	a.CreatedAt = now
	a.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		a.ID,
		a.ExamID,
		a.StudentID,
		a.AssignedAt,
		a.Status,
		a.CreatedAt,
		a.UpdatedAt,
	)
	return err
}

func (r *pgRepository) Revoke(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE assignments
		SET status = 'REVOKED', updated_at = NOW()
		WHERE id = $1 AND status = 'ASSIGNED'
	`
	res, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrAssignmentNotFound
	}
	return nil
}
