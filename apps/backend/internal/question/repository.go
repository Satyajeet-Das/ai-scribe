package question

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Question, error)
	GetByIDWithOptions(ctx context.Context, id uuid.UUID) (*Question, error)
	ListByExamID(ctx context.Context, examID uuid.UUID) ([]Question, error)
	Create(ctx context.Context, q *Question) error
	Update(ctx context.Context, q *Question) error
	Delete(ctx context.Context, id uuid.UUID) error

	GetOptionByID(ctx context.Context, id uuid.UUID) (*QuestionOption, error)
	ListOptionsByQuestionID(ctx context.Context, questionID uuid.UUID) ([]QuestionOption, error)
	CreateOption(ctx context.Context, opt *QuestionOption) error
	DeleteOption(ctx context.Context, id uuid.UUID) error
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
	query := `
		SELECT id, exam_id, question_number, text, type, points, created_at, updated_at
		FROM questions
		WHERE id = $1
	`
	var q Question
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&q.ID,
		&q.ExamID,
		&q.QuestionNumber,
		&q.Text,
		&q.Type,
		&q.Points,
		&q.CreatedAt,
		&q.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrQuestionNotFound
		}
		return nil, err
	}
	return &q, nil
}

func (r *pgRepository) GetByIDWithOptions(ctx context.Context, id uuid.UUID) (*Question, error) {
	q, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	opts, err := r.ListOptionsByQuestionID(ctx, id)
	if err != nil {
		return nil, err
	}
	q.Options = opts
	return q, nil
}

func (r *pgRepository) ListByExamID(ctx context.Context, examID uuid.UUID) ([]Question, error) {
	query := `
		SELECT id, exam_id, question_number, text, type, points, created_at, updated_at
		FROM questions
		WHERE exam_id = $1
		ORDER BY question_number ASC
	`
	rows, err := r.pool.Query(ctx, query, examID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var questions []Question
	for rows.Next() {
		var q Question
		if err := rows.Scan(
			&q.ID,
			&q.ExamID,
			&q.QuestionNumber,
			&q.Text,
			&q.Type,
			&q.Points,
			&q.CreatedAt,
			&q.UpdatedAt,
		); err != nil {
			return nil, err
		}
		questions = append(questions, q)
	}

	for i := range questions {
		opts, err := r.ListOptionsByQuestionID(ctx, questions[i].ID)
		if err != nil {
			return nil, err
		}
		questions[i].Options = opts
	}

	return questions, rows.Err()
}

func (r *pgRepository) Create(ctx context.Context, q *Question) error {
	query := `
		INSERT INTO questions (id, exam_id, question_number, text, type, points, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	now := time.Now().UTC()
	if q.ID == uuid.Nil {
		q.ID = uuid.New()
	}
	q.CreatedAt = now
	q.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		q.ID,
		q.ExamID,
		q.QuestionNumber,
		q.Text,
		q.Type,
		q.Points,
		q.CreatedAt,
		q.UpdatedAt,
	)
	return err
}

func (r *pgRepository) Update(ctx context.Context, q *Question) error {
	query := `
		UPDATE questions
		SET question_number = $2, text = $3, points = $4, updated_at = NOW()
		WHERE id = $1
	`
	res, err := r.pool.Exec(ctx, query, q.ID, q.QuestionNumber, q.Text, q.Points)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrQuestionNotFound
	}
	return nil
}

func (r *pgRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM questions WHERE id = $1`
	res, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrQuestionNotFound
	}
	return nil
}

func (r *pgRepository) GetOptionByID(ctx context.Context, id uuid.UUID) (*QuestionOption, error) {
	query := `
		SELECT id, question_id, option_key, option_text, display_order, is_correct, created_at, updated_at
		FROM question_options
		WHERE id = $1
	`
	var opt QuestionOption
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&opt.ID,
		&opt.QuestionID,
		&opt.OptionKey,
		&opt.OptionText,
		&opt.DisplayOrder,
		&opt.IsCorrect,
		&opt.CreatedAt,
		&opt.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOptionNotFound
		}
		return nil, err
	}
	return &opt, nil
}

func (r *pgRepository) ListOptionsByQuestionID(ctx context.Context, questionID uuid.UUID) ([]QuestionOption, error) {
	query := `
		SELECT id, question_id, option_key, option_text, display_order, is_correct, created_at, updated_at
		FROM question_options
		WHERE question_id = $1
		ORDER BY display_order ASC, option_key ASC
	`
	rows, err := r.pool.Query(ctx, query, questionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var options []QuestionOption
	for rows.Next() {
		var opt QuestionOption
		if err := rows.Scan(
			&opt.ID,
			&opt.QuestionID,
			&opt.OptionKey,
			&opt.OptionText,
			&opt.DisplayOrder,
			&opt.IsCorrect,
			&opt.CreatedAt,
			&opt.UpdatedAt,
		); err != nil {
			return nil, err
		}
		options = append(options, opt)
	}
	return options, rows.Err()
}

func (r *pgRepository) CreateOption(ctx context.Context, opt *QuestionOption) error {
	query := `
		INSERT INTO question_options (id, question_id, option_key, option_text, display_order, is_correct, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	now := time.Now().UTC()
	if opt.ID == uuid.Nil {
		opt.ID = uuid.New()
	}
	opt.CreatedAt = now
	opt.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		opt.ID,
		opt.QuestionID,
		opt.OptionKey,
		opt.OptionText,
		opt.DisplayOrder,
		opt.IsCorrect,
		opt.CreatedAt,
		opt.UpdatedAt,
	)
	return err
}

func (r *pgRepository) DeleteOption(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM question_options WHERE id = $1`
	res, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrOptionNotFound
	}
	return nil
}
