package grading

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Satyajeet-Das/ai-scribe/internal/audit"
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

// MarkWrite is what to store for one answer.
type MarkWrite struct {
	MarksHalf *int
	MaxHalf   int
	Comment   *string
	MarkedBy  uuid.UUID
	// ExpectedVersion is the version the answer's mark must still have for the
	// write to happen: 0 when nothing has been saved. The check is part of the
	// write itself, so it holds even if two writers get this far at once.
	ExpectedVersion int
}

// Writer is a read-write transaction in which one paper is locked. It also reads,
// and sees its own writes.
type Writer interface {
	Reader
	// LockPaper creates the paper's grading record if there is none and locks it
	// until the transaction ends. Every change to a paper takes this lock first,
	// so changes to one paper happen one after another.
	LockPaper(ctx context.Context, examID, sessionID uuid.UUID) (*PaperRow, error)
	// Question returns the question's current points, or nil if the exam has no
	// such question.
	Question(ctx context.Context, examID, questionID uuid.UUID) (*QuestionPoints, error)
	// Mark returns what was saved for one answer, or nil.
	Mark(ctx context.Context, sessionID, questionID uuid.UUID) (*MarkRow, error)
	// UpsertMark stores a mark and raises its version by one. It returns
	// ErrStaleVersion, and writes nothing, if the stored version is not
	// w.ExpectedVersion.
	UpsertMark(ctx context.Context, sessionID, questionID uuid.UUID, w MarkWrite) (MarkRow, error)
	// BumpPaper raises the paper's version by one and returns it.
	BumpPaper(ctx context.Context, sessionID uuid.UUID) (int, error)
	// Audit records an event in this transaction. If it fails, the transaction
	// must not commit, and the service returns the error.
	Audit(ctx context.Context, e audit.Event) error
	// SetFinal marks the locked paper final with its frozen marks, and raises its
	// version. It returns the new version.
	SetFinal(ctx context.Context, sessionID, by uuid.UUID, snap Snapshot, totalHalf, maxHalf int) (int, error)
	// SetDraft takes the locked paper back to a draft, clears what finalising
	// stored, and raises its version. It returns the new version.
	SetDraft(ctx context.Context, sessionID uuid.UUID) (int, error)
	// AddEvent appends to the paper's finalise/reopen history.
	AddEvent(ctx context.Context, e GradingEvent) error
}

// Kinds of GradingEvent.
const (
	EventFinalised = "FINALISED"
	EventReopened  = "REOPENED"
)

// GradingEvent is one finalise or reopen, kept for good.
type GradingEvent struct {
	SessionID uuid.UUID
	ExamID    uuid.UUID
	Event     string
	ActorID   uuid.UUID
	ActorRole string
	// Reason is required for a reopen.
	Reason    *string
	TotalHalf *int
	MaxHalf   *int
	Snapshot  *Snapshot
}

// QuestionPoints is a question's current worth.
type QuestionPoints struct {
	ID     uuid.UUID
	Points int
}

