package recording

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Expired is a stored recording whose retention period has ended.
type Expired struct {
	ID         uuid.UUID
	SessionID  uuid.UUID
	StorageKey string
}

// JanitorRepository is what housekeeping needs from the database.
type JanitorRepository interface {
	// TryLock takes the cluster-wide cleanup lock. When another instance holds
	// it, ok is false and nothing is done. release must be called when done.
	TryLock(ctx context.Context) (release func(), ok bool, err error)
	// Expired returns up to limit stored recordings whose retention date has
	// passed, oldest first. Recordings with no retention date are never returned.
	Expired(ctx context.Context, before time.Time, limit int) ([]Expired, error)
	// Unconfirmed returns up to limit stored recordings that were made at or
	// before the given time and never became part of a confirmed answer: the
	// student denied the transcript, dictated again, or walked away.
	Unconfirmed(ctx context.Context, madeBefore time.Time, limit int) ([]Expired, error)
	// MarkDeleted records that a recording's audio is gone. It reports whether
	// this call changed the row, so a repeat is harmless.
	MarkDeleted(ctx context.Context, id uuid.UUID, at time.Time) (bool, error)
	// StoredKeys returns the storage keys of every recording whose audio should
	// exist. A deleted recording's key is not included: its audio must not.
	StoredKeys(ctx context.Context) (map[string]uuid.UUID, error)
}

// cleanupLockKey identifies the advisory lock ("recording janitor" as a number).
const cleanupLockKey int64 = 0x7265636a616e

type pgJanitorRepository struct {
	pool *pgxpool.Pool
}

func NewJanitorRepository(pool *pgxpool.Pool) JanitorRepository {
	return &pgJanitorRepository{pool: pool}
}

func (r *pgJanitorRepository) TryLock(ctx context.Context) (func(), bool, error) {
	// An advisory lock belongs to one connection, so hold one for the whole run.
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	var ok bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, cleanupLockKey).Scan(&ok); err != nil {
		conn.Release()
		return nil, false, err
	}
	if !ok {
		conn.Release()
		return nil, false, nil
	}
	release := func() {
		// The caller's context may be cancelled by now; unlocking must not be.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, cleanupLockKey)
		conn.Release()
	}
	return release, true, nil
}

func (r *pgJanitorRepository) Expired(ctx context.Context, before time.Time, limit int) ([]Expired, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, session_id, storage_key
		FROM audio_recordings
		WHERE status = 'STORED' AND retention_until IS NOT NULL AND retention_until <= $1
		ORDER BY retention_until, id
		LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Expired
	for rows.Next() {
		var e Expired
		if err := rows.Scan(&e.ID, &e.SessionID, &e.StorageKey); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *pgJanitorRepository) Unconfirmed(ctx context.Context, madeBefore time.Time, limit int) ([]Expired, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, session_id, storage_key
		FROM audio_recordings
		WHERE status = 'STORED' AND answer_revision_id IS NULL AND recorded_at <= $1
		ORDER BY recorded_at, id
		LIMIT $2`, madeBefore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Expired
	for rows.Next() {
		var e Expired
		if err := rows.Scan(&e.ID, &e.SessionID, &e.StorageKey); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *pgJanitorRepository) MarkDeleted(ctx context.Context, id uuid.UUID, at time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE audio_recordings SET status = 'DELETED', deleted_at = $2
		WHERE id = $1 AND status = 'STORED'`, id, at)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *pgJanitorRepository) StoredKeys(ctx context.Context) (map[string]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `SELECT storage_key, id FROM audio_recordings WHERE status = 'STORED'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := map[string]uuid.UUID{}
	for rows.Next() {
		var key string
		var id uuid.UUID
		if err := rows.Scan(&key, &id); err != nil {
			return nil, err
		}
		keys[key] = id
	}
	return keys, rows.Err()
}
