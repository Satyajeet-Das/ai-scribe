package recording

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists recording metadata.
type Repository interface {
	// Insert stores the recording and its transcription in one transaction. If
	// a recording already exists for the same (session, stream), nothing is
	// written and that recording is returned with created=false. It refuses
	// with ErrSessionEnded unless the session is still in progress.
	Insert(ctx context.Context, rec *Recording, tr *Transcription) (existing *Recording, existingTranscription uuid.UUID, created bool, err error)
}

type pgRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

func (r *pgRepository) Insert(ctx context.Context, rec *Recording, tr *Transcription) (*Recording, uuid.UUID, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, uuid.Nil, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// FOR SHARE waits for a submit or expiry that is updating the session, so
	// a recording can never be written after the session it belongs to ended.
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM sessions WHERE id = $1 FOR SHARE`, rec.SessionID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && status != "IN_PROGRESS") {
		return nil, uuid.Nil, false, ErrSessionEnded
	}
	if err != nil {
		return nil, uuid.Nil, false, err
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO audio_recordings
			(id, session_id, question_id, student_id, stream_id, segment_index, content_type,
			 sample_rate, duration_ms, byte_size, chunk_count, sha256, storage_key, recorded_at, retention_until)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (session_id, stream_id) DO NOTHING
		RETURNING id`,
		rec.ID, rec.SessionID, rec.QuestionID, rec.StudentID, rec.StreamID, rec.SegmentIndex, rec.ContentType,
		rec.SampleRate, rec.DurationMillis, rec.ByteSize, rec.ChunkCount, rec.SHA256, rec.StorageKey,
		rec.RecordedAt, rec.RetentionUntil,
	).Scan(&rec.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Already saved for this stream: report what is there.
		var ex Recording
		var trID uuid.UUID
		qerr := tx.QueryRow(ctx, `
			SELECT r.id, r.session_id, r.question_id, r.student_id, r.stream_id, r.segment_index, r.content_type,
			       r.sample_rate, r.duration_ms, r.byte_size, r.chunk_count, r.sha256, r.storage_key,
			       r.recorded_at, r.retention_until, COALESCE(t.id, '00000000-0000-0000-0000-000000000000')
			FROM audio_recordings r
			LEFT JOIN transcriptions t ON t.recording_id = r.id
			WHERE r.session_id = $1 AND r.stream_id = $2`, rec.SessionID, rec.StreamID,
		).Scan(&ex.ID, &ex.SessionID, &ex.QuestionID, &ex.StudentID, &ex.StreamID, &ex.SegmentIndex, &ex.ContentType,
			&ex.SampleRate, &ex.DurationMillis, &ex.ByteSize, &ex.ChunkCount, &ex.SHA256, &ex.StorageKey,
			&ex.RecordedAt, &ex.RetentionUntil, &trID)
		if qerr != nil {
			return nil, uuid.Nil, false, qerr
		}
		return &ex, trID, false, nil
	}
	if err != nil {
		return nil, uuid.Nil, false, err
	}

	tr.RecordingID = rec.ID
	if err := tx.QueryRow(ctx, `
		INSERT INTO transcriptions (id, recording_id, text, provider, language, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id`,
		tr.ID, tr.RecordingID, tr.Text, tr.Provider, tr.Language, tr.CreatedAt,
	).Scan(&tr.ID); err != nil {
		return nil, uuid.Nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, uuid.Nil, false, err
	}
	return nil, uuid.Nil, true, nil
}
