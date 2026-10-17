package grading

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Satyajeet-Das/ai-scribe/internal/review"
)

// PaperRow is a paper's grading record. A paper that has never been written to
// has no row, and counts as a draft at version 0.
type PaperRow struct {
	SessionID   uuid.UUID
	Status      Status
	Version     int
	FinalisedBy *uuid.UUID
	FinalisedAt *time.Time
	TotalHalf   *int
	MaxHalf     *int
	Snapshot    *Snapshot
}

// MarkRow is what a teacher saved for one answer.
type MarkRow struct {
	QuestionID uuid.UUID
	MarksHalf  *int
	MaxHalf    int
	Comment    *string
	Version    int
	MarkedAt   time.Time
}

// Reader is a consistent, read-only view of one exam's marking data.
type Reader interface {
	// Session is the attempt the review shows for the student: nil if they have
	// not started, review.ErrStudentNotAssigned if they are not assigned.
	Session(ctx context.Context, examID, studentID uuid.UUID) (*review.SessionInfo, error)
	// Paper returns the grading record, or nil if there is none yet.
	Paper(ctx context.Context, sessionID uuid.UUID) (*PaperRow, error)
	// Marks returns the teacher's marks for a session, by question.
	Marks(ctx context.Context, sessionID uuid.UUID) (map[uuid.UUID]MarkRow, error)
	// Questions returns the exam's questions, in order, with the session's answers.
	Questions(ctx context.Context, examID, sessionID uuid.UUID) ([]QuestionInput, error)
}

// Repository gives access to marking data.
type Repository interface {
	// Read runs fn against one consistent snapshot. Nothing can be written in it.
	Read(ctx context.Context, fn func(Reader) error) error
}

type pgRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

func (r *pgRepository) Read(ctx context.Context, fn func(Reader) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return fn(pgReader{tx: tx})
}

type pgReader struct {
	tx pgx.Tx
}

func (r pgReader) Session(ctx context.Context, examID, studentID uuid.UUID) (*review.SessionInfo, error) {
	return review.ResolveSession(ctx, r.tx, examID, studentID)
}

func (r pgReader) Paper(ctx context.Context, sessionID uuid.UUID) (*PaperRow, error) {
	var p PaperRow
	var status string
	var snapshot []byte
	err := r.tx.QueryRow(ctx, `
		SELECT session_id, status, version, finalised_by, finalised_at, total_half, max_half, snapshot
		FROM paper_gradings WHERE session_id = $1`, sessionID,
	).Scan(&p.SessionID, &status, &p.Version, &p.FinalisedBy, &p.FinalisedAt, &p.TotalHalf, &p.MaxHalf, &snapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.Status = Status(status)
	if len(snapshot) > 0 {
		var snap Snapshot
		if err := json.Unmarshal(snapshot, &snap); err != nil {
			return nil, err
		}
		p.Snapshot = &snap
	}
	return &p, nil
}

func (r pgReader) Marks(ctx context.Context, sessionID uuid.UUID) (map[uuid.UUID]MarkRow, error) {
	rows, err := r.tx.Query(ctx, `
		SELECT question_id, marks_half, max_half, comment, version, updated_at
		FROM answer_marks WHERE session_id = $1`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]MarkRow{}
	for rows.Next() {
		var m MarkRow
		if err := rows.Scan(&m.QuestionID, &m.MarksHalf, &m.MaxHalf, &m.Comment, &m.Version, &m.MarkedAt); err != nil {
			return nil, err
		}
		out[m.QuestionID] = m
	}
	return out, rows.Err()
}

func (r pgReader) Questions(ctx context.Context, examID, sessionID uuid.UUID) ([]QuestionInput, error) {
	records, err := review.LoadPaper(ctx, r.tx, examID, sessionID)
	if err != nil {
		return nil, err
	}
	return questionInputs(records), nil
}

// questionInputs maps the review's description of a paper onto what the rules
// need. "Answered" is the review's own definition.
func questionInputs(records []review.QuestionRecord) []QuestionInput {
	out := make([]QuestionInput, 0, len(records))
	for _, q := range records {
		in := QuestionInput{
			ID: q.ID, Number: q.QuestionNumber, Type: q.Type, Points: q.Points,
			Answered: review.IsAnswered(q.Answer),
		}
		if q.Answer != nil {
			in.SelectedOptionID = q.Answer.SelectedOptionID
		}
		for _, o := range q.Options {
			if o.IsCorrect {
				in.CorrectOptionIDs = append(in.CorrectOptionIDs, o.ID)
			}
		}
		out = append(out, in)
	}
	return out
}