// Repository gives access to marking data.
type Repository interface {
	// Read runs fn against one consistent snapshot. Nothing can be written in it.
	Read(ctx context.Context, fn func(Reader) error) error
	// Write runs fn in a transaction. It commits if fn returns nil and rolls back
	// otherwise, taking every change and audit event with it.
	Write(ctx context.Context, fn func(Writer) error) error
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

func (r *pgRepository) Write(ctx context.Context, fn func(Writer) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(pgWriter{pgReader: pgReader{tx: tx}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type pgWriter struct {
	pgReader
}

func (w pgWriter) LockPaper(ctx context.Context, examID, sessionID uuid.UUID) (*PaperRow, error) {
	if _, err := w.tx.Exec(ctx, `
		INSERT INTO paper_gradings (session_id, exam_id) VALUES ($1, $2)
		ON CONFLICT (session_id) DO NOTHING`, sessionID, examID); err != nil {
		return nil, err
	}
	var p PaperRow
	var status string
	var snapshot []byte
	err := w.tx.QueryRow(ctx, `
		SELECT session_id, status, version, finalised_by, finalised_at, total_half, max_half, snapshot
		FROM paper_gradings WHERE session_id = $1 FOR UPDATE`, sessionID,
	).Scan(&p.SessionID, &status, &p.Version, &p.FinalisedBy, &p.FinalisedAt, &p.TotalHalf, &p.MaxHalf, &snapshot)
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

func (w pgWriter) Question(ctx context.Context, examID, questionID uuid.UUID) (*QuestionPoints, error) {
	var q QuestionPoints
	err := w.tx.QueryRow(ctx, `SELECT id, points FROM questions WHERE id = $1 AND exam_id = $2`,
		questionID, examID).Scan(&q.ID, &q.Points)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &q, nil
}

func (w pgWriter) Mark(ctx context.Context, sessionID, questionID uuid.UUID) (*MarkRow, error) {
	var m MarkRow
	err := w.tx.QueryRow(ctx, `
		SELECT question_id, marks_half, max_half, comment, version, updated_at
		FROM answer_marks WHERE session_id = $1 AND question_id = $2`, sessionID, questionID,
	).Scan(&m.QuestionID, &m.MarksHalf, &m.MaxHalf, &m.Comment, &m.Version, &m.MarkedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (w pgWriter) UpsertMark(ctx context.Context, sessionID, questionID uuid.UUID, in MarkWrite) (MarkRow, error) {
	var m MarkRow
	err := w.tx.QueryRow(ctx, `
		INSERT INTO answer_marks (session_id, question_id, marks_half, max_half, comment, version, marked_by)
		VALUES ($1, $2, $3, $4, $5, 1, $6)
		ON CONFLICT (session_id, question_id) DO UPDATE SET
			marks_half = EXCLUDED.marks_half,
			max_half = EXCLUDED.max_half,
			comment = EXCLUDED.comment,
			version = answer_marks.version + 1,
			marked_by = EXCLUDED.marked_by,
			updated_at = NOW()
		WHERE answer_marks.version = $7
		RETURNING question_id, marks_half, max_half, comment, version, updated_at`,
		sessionID, questionID, in.MarksHalf, in.MaxHalf, in.Comment, in.MarkedBy, in.ExpectedVersion,
	).Scan(&m.QuestionID, &m.MarksHalf, &m.MaxHalf, &m.Comment, &m.Version, &m.MarkedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// A row exists and its version is not the one expected.
		return MarkRow{}, ErrStaleVersion
	}
	return m, err
}

func (w pgWriter) BumpPaper(ctx context.Context, sessionID uuid.UUID) (int, error) {
	var v int
	err := w.tx.QueryRow(ctx, `
		UPDATE paper_gradings SET version = version + 1, updated_at = NOW()
		WHERE session_id = $1 RETURNING version`, sessionID).Scan(&v)
	return v, err
}

func (w pgWriter) SetFinal(ctx context.Context, sessionID, by uuid.UUID, snap Snapshot, totalHalf, maxHalf int) (int, error) {
	raw, err := json.Marshal(snap)
	if err != nil {
		return 0, err
	}
	var v int
	err = w.tx.QueryRow(ctx, `
		UPDATE paper_gradings SET status = 'FINAL', finalised_by = $2, finalised_at = NOW(),
			total_half = $3, max_half = $4, snapshot = $5, version = version + 1, updated_at = NOW()
		WHERE session_id = $1 RETURNING version`,
		sessionID, by, totalHalf, maxHalf, raw).Scan(&v)
	return v, err
}

func (w pgWriter) SetDraft(ctx context.Context, sessionID uuid.UUID) (int, error) {
	var v int
	err := w.tx.QueryRow(ctx, `
		UPDATE paper_gradings SET status = 'DRAFT', finalised_by = NULL, finalised_at = NULL,
			total_half = NULL, max_half = NULL, snapshot = NULL, version = version + 1, updated_at = NOW()
		WHERE session_id = $1 RETURNING version`, sessionID).Scan(&v)
	return v, err
}

func (w pgWriter) AddEvent(ctx context.Context, e GradingEvent) error {
	var raw []byte
	if e.Snapshot != nil {
		b, err := json.Marshal(e.Snapshot)
		if err != nil {
			return err
		}
		raw = b
	}
	_, err := w.tx.Exec(ctx, `
		INSERT INTO paper_grading_events (session_id, exam_id, event, actor_id, actor_role, reason, total_half, max_half, snapshot)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		e.SessionID, e.ExamID, e.Event, e.ActorID, e.ActorRole, e.Reason, e.TotalHalf, e.MaxHalf, raw)
	return err
}

func (w pgWriter) Audit(ctx context.Context, e audit.Event) error {
	return audit.RecordTx(ctx, w.tx, e)
}
