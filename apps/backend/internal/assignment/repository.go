package assignment

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Assignment, error)
	GetActiveByExamAndStudent(ctx context.Context, examID, studentID uuid.UUID) (*Assignment, error)
	List(ctx context.Context, limit, offset int, examID, studentID *uuid.UUID, status *Status, search *string) ([]Assignment, int, error)
	ListStudentExams(ctx context.Context, studentID uuid.UUID, limit, offset int, examStatus *string) ([]StudentAssignedExam, int, error)
	Create(ctx context.Context, a *Assignment) error
	Revoke(ctx context.Context, id uuid.UUID) error
	RevokeByExamAndStudent(ctx context.Context, examID, studentID uuid.UUID) error
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
		SELECT a.id, a.exam_id, a.student_id, a.assigned_at, a.status, a.created_at, a.updated_at,
		       COALESCE(TRIM(CONCAT(u.first_name, ' ', u.last_name)), '') AS student_name,
		       COALESCE(u.roll_no, '') AS student_roll_no
		FROM assignments a
		LEFT JOIN users u ON a.student_id = u.id
		WHERE a.id = $1
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
		&a.StudentName,
		&a.StudentRollNo,
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
		SELECT a.id, a.exam_id, a.student_id, a.assigned_at, a.status, a.created_at, a.updated_at,
		       COALESCE(TRIM(CONCAT(u.first_name, ' ', u.last_name)), '') AS student_name,
		       COALESCE(u.roll_no, '') AS student_roll_no
		FROM assignments a
		LEFT JOIN users u ON a.student_id = u.id
		WHERE a.exam_id = $1 AND a.student_id = $2 AND a.status = 'ASSIGNED'
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
		&a.StudentName,
		&a.StudentRollNo,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

func (r *pgRepository) List(ctx context.Context, limit, offset int, examID, studentID *uuid.UUID, status *Status, search *string) ([]Assignment, int, error) {
	var statusFilter *string
	if status != nil {
		s := string(*status)
		statusFilter = &s
	}

	var searchFilter *string
	if search != nil && strings.TrimSpace(*search) != "" {
		s := strings.TrimSpace(*search)
		searchFilter = &s
	}

	countQuery := `
		SELECT COUNT(*)
		FROM assignments a
		LEFT JOIN users u ON a.student_id = u.id
		WHERE ($1::uuid IS NULL OR a.exam_id = $1)
		  AND ($2::uuid IS NULL OR a.student_id = $2)
		  AND ($3::text IS NULL OR a.status = $3)
		  AND ($4::text IS NULL OR (
		      u.roll_no ILIKE '%' || $4 || '%' OR
		      u.first_name ILIKE '%' || $4 || '%' OR
		      u.last_name ILIKE '%' || $4 || '%' OR
		      TRIM(CONCAT(u.first_name, ' ', u.last_name)) ILIKE '%' || $4 || '%'
		  ))
	`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, examID, studentID, statusFilter, searchFilter).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT a.id, a.exam_id, a.student_id, a.assigned_at, a.status, a.created_at, a.updated_at,
		       COALESCE(TRIM(CONCAT(u.first_name, ' ', u.last_name)), '') AS student_name,
		       COALESCE(u.roll_no, '') AS student_roll_no
		FROM assignments a
		LEFT JOIN users u ON a.student_id = u.id
		WHERE ($1::uuid IS NULL OR a.exam_id = $1)
		  AND ($2::uuid IS NULL OR a.student_id = $2)
		  AND ($3::text IS NULL OR a.status = $3)
		  AND ($4::text IS NULL OR (
		      u.roll_no ILIKE '%' || $4 || '%' OR
		      u.first_name ILIKE '%' || $4 || '%' OR
		      u.last_name ILIKE '%' || $4 || '%' OR
		      TRIM(CONCAT(u.first_name, ' ', u.last_name)) ILIKE '%' || $4 || '%'
		  ))
		ORDER BY a.assigned_at DESC
		LIMIT $5 OFFSET $6
	`
	rows, err := r.pool.Query(ctx, query, examID, studentID, statusFilter, searchFilter, limit, offset)
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
			&a.StudentName,
			&a.StudentRollNo,
		); err != nil {
			return nil, 0, err
		}
		assignments = append(assignments, a)
	}

	return assignments, total, rows.Err()
}

func (r *pgRepository) ListStudentExams(ctx context.Context, studentID uuid.UUID, limit, offset int, examStatus *string) ([]StudentAssignedExam, int, error) {
	countQuery := `
		SELECT COUNT(*)
		FROM assignments a
		INNER JOIN exams e ON a.exam_id = e.id
		WHERE a.student_id = $1
		  AND a.status = 'ASSIGNED'
		  AND ($2::text IS NULL OR e.status = $2)
	`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, studentID, examStatus).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT a.id AS assignment_id,
		       a.exam_id,
		       e.title,
		       e.subject,
		       e.description,
		       e.duration_mins,
		       e.status AS exam_status,
		       a.assigned_at,
		       a.status
		FROM assignments a
		INNER JOIN exams e ON a.exam_id = e.id
		WHERE a.student_id = $1
		  AND a.status = 'ASSIGNED'
		  AND ($2::text IS NULL OR e.status = $2)
		ORDER BY a.assigned_at DESC
		LIMIT $3 OFFSET $4
	`
	rows, err := r.pool.Query(ctx, query, studentID, examStatus, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []StudentAssignedExam
	for rows.Next() {
		var item StudentAssignedExam
		if err := rows.Scan(
			&item.AssignmentID,
			&item.ExamID,
			&item.Title,
			&item.Subject,
			&item.Description,
			&item.DurationMins,
			&item.ExamStatus,
			&item.AssignedAt,
			&item.Status,
		); err != nil {
			return nil, 0, err
		}
		list = append(list, item)
	}

	return list, total, rows.Err()
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
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateAssignment
		}
		return err
	}
	return nil
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
		// Check if it exists as already revoked
		var existingStatus string
		checkErr := r.pool.QueryRow(ctx, "SELECT status FROM assignments WHERE id = $1", id).Scan(&existingStatus)
		if checkErr == nil && existingStatus == string(StatusRevoked) {
			return ErrAssignmentAlreadyRevoked
		}
		return ErrAssignmentNotFound
	}
	return nil
}

func (r *pgRepository) RevokeByExamAndStudent(ctx context.Context, examID, studentID uuid.UUID) error {
	query := `
		UPDATE assignments
		SET status = 'REVOKED', updated_at = NOW()
		WHERE exam_id = $1 AND student_id = $2 AND status = 'ASSIGNED'
	`
	res, err := r.pool.Exec(ctx, query, examID, studentID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		var existingStatus string
		checkErr := r.pool.QueryRow(ctx, "SELECT status FROM assignments WHERE exam_id = $1 AND student_id = $2 ORDER BY updated_at DESC LIMIT 1", examID, studentID).Scan(&existingStatus)
		if checkErr == nil && existingStatus == string(StatusRevoked) {
			return ErrAssignmentAlreadyRevoked
		}
		return ErrAssignmentNotFound
	}
	return nil
}
