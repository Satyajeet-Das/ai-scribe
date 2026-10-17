package review

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

// Repository reads review data. It has no write methods by design.
type Repository interface {
	ListStudents(ctx context.Context, p ListParams) (*StudentPage, error)
	// GetResponses returns the student's responses for the exam. Questions and
	// answers are read only when the student has a session and, for a session
	// still in progress, allowInProgress is true.
	GetResponses(ctx context.Context, examID, studentID uuid.UUID, allowInProgress bool) (*Responses, error)
	// GetRecording finds a recording only if it belongs to the given student's
	// session in the given exam. A recording of any other exam or student is
	// not found, whatever its ID.
	GetRecording(ctx context.Context, examID, studentID, recordingID uuid.UUID) (*RecordingFile, error)
}

type pgRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{pool: pool}
}

// readTx runs fn in a read-only, repeatable-read transaction so that counts,
// rows and answers come from one snapshot and nothing can be written.
func (r *pgRepository) readTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// answeredExpr is the SQL form of IsAnswered.
const answeredExpr = `(an.selected_option_id IS NOT NULL OR BTRIM(an.text_answer) <> '')`

// sessionLateral picks the session that represents an assignment: a submitted
// one if any, otherwise the most recent.
const sessionLateral = `
	LEFT JOIN LATERAL (
		SELECT id, status, started_at, submitted_at
		FROM sessions
		WHERE assignment_id = a.id
		ORDER BY (status = 'SUBMITTED') DESC, started_at DESC, id DESC
		LIMIT 1
	) s ON TRUE`

const rosterCTE = `
	roster AS (
		SELECT a.student_id,
		       COALESCE(u.roll_no, '') AS roll_no,
		       TRIM(CONCAT(u.first_name, ' ', u.last_name)) AS name,
		       s.id AS session_id,
		       COALESCE(s.status, 'NOT_STARTED') AS status,
		       s.started_at,
		       s.submitted_at
		FROM assignments a
		JOIN users u ON u.id = a.student_id` + sessionLateral + `
		WHERE a.exam_id = $1 AND a.status = 'ASSIGNED'
	)`

// rosterFilter uses a backslash escape character; see EscapeLike.
const rosterFilter = `
	WHERE ($2::text IS NULL OR status = $2)
	  AND ($3::text IS NULL OR roll_no ILIKE ('%' || $3 || '%') ESCAPE E'\\'
	                        OR name ILIKE ('%' || $3 || '%') ESCAPE E'\\')`

// orderBy maps whitelisted sort values to SQL; user input never reaches SQL.
func orderBy(field SortField, order SortOrder) string {
	dir := "ASC"
	if order == OrderDesc {
		dir = "DESC"
	}
	switch field {
	case SortRollNo:
		return fmt.Sprintf("roll_no %s, student_id", dir)
	case SortSubmittedAt:
		return fmt.Sprintf("submitted_at %s NULLS LAST, LOWER(name), student_id", dir)
	default:
		return fmt.Sprintf("LOWER(name) %s, roll_no, student_id", dir)
	}
}

