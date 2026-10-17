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
		SELECT $1, $2, $3, $4, $5, $6, $7
		FROM sessions s
		WHERE s.id = $2 AND s.status = 'IN_PROGRESS'
		ON CONFLICT (session_id, question_id)
		DO UPDATE SET
			selected_option_id = EXCLUDED.selected_option_id,
			text_answer = EXCLUDED.text_answer,
			updated_at = NOW()
		WHERE EXISTS (SELECT 1 FROM sessions s WHERE s.id = EXCLUDED.session_id AND s.status = 'IN_PROGRESS')
		RETURNING id, created_at, updated_at
	`
	now := time.Now().UTC()
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	a.CreatedAt = now
	a.UpdatedAt = now

	// The answer and its history are written together: either both exist or
	// neither does, so the current value always has a matching revision.
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// A confirmation that was already applied must change nothing. Checking
	// before the upsert matters: the upsert overwrites the answer, and a replay
	// that arrives after the student has since typed something else would
	// otherwise put the old value back with no revision to explain it.
	if replayed, err := loadReplayedConfirmation(ctx, tx, a); err != nil {
		return err
	} else if replayed {
		return nil // the deferred rollback leaves everything as it was
	}

	err = tx.QueryRow(ctx, query,
		a.ID,
		a.SessionID,
		a.QuestionID,
		a.SelectedOptionID,
		a.TextAnswer,
		a.CreatedAt,
		a.UpdatedAt,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionNotActive
		}
		return err
	}

	if err := recordRevision(ctx, tx, a); err != nil {
		if errors.Is(err, errConfirmationRaced) {
			// Another request applied this very confirmation between our check
			// and our write. Undo ours and report what is there.
			_ = tx.Rollback(ctx)
			return r.loadCurrent(ctx, a)
		}
		return err
	}
	return tx.Commit(ctx)
}

// errConfirmationRaced means a concurrent request applied the same confirmation.
var errConfirmationRaced = errors.New("answer: confirmation already applied")

// loadReplayedConfirmation reports whether a's confirmation was already applied
// to this answer while the session is still in progress. If so it fills a with
// the answer as it stands now. The answer row is locked, so two copies of one
// confirmation arriving together are serialized and the second sees the first.
func loadReplayedConfirmation(ctx context.Context, tx pgx.Tx, a *Answer) (bool, error) {
	if a.Revision == nil || a.Revision.ConfirmationID == "" {
		return false, nil
	}
	err := tx.QueryRow(ctx, `
		SELECT a.id, a.created_at, a.updated_at, a.selected_option_id, a.text_answer
		FROM answers a
		JOIN answer_revisions v ON v.answer_id = a.id
		JOIN sessions s ON s.id = a.session_id
		WHERE a.session_id = $1 AND a.question_id = $2 AND v.confirmation_id = $3
		  AND s.status = 'IN_PROGRESS'
		FOR UPDATE OF a`,
		a.SessionID, a.QuestionID, a.Revision.ConfirmationID,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt, &a.SelectedOptionID, &a.TextAnswer)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	a.Replayed = true
	return true, nil
}

// loadCurrent fills a with the stored answer, for a request that changed nothing.
func (r *pgRepository) loadCurrent(ctx context.Context, a *Answer) error {
	err := r.pool.QueryRow(ctx, `
		SELECT id, created_at, updated_at, selected_option_id, text_answer
		FROM answers WHERE session_id = $1 AND question_id = $2`, a.SessionID, a.QuestionID,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt, &a.SelectedOptionID, &a.TextAnswer)
	if err != nil {
		return err
	}
	a.Replayed = true
	return nil
}

// recordRevision appends the answer's new value to its history. The answer row
// is locked by the upsert above, so concurrent saves of one answer are
// serialized and revision numbers cannot collide.
func recordRevision(ctx context.Context, tx pgx.Tx, a *Answer) error {
	rev := a.Revision
	if rev == nil {
		rev = &RevisionInput{Source: SourceTyped}
		if a.SelectedOptionID != nil {
			rev.Source = SourceOption
		}
	}

	// Consecutive plain saves of the same kind (autosave while typing, or
	// changing the selected option with the mouse) update the latest revision
	// instead of adding one each time. A confirmed save never merges: it
	// carries a confirmation ID, is a deliberate act, and is neither merged into
	// an earlier revision nor allowed to be merged into later. created_at keeps
	// the time the revision began; answers.updated_at has the time of the last save.
	if (rev.Source == SourceTyped || rev.Source == SourceOption) && rev.ConfirmationID == "" {
		tag, err := tx.Exec(ctx, `
			UPDATE answer_revisions SET selected_option_id = $2, text_answer = $3
			WHERE id = (SELECT id FROM answer_revisions WHERE answer_id = $1 ORDER BY revision_no DESC LIMIT 1)
			  AND source = $4 AND confirmation_id IS NULL`, a.ID, a.SelectedOptionID, a.TextAnswer, rev.Source)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			return nil
		}
	}

	var confirmation *string
	if rev.ConfirmationID != "" {
		confirmation = &rev.ConfirmationID
	}
	var revisionID uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO answer_revisions
			(answer_id, revision_no, source, selected_option_id, text_answer, transcription_id, confirmation_id)
		SELECT $1, COALESCE(MAX(revision_no), 0) + 1, $2, $3, $4, $5, $6
		FROM answer_revisions WHERE answer_id = $1
		ON CONFLICT (answer_id, confirmation_id) WHERE confirmation_id IS NOT NULL DO NOTHING
		RETURNING id`,
		a.ID, rev.Source, a.SelectedOptionID, a.TextAnswer, rev.TranscriptionID, confirmation,
	).Scan(&revisionID)
	if errors.Is(err, pgx.ErrNoRows) {
		// The same confirmation was applied by a request that ran alongside this
		// one. The caller undoes this request's write.
		return errConfirmationRaced
	}
	if err != nil {
		return err
	}

	if len(rev.RecordingIDs) > 0 {
		// Only recordings of this very answer, and only ones not yet claimed.
		_, err = tx.Exec(ctx, `
			UPDATE audio_recordings SET answer_revision_id = $1
			WHERE id = ANY($2) AND session_id = $3 AND question_id = $4 AND answer_revision_id IS NULL
			  AND status = 'STORED'`,
			revisionID, rev.RecordingIDs, a.SessionID, a.QuestionID)
		if err != nil {
			return err
		}
	}
	return nil
}
