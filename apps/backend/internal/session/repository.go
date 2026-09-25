package session

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Session, error)
	GetActiveByAssignmentID(ctx context.Context, assignmentID uuid.UUID) (*Session, error)
	ListByStudentID(ctx context.Context, studentID uuid.UUID, limit, offset int) ([]Session, int, error)
	Create(ctx context.Context, s *Session) error
	Submit(ctx context.Context, id uuid.UUID, submittedAt time.Time) error
	Expire(ctx context.Context, id uuid.UUID) error
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
	query := `
		SELECT id, assignment_id, exam_id, student_id, status, started_at, submitted_at, created_at, updated_at
		FROM sessions
		WHERE id = $1
	`
	var s Session
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&s.ID,
		&s.AssignmentID,
		&s.ExamID,
		&s.StudentID,
		&s.Status,
		&s.StartedAt,
		&s.SubmittedAt,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return &s, nil
}

func (r *pgRepository) GetActiveByAssignmentID(ctx context.Context, assignmentID uuid.UUID) (*Session, error) {
	query := `
		SELECT id, assignment_id, exam_id, student_id, status, started_at, submitted_at, created_at, updated_at
		FROM sessions
		WHERE assignment_id = $1 AND status = 'IN_PROGRESS'
	`
	var s Session
	err := r.pool.QueryRow(ctx, query, assignmentID).Scan(
		&s.ID,
		&s.AssignmentID,
		&s.ExamID,
		&s.StudentID,
		&s.Status,
		&s.StartedAt,
		&s.SubmittedAt,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

func (r *pgRepository) ListByStudentID(ctx context.Context, studentID uuid.UUID, limit, offset int) ([]Session, int, error) {
	countQuery := `
		SELECT COUNT(*)
		FROM sessions
		WHERE student_id = $1
	`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, studentID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, assignment_id, exam_id, student_id, status, started_at, submitted_at, created_at, updated_at
		FROM sessions
		WHERE student_id = $1
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.pool.Query(ctx, query, studentID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(
			&s.ID,
			&s.AssignmentID,
			&s.ExamID,
			&s.StudentID,
			&s.Status,
			&s.StartedAt,
			&s.SubmittedAt,
			&s.CreatedAt,
			&s.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		sessions = append(sessions, s)
	}

	return sessions, total, rows.Err()
}

func (r *pgRepository) Create(ctx context.Context, s *Session) error {
	query := `
		INSERT INTO sessions (id, assignment_id, exam_id, student_id, status, started_at, submitted_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	now := time.Now().UTC()
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.StartedAt.IsZero() {
		s.StartedAt = now
	}
	s.CreatedAt = now
	s.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		s.ID,
		s.AssignmentID,
		s.ExamID,
		s.StudentID,
		s.Status,
		s.StartedAt,
		s.SubmittedAt,
		s.CreatedAt,
		s.UpdatedAt,
	)
	return err
}

func (r *pgRepository) Submit(ctx context.Context, id uuid.UUID, submittedAt time.Time) error {
	query := `
		UPDATE sessions
		SET status = 'SUBMITTED', submitted_at = $2, updated_at = NOW()
		WHERE id = $1 AND status = 'IN_PROGRESS'
	`
	res, err := r.pool.Exec(ctx, query, id, submittedAt)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrSessionNotInProgress
	}
	return nil
}

func (r *pgRepository) Expire(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE sessions
		SET status = 'EXPIRED', updated_at = NOW()
		WHERE id = $1 AND status = 'IN_PROGRESS'
	`
	res, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrSessionNotInProgress
	}
	return nil
}
