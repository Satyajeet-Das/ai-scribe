package document

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
)

// ClaimResult says what happened when a worker tried to take a document.
type ClaimResult int

const (
	// Claimed: the worker now owns the document.
	Claimed ClaimResult = iota
	// ClaimStale: there is nothing for this job to do. The document is gone,
	// finished, cancelled, or has moved on to a newer attempt.
	ClaimStale
	// ClaimLeaseHeld: another worker owns this attempt and its lease has not run
	// out. The job should be tried again later.
	ClaimLeaseHeld
)

// ExamState is the part of an exam that approval depends on, read under a lock.
type ExamState struct {
	Status    exam.Status
	CreatedBy uuid.UUID
	Deleted   bool
}

// Repository stores documents and their drafts.
//
// Every change a worker makes names the attempt it was queued for and only
// applies while the document is still PROCESSING under that attempt. A worker
// that was cancelled, restarted or replaced therefore cannot write: its
// updates match nothing and report false.
type Repository interface {
	Insert(ctx context.Context, d *Document, tx ...database.DBTX) error
	Get(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (*Document, error)
	// GetForUpdate reads the document and locks its row until the transaction ends.
	GetForUpdate(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (*Document, error)
	List(ctx context.Context, examID uuid.UUID) ([]Document, error)
	// CountLive counts the exam's documents that are not failed or cancelled.
	CountLive(ctx context.Context, examID uuid.UUID) (int, error)
	Delete(ctx context.Context, id uuid.UUID, tx ...database.DBTX) error

	Claim(ctx context.Context, id uuid.UUID, attempt int, lease time.Duration) (ClaimResult, *Document, error)
	// Heartbeat renews the lease. It reports false once the document is no longer
	// this attempt's to work on.
	Heartbeat(ctx context.Context, id uuid.UUID, attempt int, lease time.Duration) (bool, error)
	SetProgress(ctx context.Context, id uuid.UUID, attempt int, stage Stage, done, total int) (bool, error)
	// SaveDraft replaces the document's draft with data and moves it to
	// REVIEW_REQUIRED, all or nothing.
	SaveDraft(ctx context.Context, id uuid.UUID, attempt int, data DraftData) (bool, error)
	Fail(ctx context.Context, id uuid.UUID, attempt int, code, message string) (bool, error)
	// Requeue puts a PROCESSING document back to QUEUED after a transient failure.
	Requeue(ctx context.Context, id uuid.UUID, attempt int) (bool, error)
	// Cancel moves a QUEUED or PROCESSING document to CANCELLED.
	Cancel(ctx context.Context, id uuid.UUID) (bool, error)
	// Restart queues the document again as a new attempt and discards its draft.
	// The caller holds the row lock and has checked the status. It returns the
	// new attempt number.
	Restart(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (int, error)

	ListDraft(ctx context.Context, documentID uuid.UUID, tx ...database.DBTX) ([]DraftQuestion, error)
	GetDraftQuestion(ctx context.Context, documentID, questionID uuid.UUID, tx ...database.DBTX) (*DraftQuestion, error)
	InsertDraftQuestion(ctx context.Context, q *DraftQuestion, tx ...database.DBTX) error
	// ShiftPositions makes room for an insert by moving questions at or after
	// from one place down.
	ShiftPositions(ctx context.Context, documentID uuid.UUID, from int, tx ...database.DBTX) error
	UpdateDraftQuestion(ctx context.Context, q *DraftQuestion, tx ...database.DBTX) error
	DeleteDraftQuestion(ctx context.Context, documentID, questionID uuid.UUID, tx ...database.DBTX) error
	// SetOrder gives the listed questions positions 1..n in the order given.
	SetOrder(ctx context.Context, documentID uuid.UUID, ids []uuid.UUID, tx ...database.DBTX) error
	// BumpDraftVersion raises the version by one and returns the new value.
	BumpDraftVersion(ctx context.Context, documentID uuid.UUID, tx ...database.DBTX) (int, error)
	MarkApproved(ctx context.Context, documentID, by uuid.UUID, tx ...database.DBTX) error

	// LockExam reads the exam and locks its row until the transaction ends, so a
	// publish cannot happen between the checks and the import.
	LockExam(ctx context.Context, examID uuid.UUID, tx ...database.DBTX) (*ExamState, error)
}

type pgRepository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

func (r *pgRepository) db(tx ...database.DBTX) database.DBTX {
	if len(tx) > 0 && tx[0] != nil {
		return tx[0]
	}
	return r.pool
}

const docCols = `id, exam_id, uploaded_by, original_filename, storage_key, size_bytes, sha256, page_count,
	status, COALESCE(stage, ''), progress_done, progress_total, attempt, lease_expires_at,
	provider, prompt_version, COALESCE(failure_code, ''), COALESCE(failure_message, ''),
	page_summary, document_instructions, unassigned_content, draft_version, approved_by, approved_at,
	created_at, updated_at`

func scanDoc(row pgx.Row) (*Document, error) {
	var d Document
	var summary, unassigned []byte
	err := row.Scan(&d.ID, &d.ExamID, &d.UploadedBy, &d.OriginalFilename, &d.StorageKey, &d.SizeBytes, &d.SHA256, &d.PageCount,
		&d.Status, &d.Stage, &d.ProgressDone, &d.ProgressTotal, &d.Attempt, &d.LeaseExpires,
		&d.Provider, &d.PromptVersion, &d.FailureCode, &d.FailureMessage,
		&summary, &d.DocumentInstructions, &unassigned, &d.DraftVersion, &d.ApprovedBy, &d.ApprovedAt,
		&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal(summary, &d.PageSummary); err != nil {
		return nil, fmt.Errorf("reading page summary: %w", err)
	}
	if err := json.Unmarshal(unassigned, &d.Unassigned); err != nil {
		return nil, fmt.Errorf("reading unassigned content: %w", err)
	}
	return &d, nil
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

// duplicateOf finds the live document that stops this file being uploaded again.
// It uses the pool, not a transaction: the failed statement has aborted that.
func (r *pgRepository) duplicateOf(ctx context.Context, examID uuid.UUID, sha string) error {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT id FROM exam_documents WHERE exam_id = $1 AND sha256 = $2 AND status NOT IN ('FAILED', 'CANCELLED') LIMIT 1`,
		examID, sha).Scan(&id)
	if err != nil {
		return ErrConcurrentChange
	}
	return &DuplicateError{ExistingID: id}
}

func (r *pgRepository) Insert(ctx context.Context, d *Document, tx ...database.DBTX) error {
	summary, _ := json.Marshal(nonNilPages(d.PageSummary))
	_, err := r.db(tx...).Exec(ctx, `
		INSERT INTO exam_documents (id, exam_id, uploaded_by, original_filename, storage_key, size_bytes, sha256, page_count,
			status, attempt, provider, prompt_version, page_summary, progress_total)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		d.ID, d.ExamID, d.UploadedBy, d.OriginalFilename, d.StorageKey, d.SizeBytes, d.SHA256, d.PageCount,
		d.Status, d.Attempt, d.Provider, d.PromptVersion, summary, d.ProgressTotal)
	if err != nil {
		if isUniqueViolation(err) {
			return r.duplicateOf(ctx, d.ExamID, d.SHA256)
		}
		return err
	}
	return nil
}

func (r *pgRepository) Get(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (*Document, error) {
	return scanDoc(r.db(tx...).QueryRow(ctx, `SELECT `+docCols+` FROM exam_documents WHERE id = $1`, id))
}

func (r *pgRepository) GetForUpdate(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (*Document, error) {
	return scanDoc(r.db(tx...).QueryRow(ctx, `SELECT `+docCols+` FROM exam_documents WHERE id = $1 FOR UPDATE`, id))
}

func (r *pgRepository) List(ctx context.Context, examID uuid.UUID) ([]Document, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+docCols+` FROM exam_documents WHERE exam_id = $1 ORDER BY created_at DESC`, examID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func (r *pgRepository) CountLive(ctx context.Context, examID uuid.UUID) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM exam_documents WHERE exam_id = $1 AND status NOT IN ('FAILED', 'CANCELLED')`, examID).Scan(&n)
	return n, err
}

func (r *pgRepository) Delete(ctx context.Context, id uuid.UUID, tx ...database.DBTX) error {
	_, err := r.db(tx...).Exec(ctx, `DELETE FROM exam_documents WHERE id = $1`, id)
	return err
}

func secs(d time.Duration) float64 { return d.Seconds() }

func (r *pgRepository) Claim(ctx context.Context, id uuid.UUID, attempt int, lease time.Duration) (ClaimResult, *Document, error) {
	d, err := scanDoc(r.pool.QueryRow(ctx, `
		UPDATE exam_documents
		SET status = 'PROCESSING', stage = 'READING_PAGES', progress_done = 0, progress_total = page_count,
		    lease_expires_at = NOW() + make_interval(secs => $3), updated_at = NOW()
		WHERE id = $1 AND attempt = $2
		  AND (status = 'QUEUED' OR (status = 'PROCESSING' AND lease_expires_at < NOW()))
		RETURNING `+docCols, id, attempt, secs(lease)))
	if err == nil {
		return Claimed, d, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return ClaimStale, nil, err
	}
	cur, err := r.Get(ctx, id)
	switch {
	case errors.Is(err, ErrNotFound):
		return ClaimStale, nil, nil
	case err != nil:
		return ClaimStale, nil, err
	case cur.Attempt == attempt && cur.Status == StatusProcessing:
		return ClaimLeaseHeld, cur, nil
	}
	return ClaimStale, cur, nil
}

func (r *pgRepository) Heartbeat(ctx context.Context, id uuid.UUID, attempt int, lease time.Duration) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE exam_documents SET lease_expires_at = NOW() + make_interval(secs => $3), updated_at = NOW()
		WHERE id = $1 AND attempt = $2 AND status = 'PROCESSING'`, id, attempt, secs(lease))
	return tag.RowsAffected() == 1, err
}

func (r *pgRepository) SetProgress(ctx context.Context, id uuid.UUID, attempt int, stage Stage, done, total int) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE exam_documents SET stage = $3, progress_done = $4, progress_total = $5, updated_at = NOW()
		WHERE id = $1 AND attempt = $2 AND status = 'PROCESSING'`, id, attempt, string(stage), done, total)
	return tag.RowsAffected() == 1, err
}

func (r *pgRepository) SaveDraft(ctx context.Context, id uuid.UUID, attempt int, data DraftData) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the row first: the guard and the writes then see one consistent state.
	var status string
	var cur int
	err = tx.QueryRow(ctx, `SELECT status, attempt FROM exam_documents WHERE id = $1 FOR UPDATE`, id).Scan(&status, &cur)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if status != string(StatusProcessing) || cur != attempt {
		return false, nil
	}

	if _, err := tx.Exec(ctx, `DELETE FROM exam_document_questions WHERE document_id = $1`, id); err != nil {
		return false, err
	}
	for i := range data.Questions {
		q := data.Questions[i]
		q.DocumentID = id
		q.Position = i + 1
		if q.ID == uuid.Nil {
			q.ID = uuid.New()
		}
		if err := r.InsertDraftQuestion(ctx, &q, tx); err != nil {
			return false, err
		}
	}
	summary, _ := json.Marshal(nonNilPages(data.PageSummary))
	unassigned, _ := json.Marshal(nonNilUnassigned(data.Unassigned))
	if _, err := tx.Exec(ctx, `
		UPDATE exam_documents
		SET status = 'REVIEW_REQUIRED', stage = NULL, lease_expires_at = NULL, progress_done = progress_total,
		    provider = $2, prompt_version = $3, page_summary = $4, document_instructions = $5, unassigned_content = $6,
		    draft_version = draft_version + 1, failure_code = NULL, failure_message = NULL, updated_at = NOW()
		WHERE id = $1`, id, data.Provider, data.PromptVersion, summary, data.DocumentInstructions, unassigned); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (r *pgRepository) Fail(ctx context.Context, id uuid.UUID, attempt int, code, message string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE exam_documents
		SET status = 'FAILED', stage = NULL, lease_expires_at = NULL, failure_code = $3, failure_message = $4, updated_at = NOW()
		WHERE id = $1 AND attempt = $2 AND status IN ('QUEUED', 'PROCESSING')`, id, attempt, code, message)
	return tag.RowsAffected() == 1, err
}

func (r *pgRepository) Requeue(ctx context.Context, id uuid.UUID, attempt int) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE exam_documents SET status = 'QUEUED', stage = NULL, lease_expires_at = NULL, updated_at = NOW()
		WHERE id = $1 AND attempt = $2 AND status = 'PROCESSING'`, id, attempt)
	return tag.RowsAffected() == 1, err
}

func (r *pgRepository) Cancel(ctx context.Context, id uuid.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE exam_documents SET status = 'CANCELLED', stage = NULL, lease_expires_at = NULL, updated_at = NOW()
		WHERE id = $1 AND status IN ('QUEUED', 'PROCESSING')`, id)
	return tag.RowsAffected() == 1, err
}

func (r *pgRepository) Restart(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (int, error) {
	db := r.db(tx...)
	if _, err := db.Exec(ctx, `DELETE FROM exam_document_questions WHERE document_id = $1`, id); err != nil {
		return 0, err
	}
	var attempt int
	err := db.QueryRow(ctx, `
		UPDATE exam_documents
		SET status = 'QUEUED', stage = NULL, lease_expires_at = NULL, progress_done = 0, progress_total = page_count,
		    attempt = attempt + 1, failure_code = NULL, failure_message = NULL,
		    page_summary = '[]'::jsonb, document_instructions = '', unassigned_content = '[]'::jsonb,
		    draft_version = draft_version + 1, updated_at = NOW()
		WHERE id = $1 RETURNING attempt`, id).Scan(&attempt)
	if err != nil {
		if isUniqueViolation(err) {
			// Another live upload of the same file now exists.
			var examID uuid.UUID
			var sha string
			if qerr := r.pool.QueryRow(ctx, `SELECT exam_id, sha256 FROM exam_documents WHERE id = $1`, id).Scan(&examID, &sha); qerr == nil {
				return 0, r.duplicateOf(ctx, examID, sha)
			}
		}
		return 0, err
	}
	return attempt, nil
}

const draftCols = `id, document_id, position, number_label, type, instructions, text, points, options, tables, diagrams,
	source_pages, confidence, warnings, origin, edited, created_at, updated_at`

func scanDraft(row pgx.Row) (*DraftQuestion, error) {
	var q DraftQuestion
	var options, tables, diagrams, warnings []byte
	var pages []int32
	err := row.Scan(&q.ID, &q.DocumentID, &q.Position, &q.NumberLabel, &q.Type, &q.Instructions, &q.Text, &q.Points,
		&options, &tables, &diagrams, &pages, &q.Confidence, &warnings, &q.Origin, &q.Edited, &q.CreatedAt, &q.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	for _, f := range []struct {
		raw []byte
		dst any
	}{{options, &q.Options}, {tables, &q.Tables}, {diagrams, &q.Diagrams}, {warnings, &q.Warnings}} {
		if err := json.Unmarshal(f.raw, f.dst); err != nil {
			return nil, fmt.Errorf("reading draft question: %w", err)
		}
	}
	for _, p := range pages {
		q.SourcePages = append(q.SourcePages, int(p))
	}
	return &q, nil
}

func (r *pgRepository) ListDraft(ctx context.Context, documentID uuid.UUID, tx ...database.DBTX) ([]DraftQuestion, error) {
	rows, err := r.db(tx...).Query(ctx, `SELECT `+draftCols+` FROM exam_document_questions WHERE document_id = $1 ORDER BY position`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DraftQuestion{}
	for rows.Next() {
		q, err := scanDraft(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *q)
	}
	return out, rows.Err()
}

func (r *pgRepository) GetDraftQuestion(ctx context.Context, documentID, questionID uuid.UUID, tx ...database.DBTX) (*DraftQuestion, error) {
	return scanDraft(r.db(tx...).QueryRow(ctx, `SELECT `+draftCols+` FROM exam_document_questions WHERE document_id = $1 AND id = $2`, documentID, questionID))
}

func (r *pgRepository) InsertDraftQuestion(ctx context.Context, q *DraftQuestion, tx ...database.DBTX) error {
	opts, tabs, dgs, warns := marshalDraftParts(q)
	pages := make([]int32, len(q.SourcePages))
	for i, p := range q.SourcePages {
		pages[i] = int32(p)
	}
	_, err := r.db(tx...).Exec(ctx, `
		INSERT INTO exam_document_questions (id, document_id, position, number_label, type, instructions, text, points,
			options, tables, diagrams, source_pages, confidence, warnings, origin, edited)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		q.ID, q.DocumentID, q.Position, q.NumberLabel, q.Type, q.Instructions, q.Text, q.Points,
		opts, tabs, dgs, pages, q.Confidence, warns, q.Origin, q.Edited)
	return err
}

func marshalDraftParts(q *DraftQuestion) (opts, tabs, dgs, warns []byte) {
	if q.Options == nil {
		q.Options = []extract.Option{}
	}
	if q.Tables == nil {
		q.Tables = []extract.Table{}
	}
	if q.Diagrams == nil {
		q.Diagrams = []extract.Diagram{}
	}
	if q.Warnings == nil {
		q.Warnings = []extract.Warning{}
	}
	opts, _ = json.Marshal(q.Options)
	tabs, _ = json.Marshal(q.Tables)
	dgs, _ = json.Marshal(q.Diagrams)
	warns, _ = json.Marshal(q.Warnings)
	return
}

func (r *pgRepository) ShiftPositions(ctx context.Context, documentID uuid.UUID, from int, tx ...database.DBTX) error {
	// One statement: the unique position is deferrable, so it is checked when the
	// statement ends and not row by row.
	_, err := r.db(tx...).Exec(ctx, `UPDATE exam_document_questions SET position = position + 1 WHERE document_id = $1 AND position >= $2`, documentID, from)
	return err
}

func (r *pgRepository) UpdateDraftQuestion(ctx context.Context, q *DraftQuestion, tx ...database.DBTX) error {
	opts, tabs, dgs, warns := marshalDraftParts(q)
	tag, err := r.db(tx...).Exec(ctx, `
		UPDATE exam_document_questions
		SET number_label = $3, type = $4, instructions = $5, text = $6, points = $7,
		    options = $8, tables = $9, diagrams = $10, warnings = $11, edited = $12, updated_at = NOW()
		WHERE document_id = $1 AND id = $2`,
		q.DocumentID, q.ID, q.NumberLabel, q.Type, q.Instructions, q.Text, q.Points, opts, tabs, dgs, warns, q.Edited)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgRepository) DeleteDraftQuestion(ctx context.Context, documentID, questionID uuid.UUID, tx ...database.DBTX) error {
	db := r.db(tx...)
	tag, err := db.Exec(ctx, `DELETE FROM exam_document_questions WHERE document_id = $1 AND id = $2`, documentID, questionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	// Close the gap so positions stay 1..n.
	_, err = db.Exec(ctx, `
		UPDATE exam_document_questions q SET position = r.n
		FROM (SELECT id, ROW_NUMBER() OVER (ORDER BY position) AS n FROM exam_document_questions WHERE document_id = $1) r
		WHERE q.id = r.id AND q.position <> r.n`, documentID)
	return err
}

func (r *pgRepository) SetOrder(ctx context.Context, documentID uuid.UUID, ids []uuid.UUID, tx ...database.DBTX) error {
	_, err := r.db(tx...).Exec(ctx, `
		UPDATE exam_document_questions q SET position = o.pos, updated_at = NOW()
		FROM (SELECT id, ord AS pos FROM unnest($2::uuid[]) WITH ORDINALITY AS t(id, ord)) o
		WHERE q.document_id = $1 AND q.id = o.id`, documentID, ids)
	return err
}

func (r *pgRepository) BumpDraftVersion(ctx context.Context, documentID uuid.UUID, tx ...database.DBTX) (int, error) {
	var v int
	err := r.db(tx...).QueryRow(ctx, `UPDATE exam_documents SET draft_version = draft_version + 1, updated_at = NOW() WHERE id = $1 RETURNING draft_version`, documentID).Scan(&v)
	return v, err
}

func (r *pgRepository) MarkApproved(ctx context.Context, documentID, by uuid.UUID, tx ...database.DBTX) error {
	_, err := r.db(tx...).Exec(ctx, `
		UPDATE exam_documents SET status = 'COMPLETED', approved_by = $2, approved_at = NOW(),
		    draft_version = draft_version + 1, updated_at = NOW()
		WHERE id = $1`, documentID, by)
	return err
}

func (r *pgRepository) LockExam(ctx context.Context, examID uuid.UUID, tx ...database.DBTX) (*ExamState, error) {
	var s ExamState
	var deleted *time.Time
	err := r.db(tx...).QueryRow(ctx, `SELECT status, created_by, deleted_at FROM exams WHERE id = $1 FOR UPDATE`, examID).Scan(&s.Status, &s.CreatedBy, &deleted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	s.Deleted = deleted != nil
	return &s, nil
}

func nonNilPages(p []PageInfo) []PageInfo {
	if p == nil {
		return []PageInfo{}
	}
	return p
}

func nonNilUnassigned(u []extract.Unassigned) []extract.Unassigned {
	if u == nil {
		return []extract.Unassigned{}
	}
	return u
}
