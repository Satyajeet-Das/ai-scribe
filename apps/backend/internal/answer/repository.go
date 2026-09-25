package answer

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Answer, error)
	GetBySessionAndQuestion(ctx context.Context, sessionID, questionID uuid.UUID) (*Answer, error)
	ListBySession(ctx context.Context, sessionID uuid.UUID) ([]Answer, error)
	Upsert(ctx context.Context, a *Answer) error
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
	query := `
		SELECT id, session_id, question_id, selected_option_id, text_answer, created_at, updated_at
		FROM answers
		WHERE id = $1
	`
	var a Answer
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&a.ID,
		&a.SessionID,
		&a.QuestionID,
		&a.SelectedOptionID,
		&a.TextAnswer,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAnswerNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (r *pgRepository) GetBySessionAndQuestion(ctx context.Context, sessionID, questionID uuid.UUID) (*Answer, error) {
	query := `
		SELECT id, session_id, question_id, selected_option_id, text_answer, created_at, updated_at
		FROM answers
		WHERE session_id = $1 AND question_id = $2
	`
	var a Answer
	err := r.pool.QueryRow(ctx, query, sessionID, questionID).Scan(
		&a.ID,
		&a.SessionID,
		&a.QuestionID,
		&a.SelectedOptionID,
		&a.TextAnswer,
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

func (r *pgRepository) ListBySession(ctx context.Context, sessionID uuid.UUID) ([]Answer, error) {
	query := `
		SELECT id, session_id, question_id, selected_option_id, text_answer, created_at, updated_at
		FROM answers
		WHERE session_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.pool.Query(ctx, query, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var answers []Answer
	for rows.Next() {
		var a Answer
		if err := rows.Scan(
			&a.ID,
			&a.SessionID,
			&a.QuestionID,
			&a.SelectedOptionID,
			&a.TextAnswer,
			&a.CreatedAt,
			&a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		answers = append(answers, a)
	}
	return answers, rows.Err()
}

func (r *pgRepository) Upsert(ctx context.Context, a *Answer) error {
	query := `
		INSERT INTO answers (id, session_id, question_id, selected_option_id, text_answer, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (session_id, question_id)
		DO UPDATE SET
			selected_option_id = EXCLUDED.selected_option_id,
			text_answer = EXCLUDED.text_answer,
			updated_at = NOW()
		RETURNING id, created_at, updated_at
	`
	now := time.Now().UTC()
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	a.CreatedAt = now
	a.UpdatedAt = now

	return r.pool.QueryRow(ctx, query,
		a.ID,
		a.SessionID,
		a.QuestionID,
		a.SelectedOptionID,
		a.TextAnswer,
		a.CreatedAt,
		a.UpdatedAt,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}
