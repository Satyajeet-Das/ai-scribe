package question

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (*Question, error)
	GetByIDWithOptions(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (*Question, error)
	ListByExamID(ctx context.Context, examID uuid.UUID, tx ...database.DBTX) ([]Question, error)
	Create(ctx context.Context, q *Question, tx ...database.DBTX) error
	Update(ctx context.Context, q *Question, tx ...database.DBTX) error
	Delete(ctx context.Context, id uuid.UUID, tx ...database.DBTX) error

	GetOptionByID(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (*QuestionOption, error)
	ListOptionsByQuestionID(ctx context.Context, questionID uuid.UUID, tx ...database.DBTX) ([]QuestionOption, error)
	CreateOption(ctx context.Context, opt *QuestionOption, tx ...database.DBTX) error
	UpdateOption(ctx context.Context, opt *QuestionOption, tx ...database.DBTX) error
	DeleteOption(ctx context.Context, id uuid.UUID, tx ...database.DBTX) error
	SetCorrectOption(ctx context.Context, questionID, optionID uuid.UUID, tx ...database.DBTX) error

	GetMaxQuestionNumber(ctx context.Context, examID uuid.UUID, tx ...database.DBTX) (int, error)
	ReorderQuestions(ctx context.Context, examID uuid.UUID, orderMap map[uuid.UUID]int, tx ...database.DBTX) error
	ResequenceQuestions(ctx context.Context, examID uuid.UUID, tx ...database.DBTX) error
}

type pgRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{
		pool: pool,
	}
}

func (r *pgRepository) getDB(tx ...database.DBTX) database.DBTX {
	if len(tx) > 0 && tx[0] != nil {
		return tx[0]
	}
	return r.pool
}