// EscapeLike escapes LIKE wildcards so a search for "50%" matches literally.
func EscapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (r *pgRepository) ListStudents(ctx context.Context, p ListParams) (*StudentPage, error) {
	var statusArg *string
	if p.Status != nil {
		s := string(*p.Status)
		statusArg = &s
	}
	var searchArg *string
	if p.Search != "" {
		s := EscapeLike(p.Search)
		searchArg = &s
	}

	page := &StudentPage{Rows: []StudentRow{}}
	err := r.readTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM questions WHERE exam_id = $1`, p.ExamID,
		).Scan(&page.TotalQuestions); err != nil {
			return err
		}

		rows, err := tx.Query(ctx,
			`WITH `+rosterCTE+` SELECT status, COUNT(*) FROM roster GROUP BY status`, p.ExamID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var status string
			var n int
			if err := rows.Scan(&status, &n); err != nil {
				rows.Close()
				return err
			}
			page.Counts.Assigned += n
			switch Status(status) {
			case StatusNotStarted:
				page.Counts.NotStarted = n
			case StatusInProgress:
				page.Counts.InProgress = n
			case StatusSubmitted:
				page.Counts.Submitted = n
			case StatusExpired:
				page.Counts.Expired = n
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		if err := tx.QueryRow(ctx,
			`WITH `+rosterCTE+` SELECT COUNT(*) FROM roster`+rosterFilter,
			p.ExamID, statusArg, searchArg,
		).Scan(&page.Total); err != nil {
			return err
		}
		if page.Total == 0 || p.Offset >= page.Total {
			return nil
		}

		// The answered count is computed only for the rows on this page.
		list, err := tx.Query(ctx, `
			WITH `+rosterCTE+`,
			page AS (
				SELECT roster.*, ROW_NUMBER() OVER (ORDER BY `+orderBy(p.Sort, p.Order)+`) AS rn
				FROM roster`+rosterFilter+`
				ORDER BY rn
				LIMIT $4 OFFSET $5
			)
			SELECT page.student_id, page.roll_no, page.name, page.session_id, page.status,
			       page.started_at, page.submitted_at,
			       CASE WHEN page.session_id IS NULL THEN NULL ELSE (
			           SELECT COUNT(*) FROM answers an
			           WHERE an.session_id = page.session_id AND `+answeredExpr+`
			       ) END
			FROM page
			ORDER BY page.rn`,
			p.ExamID, statusArg, searchArg, p.Limit, p.Offset)
		if err != nil {
			return err
		}
		defer list.Close()
		for list.Next() {
			var row StudentRow
			var status string
			var answered *int
			if err := list.Scan(&row.StudentID, &row.RollNo, &row.Name, &row.SessionID, &status,
				&row.StartedAt, &row.SubmittedAt, &answered); err != nil {
				return err
			}
			row.Status = Status(status)
			row.AnsweredCount = answered
			page.Rows = append(page.Rows, row)
		}
		return list.Err()
	})
	if err != nil {
		return nil, err
	}
	return page, nil
}

func (r *pgRepository) GetResponses(ctx context.Context, examID, studentID uuid.UUID, allowInProgress bool) (*Responses, error) {
	out := &Responses{}
	err := r.readTx(ctx, func(tx pgx.Tx) error {
		var (
			sessionID              *uuid.UUID
			status                 *string
			startedAt, submittedAt *time.Time
		)
		err := tx.QueryRow(ctx, `
			SELECT a.student_id,
			       COALESCE(u.roll_no, ''),
			       TRIM(CONCAT(u.first_name, ' ', u.last_name)),
			       s.id, s.status, s.started_at, s.submitted_at
			FROM assignments a
			JOIN users u ON u.id = a.student_id`+sessionLateral+`
			WHERE a.exam_id = $1 AND a.student_id = $2 AND a.status = 'ASSIGNED'`,
			examID, studentID,
		).Scan(&out.StudentID, &out.RollNo, &out.Name, &sessionID, &status, &startedAt, &submittedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrStudentNotAssigned
			}
			return err
		}
		if sessionID != nil && status != nil && startedAt != nil {
			out.Session = &SessionInfo{ID: *sessionID, Status: Status(*status), StartedAt: *startedAt, SubmittedAt: submittedAt}
		}

		load := out.Session != nil && (out.Session.Status != StatusInProgress || allowInProgress)
		if !load {
			return tx.QueryRow(ctx, `SELECT COUNT(*) FROM questions WHERE exam_id = $1`, examID).Scan(&out.TotalQuestions)
		}

		questions, err := loadQuestions(ctx, tx, examID)
		if err != nil {
			return err
		}
		if err := attachAnswers(ctx, tx, out.Session.ID, questions); err != nil {
			return err
		}
		if err := attachRecordings(ctx, tx, out.Session.ID, questions); err != nil {
			return err
		}
		out.Questions = questions
		out.TotalQuestions = len(questions)
		out.Loaded = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// loadQuestions reads the exam's questions and all their options in two
// queries, in question-number order.
func loadQuestions(ctx context.Context, tx pgx.Tx, examID uuid.UUID) ([]QuestionRecord, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, question_number, text, type, points
		FROM questions
		WHERE exam_id = $1
		ORDER BY question_number, id`, examID)
	if err != nil {
		return nil, err
	}
	questions := []QuestionRecord{}
	index := map[uuid.UUID]int{}
	ids := []uuid.UUID{}
	for rows.Next() {
		var q QuestionRecord
		if err := rows.Scan(&q.ID, &q.QuestionNumber, &q.Text, &q.Type, &q.Points); err != nil {
			rows.Close()
			return nil, err
		}
		index[q.ID] = len(questions)
		ids = append(ids, q.ID)
		questions = append(questions, q)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return questions, nil
	}

	opts, err := tx.Query(ctx, `
		SELECT id, question_id, option_key, option_text, display_order, is_correct
		FROM question_options
		WHERE question_id = ANY($1)
		ORDER BY display_order, option_key, id`, ids)
	if err != nil {
		return nil, err
	}
	defer opts.Close()
	for opts.Next() {
		var o OptionRecord
		var qid uuid.UUID
		if err := opts.Scan(&o.ID, &qid, &o.Key, &o.Text, &o.DisplayOrder, &o.IsCorrect); err != nil {
			return nil, err
		}
		if i, ok := index[qid]; ok {
			questions[i].Options = append(questions[i].Options, o)
		}
	}
	return questions, opts.Err()
}

