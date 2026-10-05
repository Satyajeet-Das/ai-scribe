package exam

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Exam, error)
	List(ctx context.Context, params ListExamsParams) ([]Exam, int, error)
	Create(ctx context.Context, e *Exam) error
	Update(ctx context.Context, e *Exam) error
	Publish(ctx context.Context, id uuid.UUID, publishedAt time.Time) error
	Unpublish(ctx context.Context, id uuid.UUID) error
	Archive(ctx context.Context, id uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
	HasActiveSessionsOrAssignments(ctx context.Context, examID uuid.UUID) (bool, error)
	IsStudentAssigned(ctx context.Context, examID, studentID uuid.UUID) (bool, error)
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
		SELECT id, title, subject, description, duration_mins, status, created_by, published_at, created_at, updated_at, deleted_at,
		       COALESCE((SELECT COUNT(*) FROM assignments WHERE exam_id = exams.id AND status = 'ASSIGNED'), 0) AS assigned_count
		FROM exams
		WHERE id = $1 AND deleted_at IS NULL
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
		&e.DeletedAt,
		&e.AssignedCount,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrExamNotFound
		}
		return nil, fmt.Errorf("exam.GetByID(%s): %w", id, err)
	}
	return &e, nil
}

func (r *pgRepository) List(ctx context.Context, params ListExamsParams) ([]Exam, int, error) {
	params.Defaults()

	conditions := []string{"deleted_at IS NULL"}
	args := []interface{}{}
	argIdx := 1

	if params.Status != nil && params.Status.IsValid() {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(*params.Status))
		argIdx++
	}

	if params.Subject != "" {
		conditions = append(conditions, fmt.Sprintf("subject ILIKE $%d", argIdx))
		args = append(args, params.Subject)
		argIdx++
	}

	if params.CreatedBy != nil && *params.CreatedBy != uuid.Nil {
		conditions = append(conditions, fmt.Sprintf("created_by = $%d", argIdx))
		args = append(args, *params.CreatedBy)
		argIdx++
	}

	if params.AssignedStudentID != nil && *params.AssignedStudentID != uuid.Nil {
		conditions = append(conditions, fmt.Sprintf("id IN (SELECT exam_id FROM assignments WHERE student_id = $%d AND status = 'ASSIGNED')", argIdx))
		args = append(args, *params.AssignedStudentID)
		argIdx++
	}

	if params.Search != "" {
		searchPattern := "%" + params.Search + "%"
		conditions = append(conditions, fmt.Sprintf("(title ILIKE $%d OR subject ILIKE $%d OR description ILIKE $%d)", argIdx, argIdx, argIdx))
		args = append(args, searchPattern)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM exams WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("exam.List count: %w", err)
	}

	selectQuery := fmt.Sprintf(`
		SELECT id, title, subject, description, duration_mins, status, created_by, published_at, created_at, updated_at, deleted_at,
		       COALESCE((SELECT COUNT(*) FROM assignments WHERE exam_id = exams.id AND status = 'ASSIGNED'), 0) AS assigned_count
		FROM exams
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)

	args = append(args, params.Limit, params.Offset)

	rows, err := r.pool.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("exam.List query: %w", err)
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
			&e.DeletedAt,
			&e.AssignedCount,
		); err != nil {
			return nil, 0, fmt.Errorf("exam.List scan: %w", err)
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
	if err != nil {
		return fmt.Errorf("exam.Create: %w", err)
	}
	return nil
}

func (r *pgRepository) Update(ctx context.Context, e *Exam) error {
	query := `
		UPDATE exams
		SET title = $2, subject = $3, description = $4, duration_mins = $5, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`
	res, err := r.pool.Exec(ctx, query, e.ID, e.Title, e.Subject, e.Description, e.DurationMins)
	if err != nil {
		return fmt.Errorf("exam.Update: %w", err)
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
		WHERE id = $1 AND status = 'DRAFT' AND deleted_at IS NULL
	`
	res, err := r.pool.Exec(ctx, query, id, publishedAt)
	if err != nil {
		return fmt.Errorf("exam.Publish: %w", err)
	}
	if res.RowsAffected() == 0 {
		// Distinguish between not found, already published, or already archived
		current, err := r.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if current.Status == StatusPublished {
			return ErrExamAlreadyPublished
		}
		if current.Status == StatusArchived {
			return ErrExamAlreadyArchived
		}
		return ErrInvalidExamState
	}
	return nil
}

func (r *pgRepository) Unpublish(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE exams
		SET status = 'DRAFT', published_at = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'PUBLISHED' AND deleted_at IS NULL
	`
	res, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("exam.Unpublish: %w", err)
	}
	if res.RowsAffected() == 0 {
		current, err := r.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if current.Status == StatusDraft {
			return ErrExamNotPublished
		}
		if current.Status == StatusArchived {
			return ErrExamAlreadyArchived
		}
		return ErrInvalidExamState
	}
	return nil
}

func (r *pgRepository) Archive(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE exams
		SET status = 'ARCHIVED', updated_at = NOW()
		WHERE id = $1 AND status != 'ARCHIVED' AND deleted_at IS NULL
	`
	res, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("exam.Archive: %w", err)
	}
	if res.RowsAffected() == 0 {
		current, err := r.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if current.Status == StatusArchived {
			return ErrExamAlreadyArchived
		}
		return ErrInvalidExamState
	}
	return nil
}

func (r *pgRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE exams
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`
	res, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("exam.Delete: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrExamNotFound
	}
	return nil
}

func (r *pgRepository) HasActiveSessionsOrAssignments(ctx context.Context, examID uuid.UUID) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM assignments WHERE exam_id = $1
			UNION ALL
			SELECT 1 FROM sessions WHERE exam_id = $1
		)
	`
	var exists bool
	if err := r.pool.QueryRow(ctx, query, examID).Scan(&exists); err != nil {
		return false, fmt.Errorf("exam.HasActiveSessionsOrAssignments: %w", err)
	}
	return exists, nil
}

func (r *pgRepository) IsStudentAssigned(ctx context.Context, examID, studentID uuid.UUID) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM assignments
			WHERE exam_id = $1 AND student_id = $2 AND status = 'ASSIGNED'
		)
	`
	var exists bool
	if err := r.pool.QueryRow(ctx, query, examID, studentID).Scan(&exists); err != nil {
		return false, fmt.Errorf("exam.IsStudentAssigned: %w", err)
	}
	return exists, nil
}