func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (*Question, error) {
	db := r.getDB(tx...)
	query := `
		SELECT id, exam_id, question_number, text, type, points, created_at, updated_at
		FROM questions
		WHERE id = $1
	`
	var q Question
	err := db.QueryRow(ctx, query, id).Scan(
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

func (r *pgRepository) GetByIDWithOptions(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (*Question, error) {
	q, err := r.GetByID(ctx, id, tx...)
	if err != nil {
		return nil, err
	}

	opts, err := r.ListOptionsByQuestionID(ctx, id, tx...)
	if err != nil {
		return nil, err
	}
	q.Options = opts
	return q, nil
}

func (r *pgRepository) ListByExamID(ctx context.Context, examID uuid.UUID, tx ...database.DBTX) ([]Question, error) {
	db := r.getDB(tx...)
	query := `
		SELECT id, exam_id, question_number, text, type, points, created_at, updated_at
		FROM questions
		WHERE exam_id = $1
		ORDER BY question_number ASC
	`
	rows, err := db.Query(ctx, query, examID)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range questions {
		opts, err := r.ListOptionsByQuestionID(ctx, questions[i].ID, tx...)
		if err != nil {
			return nil, err
		}
		questions[i].Options = opts
	}

	return questions, nil
}

func (r *pgRepository) Create(ctx context.Context, q *Question, tx ...database.DBTX) error {
	db := r.getDB(tx...)
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

	_, err := db.Exec(ctx, query,
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

func (r *pgRepository) Update(ctx context.Context, q *Question, tx ...database.DBTX) error {
	db := r.getDB(tx...)
	query := `
		UPDATE questions
		SET question_number = $2, text = $3, points = $4, updated_at = NOW()
		WHERE id = $1
	`
	res, err := db.Exec(ctx, query, q.ID, q.QuestionNumber, q.Text, q.Points)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrQuestionNotFound
	}
	return nil
}

func (r *pgRepository) Delete(ctx context.Context, id uuid.UUID, tx ...database.DBTX) error {
	db := r.getDB(tx...)
	query := `DELETE FROM questions WHERE id = $1`
	res, err := db.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrQuestionNotFound
	}
	return nil
}

func (r *pgRepository) GetOptionByID(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (*QuestionOption, error) {
	db := r.getDB(tx...)
	query := `
		SELECT id, question_id, option_key, option_text, display_order, is_correct, created_at, updated_at
		FROM question_options
		WHERE id = $1
	`
	var opt QuestionOption
	err := db.QueryRow(ctx, query, id).Scan(
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

func (r *pgRepository) ListOptionsByQuestionID(ctx context.Context, questionID uuid.UUID, tx ...database.DBTX) ([]QuestionOption, error) {
	db := r.getDB(tx...)
	query := `
		SELECT id, question_id, option_key, option_text, display_order, is_correct, created_at, updated_at
		FROM question_options
		WHERE question_id = $1
		ORDER BY display_order ASC, option_key ASC
	`
	rows, err := db.Query(ctx, query, questionID)
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

func (r *pgRepository) CreateOption(ctx context.Context, opt *QuestionOption, tx ...database.DBTX) error {
	db := r.getDB(tx...)
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

	_, err := db.Exec(ctx, query,
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

func (r *pgRepository) UpdateOption(ctx context.Context, opt *QuestionOption, tx ...database.DBTX) error {
	db := r.getDB(tx...)
	query := `
		UPDATE question_options
		SET option_key = $2, option_text = $3, display_order = $4, is_correct = $5, updated_at = NOW()
		WHERE id = $1 AND question_id = $6
	`
	res, err := db.Exec(ctx, query, opt.ID, opt.OptionKey, opt.OptionText, opt.DisplayOrder, opt.IsCorrect, opt.QuestionID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrOptionNotFound
	}
	return nil
}

func (r *pgRepository) DeleteOption(ctx context.Context, id uuid.UUID, tx ...database.DBTX) error {
	db := r.getDB(tx...)
	query := `DELETE FROM question_options WHERE id = $1`
	res, err := db.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrOptionNotFound
	}
	return nil
}

func (r *pgRepository) SetCorrectOption(ctx context.Context, questionID, optionID uuid.UUID, tx ...database.DBTX) error {
	db := r.getDB(tx...)
	resetQuery := `UPDATE question_options SET is_correct = FALSE, updated_at = NOW() WHERE question_id = $1`
	if _, err := db.Exec(ctx, resetQuery, questionID); err != nil {
		return err
	}

	setQuery := `UPDATE question_options SET is_correct = TRUE, updated_at = NOW() WHERE id = $1 AND question_id = $2`
	res, err := db.Exec(ctx, setQuery, optionID, questionID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrOptionNotFound
	}
	return nil
}

func (r *pgRepository) GetMaxQuestionNumber(ctx context.Context, examID uuid.UUID, tx ...database.DBTX) (int, error) {
	db := r.getDB(tx...)
	query := `SELECT COALESCE(MAX(question_number), 0) FROM questions WHERE exam_id = $1`
	var maxNum int
	err := db.QueryRow(ctx, query, examID).Scan(&maxNum)
	if err != nil {
		return 0, err
	}
	return maxNum, nil
}

func (r *pgRepository) ReorderQuestions(ctx context.Context, examID uuid.UUID, orderMap map[uuid.UUID]int, tx ...database.DBTX) error {
	db := r.getDB(tx...)
	// Pass 1: Set temporary question numbers to avoid violating uq_exam_question_number
	// Value 1000000 + targetNum ensures value is positive to satisfy CHECK (question_number > 0)
	for id, targetNum := range orderMap {
		tempNum := 1000000 + targetNum
		query := `UPDATE questions SET question_number = $1, updated_at = NOW() WHERE id = $2 AND exam_id = $3`
		if _, err := db.Exec(ctx, query, tempNum, id, examID); err != nil {
			return err
		}
	}
	// Pass 2: Set final target question numbers
	for id, targetNum := range orderMap {
		query := `UPDATE questions SET question_number = $1, updated_at = NOW() WHERE id = $2 AND exam_id = $3`
		if _, err := db.Exec(ctx, query, targetNum, id, examID); err != nil {
			return err
		}
	}
	return nil
}

func (r *pgRepository) ResequenceQuestions(ctx context.Context, examID uuid.UUID, tx ...database.DBTX) error {
	db := r.getDB(tx...)
	query := `SELECT id, question_number FROM questions WHERE exam_id = $1 ORDER BY question_number ASC`
	rows, err := db.Query(ctx, query, examID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type qNum struct {
		id  uuid.UUID
		num int
	}
	var items []qNum
	for rows.Next() {
		var item qNum
		if err := rows.Scan(&item.id, &item.num); err != nil {
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	orderMap := make(map[uuid.UUID]int)
	for i, item := range items {
		target := i + 1
		if item.num != target {
			orderMap[item.id] = target
		}
	}

	if len(orderMap) > 0 {
		return r.ReorderQuestions(ctx, examID, orderMap, tx...)
	}
	return nil
}