// attachAnswers fills in each question's answer from one query on the session.
func attachAnswers(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID, questions []QuestionRecord) error {
	index := make(map[uuid.UUID]int, len(questions))
	for i := range questions {
		index[questions[i].ID] = i
	}
	rows, err := tx.Query(ctx, `
		SELECT a.question_id, a.selected_option_id, a.text_answer, a.updated_at,
		       COALESCE((SELECT v.source FROM answer_revisions v
		                 WHERE v.answer_id = a.id ORDER BY v.revision_no DESC LIMIT 1), '')
		FROM answers a
		WHERE a.session_id = $1`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var qid uuid.UUID
		var a AnswerRecord
		if err := rows.Scan(&qid, &a.SelectedOptionID, &a.TextAnswer, &a.UpdatedAt, &a.Source); err != nil {
			return err
		}
		// Answers for questions no longer in the exam are not shown.
		if i, ok := index[qid]; ok {
			questions[i].Answer = &a
		}
	}
	return rows.Err()
}

// attachRecordings fills in each question's retained audio from one query. A
// removed recording is listed (so the reviewer knows audio once existed) but
// carries no way to play it.
func attachRecordings(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID, questions []QuestionRecord) error {
	index := make(map[uuid.UUID]int, len(questions))
	for i := range questions {
		index[questions[i].ID] = i
	}
	rows, err := tx.Query(ctx, `
		SELECT r.question_id, r.id, r.segment_index, r.status, r.duration_ms, r.recorded_at,
		       COALESCE(t.text, ''), r.answer_revision_id IS NOT NULL
		FROM audio_recordings r
		LEFT JOIN transcriptions t ON t.recording_id = r.id
		WHERE r.session_id = $1
		ORDER BY r.recorded_at, r.segment_index, r.id`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var qid uuid.UUID
		var rec RecordingRecord
		var status string
		if err := rows.Scan(&qid, &rec.ID, &rec.SegmentIndex, &status, &rec.DurationMillis, &rec.RecordedAt,
			&rec.Transcript, &rec.Confirmed); err != nil {
			return err
		}
		rec.Status = RecordingAvailable
		if status != "STORED" {
			rec.Status = RecordingRemoved
		}
		if i, ok := index[qid]; ok {
			questions[i].Recordings = append(questions[i].Recordings, rec)
		}
	}
	return rows.Err()
}

func (r *pgRepository) GetRecording(ctx context.Context, examID, studentID, recordingID uuid.UUID) (*RecordingFile, error) {
	var f RecordingFile
	var status, sessionStatus string
	err := r.pool.QueryRow(ctx, `
		SELECT r.id, r.session_id, s.status, r.storage_key, r.content_type, r.byte_size, r.status
		FROM audio_recordings r
		JOIN sessions s ON s.id = r.session_id
		WHERE r.id = $1 AND s.exam_id = $2 AND s.student_id = $3`,
		recordingID, examID, studentID,
	).Scan(&f.ID, &f.SessionID, &sessionStatus, &f.StorageKey, &f.ContentType, &f.ByteSize, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecordingNotFound
		}
		return nil, err
	}
	f.SessionStatus = Status(sessionStatus)
	f.Removed = status != "STORED"
	return &f, nil
}
