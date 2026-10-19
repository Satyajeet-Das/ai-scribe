package integration

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/answer"
	"github.com/Satyajeet-Das/ai-scribe/internal/audit"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/recording"
	"github.com/Satyajeet-Das/ai-scribe/internal/review"
)

// submitOnPut is a store that ends the session as soon as the audio is
// stored, so the metadata write runs against a session that has just ended.
type submitOnPut struct {
	*audiostore.Memory
	submit func()
}

func (s *submitOnPut) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	if err := s.Memory.Put(ctx, key, r, size); err != nil {
		return err
	}
	s.submit()
	return nil
}

// The seed helpers (reviewSeed) are shared with the review integration test.
func TestRecording_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	testDB, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	seed := reviewSeed{pool: testDB.Pool, ctx: ctx, t: t}
	nop := zerolog.Nop()

	teacher := seed.user("TEACHER", "Terry", "Teacher", "")
	student := seed.user("STUDENT", "Sam", "Student", "R100")
	examID := seed.exam(teacher, "Biology")
	qID := seed.question(examID, 1, "ESSAY", "Explain photosynthesis", 10)
	assignment := seed.assign(examID, student, "ASSIGNED")
	sessionID := seed.session(assignment, examID, student, "IN_PROGRESS", time.Now())

	store := audiostore.NewMemory()
	svc := recording.NewService(recording.NewRepository(testDB.Pool), store, recording.Options{RetentionDays: 90}, &nop)

	newInput := func(stream uuid.UUID) recording.SaveInput {
		return recording.SaveInput{
			SessionID: sessionID, QuestionID: qID, StudentID: student, StreamID: stream,
			SampleRate: 16000, ChunkCount: 3, PCM: bytes.Repeat([]byte{7, 0}, 8000),
			Transcript: "plants make sugar from light", Provider: "deepgram", Language: "en",
		}
	}
	count := func(table string) int {
		var n int
		require.NoError(t, testDB.Pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n))
		return n
	}

	t.Run("saves audio metadata and the machine transcription together", func(t *testing.T) {
		stream := uuid.New()
		saved, err := svc.Save(ctx, newInput(stream))
		require.NoError(t, err)

		var key, text, contentType string
		var dur int
		var until *time.Time
		var sha []byte
		require.NoError(t, testDB.Pool.QueryRow(ctx, `
			SELECT r.storage_key, r.content_type, r.duration_ms, r.retention_until, r.sha256, t.text
			FROM audio_recordings r JOIN transcriptions t ON t.recording_id = r.id
			WHERE r.id = $1`, saved.RecordingID).Scan(&key, &contentType, &dur, &until, &sha, &text))

		assert.Equal(t, "audio/wav", contentType)
		assert.Equal(t, 500, dur)
		assert.Equal(t, "plants make sugar from light", text)
		assert.Len(t, sha, 32)
		require.NotNil(t, until)
		assert.WithinDuration(t, time.Now().AddDate(0, 0, 90), *until, time.Minute)
		assert.NotContains(t, key, sessionID.String(), "the storage key reveals nothing about the exam")
		_, _, err = store.Open(ctx, key)
		assert.NoError(t, err, "the row points at audio that exists")
	})

	t.Run("a retried stream is stored once, even when retries race", func(t *testing.T) {
		before := count("audio_recordings")
		stream := uuid.New()

		var wg sync.WaitGroup
		results := make([]*recording.Saved, 6)
		errs := make([]error, 6)
		for i := range results {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i], errs[i] = svc.Save(ctx, newInput(stream))
			}()
		}
		wg.Wait()

		for i := range results {
			require.NoError(t, errs[i])
			assert.Equal(t, results[0].RecordingID, results[i].RecordingID, "every retry sees the same recording")
		}
		assert.Equal(t, before+1, count("audio_recordings"))

		var objects int
		for range store.Keys() {
			objects++
		}
		assert.Equal(t, count("audio_recordings"), objects, "no orphaned audio is left by the losing retries")
	})

	t.Run("a stream id reused with different audio is refused, and the first transcription is untouched", func(t *testing.T) {
		stream := uuid.New()
		first, err := svc.Save(ctx, newInput(stream))
		require.NoError(t, err)
		recordings, objects := count("audio_recordings"), len(store.Keys())

		other := newInput(stream)
		other.PCM = bytes.Repeat([]byte{5, 0}, 8000)
		other.Transcript = "read the question"
		_, err = svc.Save(ctx, other)
		assert.ErrorIs(t, err, recording.ErrStreamReused)
		assert.Equal(t, recordings, count("audio_recordings"))
		assert.Equal(t, objects, len(store.Keys()), "the new audio is not left in the store")

		var text string
		require.NoError(t, testDB.Pool.QueryRow(ctx,
			`SELECT text FROM transcriptions WHERE id = $1`, first.TranscriptionID).Scan(&text))
		assert.Equal(t, "plants make sugar from light", text)
	})

	t.Run("a recording is refused once the session has ended, even if it ends while the audio is stored", func(t *testing.T) {
		endedExam := seed.exam(teacher, "Ended")
		ended := seed.session(seed.assign(endedExam, student, "ASSIGNED"), endedExam, student, "SUBMITTED", time.Now())
		in := newInput(uuid.New())
		in.SessionID = ended
		_, err := svc.Save(ctx, in)
		assert.ErrorIs(t, err, recording.ErrSessionEnded)

		racingExam := seed.exam(teacher, "Racing")
		racing := seed.session(seed.assign(racingExam, student, "ASSIGNED"), racingExam, student, "IN_PROGRESS", time.Now())
		submitting := &submitOnPut{Memory: audiostore.NewMemory(), submit: func() {
			_, err := testDB.Pool.Exec(ctx, `UPDATE sessions SET status = 'SUBMITTED', submitted_at = NOW() WHERE id = $1`, racing)
			require.NoError(t, err)
		}}
		raced := recording.NewService(recording.NewRepository(testDB.Pool), submitting, recording.Options{}, &nop)
		in = newInput(uuid.New())
		in.SessionID = racing
		_, err = raced.Save(ctx, in)
		assert.ErrorIs(t, err, recording.ErrSessionEnded)
		assert.Empty(t, submitting.Keys(), "the audio stored before the session ended is removed")

		var n int
		require.NoError(t, testDB.Pool.QueryRow(ctx,
			`SELECT count(*) FROM audio_recordings WHERE session_id = ANY($1)`, []uuid.UUID{ended, racing}).Scan(&n))
		assert.Zero(t, n)
	})

	t.Run("a transcription cannot be added twice for one recording", func(t *testing.T) {
		saved, err := svc.Save(ctx, newInput(uuid.New()))
		require.NoError(t, err)
		_, err = testDB.Pool.Exec(ctx,
			`INSERT INTO transcriptions (recording_id, text) VALUES ($1, 'a second, different transcript')`, saved.RecordingID)
		assert.Error(t, err)
	})

	t.Run("the database refuses malformed recordings", func(t *testing.T) {
		_, err := testDB.Pool.Exec(ctx, `
			INSERT INTO audio_recordings (session_id, question_id, student_id, stream_id, content_type, sample_rate,
			                              duration_ms, byte_size, chunk_count, sha256, storage_key)
			VALUES ($1, $2, $3, $4, 'audio/wav', 16000, 1, 10, 1, '\x00', 'k')`, sessionID, qID, student, uuid.New())
		assert.Error(t, err, "a checksum must be 32 bytes")
	})
}

func TestAnswerRevisions_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	testDB, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	seed := reviewSeed{pool: testDB.Pool, ctx: ctx, t: t}
	nop := zerolog.Nop()

	teacher := seed.user("TEACHER", "Terry", "Teacher", "")
	student := seed.user("STUDENT", "Sam", "Student", "R100")
	examID := seed.exam(teacher, "Biology")
	essay := seed.question(examID, 1, "ESSAY", "Explain photosynthesis", 10)
	otherEssay := seed.question(examID, 2, "ESSAY", "Explain respiration", 10)
	assignment := seed.assign(examID, student, "ASSIGNED")
	sessionID := seed.session(assignment, examID, student, "IN_PROGRESS", time.Now())

	repo := answer.NewRepository(testDB.Pool)
	rec := recording.NewService(recording.NewRepository(testDB.Pool), audiostore.NewMemory(), recording.Options{}, &nop)

	type revision struct {
		No     int
		Source string
		Text   string
	}
	revisions := func(q uuid.UUID) []revision {
		rows, err := testDB.Pool.Query(ctx, `
			SELECT r.revision_no, r.source, r.text_answer
			FROM answer_revisions r JOIN answers a ON a.id = r.answer_id
			WHERE a.session_id = $1 AND a.question_id = $2 ORDER BY r.revision_no`, sessionID, q)
		require.NoError(t, err)
		defer rows.Close()
		var out []revision
		for rows.Next() {
			var r revision
			require.NoError(t, rows.Scan(&r.No, &r.Source, &r.Text))
			out = append(out, r)
		}
		return out
	}
	save := func(q uuid.UUID, text string, rev *answer.RevisionInput) *answer.Answer {
		a := &answer.Answer{SessionID: sessionID, QuestionID: q, TextAnswer: text, Revision: rev}
		require.NoError(t, repo.Upsert(ctx, a))
		return a
	}
	dictate := func(q uuid.UUID, text string) *recording.Saved {
		saved, err := rec.Save(ctx, recording.SaveInput{
			SessionID: sessionID, QuestionID: q, StudentID: student, StreamID: uuid.New(),
			SampleRate: 16000, ChunkCount: 1, PCM: bytes.Repeat([]byte{1, 0}, 800),
			Transcript: text, Provider: "test", Language: "en",
		})
		require.NoError(t, err)
		return saved
	}

	t.Run("autosaved typing coalesces into one revision", func(t *testing.T) {
		save(essay, "Pl", nil)
		save(essay, "Plants", nil)
		save(essay, "Plants make sugar", nil)
		assert.Equal(t, []revision{{1, "TYPED", "Plants make sugar"}}, revisions(essay))
	})

	t.Run("a confirmed dictation is a new revision linked to its recording and transcription", func(t *testing.T) {
		saved := dictate(essay, "Plants turn light into energy")
		a := save(essay, "Plants turn light into energy", &answer.RevisionInput{
			Source: answer.SourceSTTConfirmed, TranscriptionID: &saved.TranscriptionID,
			ConfirmationID: "conf-1", RecordingIDs: []uuid.UUID{saved.RecordingID},
		})

		assert.Equal(t, []revision{
			{1, "TYPED", "Plants make sugar"},
			{2, "STT_CONFIRMED", "Plants turn light into energy"},
		}, revisions(essay), "the earlier typed text is kept, not overwritten")

		var linkedAnswer uuid.UUID
		require.NoError(t, testDB.Pool.QueryRow(ctx, `
			SELECT a.id FROM audio_recordings r
			JOIN answer_revisions v ON v.id = r.answer_revision_id
			JOIN answers a ON a.id = v.answer_id WHERE r.id = $1`, saved.RecordingID).Scan(&linkedAnswer))
		assert.Equal(t, a.ID, linkedAnswer)
	})

	t.Run("the same confirmation twice adds nothing", func(t *testing.T) {
		before := len(revisions(essay))
		save(essay, "Plants turn light into energy", &answer.RevisionInput{
			Source: answer.SourceSTTConfirmed, ConfirmationID: "conf-1",
		})
		assert.Len(t, revisions(essay), before)
	})

	t.Run("typing after a dictation starts a new revision and keeps the confirmed one", func(t *testing.T) {
		save(essay, "Plants turn light into energy, stored as sugar", nil)
		got := revisions(essay)
		require.Len(t, got, 3)
		assert.Equal(t, revision{2, "STT_CONFIRMED", "Plants turn light into energy"}, got[1], "a confirmed revision is never rewritten")
		assert.Equal(t, revision{3, "TYPED", "Plants turn light into energy, stored as sugar"}, got[2])
	})

	t.Run("a recording of another question cannot be attached to this answer", func(t *testing.T) {
		stranger := dictate(otherEssay, "something else entirely")
		save(essay, "Plants, again", &answer.RevisionInput{
			Source: answer.SourceSTTConfirmed, ConfirmationID: "conf-2",
			RecordingIDs: []uuid.UUID{stranger.RecordingID},
		})
		var linked *uuid.UUID
		require.NoError(t, testDB.Pool.QueryRow(ctx,
			`SELECT answer_revision_id FROM audio_recordings WHERE id = $1`, stranger.RecordingID).Scan(&linked))
		assert.Nil(t, linked)
	})

	current := func(q uuid.UUID) (text string, option *uuid.UUID) {
		require.NoError(t, testDB.Pool.QueryRow(ctx,
			`SELECT text_answer, selected_option_id FROM answers WHERE session_id = $1 AND question_id = $2`,
			sessionID, q).Scan(&text, &option))
		return
	}
	confirmed := func(id, text string) *answer.RevisionInput {
		return &answer.RevisionInput{Source: answer.SourceSTTConfirmed, ConfirmationID: id}
	}

	t.Run("a replayed confirmation changes nothing, even after the student has typed something else", func(t *testing.T) {
		save(otherEssay, "foo", confirmed("replay-1", "foo"))
		save(otherEssay, "bar", nil) // the student then types a different answer
		before := revisions(otherEssay)

		// The confirmation arrives again, with the same text and then with different text.
		for _, text := range []string{"foo", "something else"} {
			a := &answer.Answer{SessionID: sessionID, QuestionID: otherEssay, TextAnswer: text, Revision: confirmed("replay-1", text)}
			require.NoError(t, repo.Upsert(ctx, a))
			assert.True(t, a.Replayed)
			assert.Equal(t, "bar", a.TextAnswer, "the caller is told what the answer is now, not what the replay asked")
		}

		got, _ := current(otherEssay)
		assert.Equal(t, "bar", got, "the old value must not overwrite the newer one")
		assert.Equal(t, before, revisions(otherEssay), "and the history is exactly as it was")
		last := before[len(before)-1]
		assert.Equal(t, revision{last.No, "TYPED", "bar"}, last)
	})

	t.Run("the same confirmation arriving at the same moment is applied once", func(t *testing.T) {
		const copies = 8
		var wg sync.WaitGroup
		results := make([]*answer.Answer, copies)
		for i := 0; i < copies; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				a := &answer.Answer{SessionID: sessionID, QuestionID: essay, TextAnswer: "raced answer",
					Revision: confirmed("race-1", "raced answer")}
				assert.NoError(t, repo.Upsert(ctx, a))
				results[i] = a
			}()
		}
		wg.Wait()

		var n int
		require.NoError(t, testDB.Pool.QueryRow(ctx, `
			SELECT count(*) FROM answer_revisions v JOIN answers a ON a.id = v.answer_id
			WHERE a.session_id = $1 AND a.question_id = $2 AND v.confirmation_id = 'race-1'`, sessionID, essay).Scan(&n))
		assert.Equal(t, 1, n)
		replays := 0
		for _, a := range results {
			if a.Replayed {
				replays++
			}
		}
		assert.Equal(t, copies-1, replays, "one request wrote it and the others saw that it was done")
		got, _ := current(essay)
		assert.Equal(t, "raced answer", got)
	})

	t.Run("a confirmed option is a revision of its own and keeps its confirmation id", func(t *testing.T) {
		mcq := seed.question(examID, 3, "MCQ", "Pick one", 1)
		optA := seed.option(mcq, "A", "alpha", 1, false)
		optB := seed.option(mcq, "B", "beta", 2, false)
		saveOption := func(opt uuid.UUID, rev *answer.RevisionInput) {
			require.NoError(t, repo.Upsert(ctx, &answer.Answer{SessionID: sessionID, QuestionID: mcq, SelectedOptionID: &opt, Revision: rev}))
		}
		type optRev struct {
			No           int
			Source       string
			Option       uuid.UUID
			Confirmation *string
		}
		history := func() []optRev {
			rows, err := testDB.Pool.Query(ctx, `
				SELECT v.revision_no, v.source, v.selected_option_id, v.confirmation_id
				FROM answer_revisions v JOIN answers a ON a.id = v.answer_id
				WHERE a.session_id = $1 AND a.question_id = $2 ORDER BY v.revision_no`, sessionID, mcq)
			require.NoError(t, err)
			defer rows.Close()
			var out []optRev
			for rows.Next() {
				var r optRev
				require.NoError(t, rows.Scan(&r.No, &r.Source, &r.Option, &r.Confirmation))
				out = append(out, r)
			}
			return out
		}
		conf := "opt-1"

		saveOption(optA, nil)                                                                      // mouse
		saveOption(optB, &answer.RevisionInput{Source: answer.SourceOption, ConfirmationID: conf}) // voice, confirmed
		h := history()
		require.Len(t, h, 2, "the confirmed choice is not merged into the earlier mouse choice")
		require.NotNil(t, h[1].Confirmation)
		assert.Equal(t, conf, *h[1].Confirmation, "its confirmation id is recorded, so a replay can be recognized")

		// A replay of that confirmation, after the student went back to A with the mouse.
		saveOption(optA, nil)
		h = history()
		require.Len(t, h, 3, "a later mouse choice starts a new revision rather than rewriting the confirmed one")
		assert.Equal(t, optB, h[1].Option, "the confirmed revision still says B")

		replay := &answer.Answer{SessionID: sessionID, QuestionID: mcq, SelectedOptionID: &optB,
			Revision: &answer.RevisionInput{Source: answer.SourceOption, ConfirmationID: conf}}
		require.NoError(t, repo.Upsert(ctx, replay))
		assert.True(t, replay.Replayed)
		_, opt := current(mcq)
		require.NotNil(t, opt)
		assert.Equal(t, optA, *opt, "the replay does not put B back over the student's later choice")
		assert.Len(t, history(), 3)
	})

	t.Run("autosaving keeps the time a typed revision began", func(t *testing.T) {
		q := seed.question(examID, 4, "ESSAY", "Another", 1)
		save(q, "a", nil)
		var first time.Time
		require.NoError(t, testDB.Pool.QueryRow(ctx, `
			SELECT v.created_at FROM answer_revisions v JOIN answers a ON a.id = v.answer_id
			WHERE a.session_id = $1 AND a.question_id = $2`, sessionID, q).Scan(&first))

		time.Sleep(20 * time.Millisecond)
		save(q, "ab", nil)
		save(q, "abc", nil)

		var again time.Time
		require.NoError(t, testDB.Pool.QueryRow(ctx, `
			SELECT v.created_at FROM answer_revisions v JOIN answers a ON a.id = v.answer_id
			WHERE a.session_id = $1 AND a.question_id = $2`, sessionID, q).Scan(&again))
		assert.True(t, first.Equal(again), "coalescing must not rewrite when the revision began")
		assert.Len(t, revisions(q), 1)
	})

	t.Run("nothing is written once the session has ended", func(t *testing.T) {
		before := len(revisions(essay))
		_, err := testDB.Pool.Exec(ctx, `UPDATE sessions SET status = 'SUBMITTED', submitted_at = NOW() WHERE id = $1`, sessionID)
		require.NoError(t, err)

		a := &answer.Answer{SessionID: sessionID, QuestionID: essay, TextAnswer: "too late"}
		assert.ErrorIs(t, repo.Upsert(ctx, a), answer.ErrSessionNotActive)
		assert.Len(t, revisions(essay), before)

		// A replayed confirmation on an ended session is refused too; it is not
		// answered as though everything were fine.
		replay := &answer.Answer{SessionID: sessionID, QuestionID: otherEssay, TextAnswer: "foo", Revision: confirmed("replay-1", "foo")}
		assert.ErrorIs(t, repo.Upsert(ctx, replay), answer.ErrSessionNotActive)
	})
}

func TestReviewPlayback_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	testDB, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	seed := reviewSeed{pool: testDB.Pool, ctx: ctx, t: t}
	nop := zerolog.Nop()

	teacher := seed.user("TEACHER", "Terry", "Teacher", "")
	otherTeacher := seed.user("TEACHER", "Olive", "Other", "")
	student := seed.user("STUDENT", "Sam", "Student", "R100")
	classmate := seed.user("STUDENT", "Cam", "Classmate", "R101")

	examID := seed.exam(teacher, "Biology")
	essay := seed.question(examID, 1, "ESSAY", "Explain photosynthesis", 10)
	otherExam := seed.exam(teacher, "Chemistry")
	seed.question(otherExam, 1, "ESSAY", "Explain bonding", 10)

	sessionOf := func(exam, who uuid.UUID, status string) uuid.UUID {
		return seed.session(seed.assign(exam, who, "ASSIGNED"), exam, who, status, time.Now())
	}
	sessionID := sessionOf(examID, student, "IN_PROGRESS")
	classmateSession := sessionOf(examID, classmate, "IN_PROGRESS")
	sessionOf(otherExam, student, "SUBMITTED")

	store := audiostore.NewMemory()
	recSvc := recording.NewService(recording.NewRepository(testDB.Pool), store, recording.Options{}, &nop)
	answers := answer.NewRepository(testDB.Pool)
	repo := review.NewRepository(testDB.Pool)

	dictate := func(sid, student uuid.UUID, transcript string) *recording.Saved {
		saved, err := recSvc.Save(ctx, recording.SaveInput{
			SessionID: sid, QuestionID: essay, StudentID: student, StreamID: uuid.New(),
			SampleRate: 16000, ChunkCount: 1, PCM: bytes.Repeat([]byte{3, 0}, 1600), Transcript: transcript,
		})
		require.NoError(t, err)
		return saved
	}

	// Two dictations: the student confirmed the second, and corrected a word
	// of it by typing afterwards.
	rejected := dictate(sessionID, student, "plants make shugar")
	kept := dictate(sessionID, student, "plants make sugar")
	require.NoError(t, answers.Upsert(ctx, &answer.Answer{
		SessionID: sessionID, QuestionID: essay, TextAnswer: "plants make sugar",
		Revision: &answer.RevisionInput{
			Source: answer.SourceSTTConfirmed, TranscriptionID: &kept.TranscriptionID,
			ConfirmationID: "c1", RecordingIDs: []uuid.UUID{kept.RecordingID},
		},
	}))
	// Audio is only ever recorded while a session is in progress; the
	// classmate has since submitted.
	foreign := dictate(classmateSession, classmate, "the classmate's private answer")
	_, err := testDB.Pool.Exec(ctx, `UPDATE sessions SET status = 'SUBMITTED', submitted_at = NOW() WHERE id = $1`, classmateSession)
	require.NoError(t, err)

	t.Run("the review lists each recording with the unedited transcript and what the student confirmed", func(t *testing.T) {
		got, err := repo.GetResponses(ctx, examID, student, true)
		require.NoError(t, err)
		require.True(t, got.Loaded)
		q := got.Questions[0]

		require.Len(t, q.Recordings, 2)
		assert.Equal(t, "plants make shugar", q.Recordings[0].Transcript, "the recognizer's words are kept as heard")
		assert.False(t, q.Recordings[0].Confirmed)
		assert.Equal(t, rejected.RecordingID, q.Recordings[0].ID)
		assert.Equal(t, "plants make sugar", q.Recordings[1].Transcript)
		assert.True(t, q.Recordings[1].Confirmed)
		assert.Equal(t, review.RecordingAvailable, q.Recordings[1].Status)
		assert.Equal(t, 100, q.Recordings[1].DurationMillis)

		require.NotNil(t, q.Answer)
		assert.Equal(t, answer.SourceSTTConfirmed, q.Answer.Source)
	})

	t.Run("a question with no audio has an empty list, not a missing one", func(t *testing.T) {
		got, err := repo.GetResponses(ctx, otherExam, student, true)
		require.NoError(t, err)
		require.True(t, got.Loaded)
		assert.Empty(t, got.Questions[0].Recordings)
		dto := review.ToStudentResponsesResponse(&review.StudentResponses{Questions: got.Questions})
		assert.NotNil(t, dto.Questions[0].Recordings)
	})

	t.Run("a recording is found only through its own student and exam", func(t *testing.T) {
		f, err := repo.GetRecording(ctx, examID, student, kept.RecordingID)
		require.NoError(t, err)
		assert.Equal(t, sessionID, f.SessionID)
		assert.False(t, f.Removed)

		_, err = repo.GetRecording(ctx, examID, classmate, kept.RecordingID)
		assert.ErrorIs(t, err, review.ErrRecordingNotFound, "another student's ID cannot reach it")
		_, err = repo.GetRecording(ctx, otherExam, student, kept.RecordingID)
		assert.ErrorIs(t, err, review.ErrRecordingNotFound, "another exam's ID cannot reach it")
		_, err = repo.GetRecording(ctx, examID, student, foreign.RecordingID)
		assert.ErrorIs(t, err, review.ErrRecordingNotFound, "the classmate's recording is not reachable through this student")
		_, err = repo.GetRecording(ctx, examID, student, uuid.New())
		assert.ErrorIs(t, err, review.ErrRecordingNotFound)
	})

	examSvc := exam.NewService(exam.NewRepository(testDB.Pool), &nop)
	auditRec := audit.NewRecorder(testDB.Pool)
	svc := review.NewService(repo, examSvc, review.DefaultPolicy(), &nop)
	playback := review.EnablePlayback(svc, store, auditRec)
	owner := review.Caller{ID: teacher, Role: platformauth.RoleTeacher}
	auditCount := func() int {
		var n int
		require.NoError(t, testDB.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events`).Scan(&n))
		return n
	}

	t.Run("playing a recording is audited, without any content", func(t *testing.T) {
		before := auditCount()
		audio, err := playback.OpenRecording(ctx, examID, student, kept.RecordingID, owner,
			review.AccessMeta{RequestID: "req-abc", IP: "203.0.113.7"})
		require.NoError(t, err)
		data, _ := io.ReadAll(audio.Body)
		audio.Body.Close()
		assert.Equal(t, "RIFF", string(data[:4]))
		assert.Equal(t, before+1, auditCount())

		var actor uuid.UUID
		var action, role, requestID, ip, meta string
		var resource, sess uuid.UUID
		require.NoError(t, testDB.Pool.QueryRow(ctx, `
			SELECT actor_id, actor_role, action, resource_id, session_id, request_id, host(ip), metadata::text
			FROM audit_events ORDER BY id DESC LIMIT 1`).Scan(&actor, &role, &action, &resource, &sess, &requestID, &ip, &meta))
		assert.Equal(t, teacher, actor)
		assert.Equal(t, "audio.played", action)
		assert.Equal(t, kept.RecordingID, resource)
		assert.Equal(t, sessionID, sess)
		assert.Equal(t, "req-abc", requestID)
		assert.Equal(t, "203.0.113.7", ip)
		assert.Equal(t, "{}", meta)
	})

	t.Run("a refused request writes no playback entry", func(t *testing.T) {
		before := auditCount()
		stranger := review.Caller{ID: otherTeacher, Role: platformauth.RoleTeacher}
		_, err := playback.OpenRecording(ctx, examID, student, kept.RecordingID, stranger, review.AccessMeta{})
		assert.ErrorIs(t, err, review.ErrExamNotFound)
		_, err = playback.OpenRecording(ctx, examID, classmate, kept.RecordingID, owner, review.AccessMeta{})
		assert.ErrorIs(t, err, review.ErrRecordingNotFound)
		assert.Equal(t, before, auditCount())
	})

	t.Run("an unusable client address is stored as null, and a long request id is shortened", func(t *testing.T) {
		require.NoError(t, auditRec.Record(ctx, audit.Event{
			ActorRole: "TEACHER", Action: audit.ActionAudioPlayed, ResourceType: audit.ResourceRecording,
			IP: "not-an-ip", RequestID: strings.Repeat("x", 200),
		}))
		var ip *string
		var rid string
		require.NoError(t, testDB.Pool.QueryRow(ctx,
			`SELECT host(ip), request_id FROM audit_events ORDER BY id DESC LIMIT 1`).Scan(&ip, &rid))
		assert.Nil(t, ip)
		assert.Len(t, rid, 64)
	})

	t.Run("a removed recording stays listed with its transcript but cannot be played", func(t *testing.T) {
		_, err := testDB.Pool.Exec(ctx,
			`UPDATE audio_recordings SET status = 'DELETED', deleted_at = NOW() WHERE id = $1`, rejected.RecordingID)
		require.NoError(t, err)

		got, err := repo.GetResponses(ctx, examID, student, true)
		require.NoError(t, err)
		listed := got.Questions[0].Recordings[0]
		assert.Equal(t, review.RecordingRemoved, listed.Status)
		assert.Equal(t, "plants make shugar", listed.Transcript)

		_, err = playback.OpenRecording(ctx, examID, student, rejected.RecordingID, owner, review.AccessMeta{})
		assert.ErrorIs(t, err, review.ErrRecordingRemoved)
	})

	t.Run("playing never changes an answer, a session or a recording", func(t *testing.T) {
		before := reviewSeed{pool: testDB.Pool, ctx: ctx, t: t}.snapshot()
		audio, err := playback.OpenRecording(ctx, examID, student, kept.RecordingID, owner, review.AccessMeta{})
		require.NoError(t, err)
		audio.Body.Close()
		assert.Equal(t, before, reviewSeed{pool: testDB.Pool, ctx: ctx, t: t}.snapshot())
	})
}

func TestRecordingJanitor_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	testDB, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	seed := reviewSeed{pool: testDB.Pool, ctx: ctx, t: t}
	nop := zerolog.Nop()

	teacher := seed.user("TEACHER", "Terry", "Teacher", "")
	student := seed.user("STUDENT", "Sam", "Student", "R100")
	examID := seed.exam(teacher, "Biology")
	essay := seed.question(examID, 1, "ESSAY", "Explain photosynthesis", 10)
	assignment := seed.assign(examID, student, "ASSIGNED")
	sessionID := seed.session(assignment, examID, student, "IN_PROGRESS", time.Now())

	store := audiostore.NewMemory()
	recSvc := recording.NewService(recording.NewRepository(testDB.Pool), store, recording.Options{}, &nop)
	repo := recording.NewJanitorRepository(testDB.Pool)

	dictate := func(retention *time.Time) *recording.Saved {
		saved, err := recSvc.Save(ctx, recording.SaveInput{
			SessionID: sessionID, QuestionID: essay, StudentID: student, StreamID: uuid.New(),
			SampleRate: 16000, ChunkCount: 1, PCM: bytes.Repeat([]byte{2, 0}, 800), Transcript: "plants make sugar",
		})
		require.NoError(t, err)
		_, err = testDB.Pool.Exec(ctx, `UPDATE audio_recordings SET retention_until = $2 WHERE id = $1`, saved.RecordingID, retention)
		require.NoError(t, err)
		return saved
	}
	ago := func(d time.Duration) *time.Time { v := time.Now().Add(-d); return &v }
	later := func(d time.Duration) *time.Time { v := time.Now().Add(d); return &v }
	status := func(id uuid.UUID) (string, *time.Time) {
		var s string
		var at *time.Time
		require.NoError(t, testDB.Pool.QueryRow(ctx, `SELECT status, deleted_at FROM audio_recordings WHERE id = $1`, id).Scan(&s, &at))
		return s, at
	}
	auditRows := func(action string) int {
		var n int
		require.NoError(t, testDB.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action = $1`, action).Scan(&n))
		return n
	}
	keyOf := func(id uuid.UUID) string {
		var k string
		require.NoError(t, testDB.Pool.QueryRow(ctx, `SELECT storage_key FROM audio_recordings WHERE id = $1`, id).Scan(&k))
		return k
	}
	hasObject := func(key string) bool { _, _, err := store.Open(ctx, key); return err == nil }

	t.Run("only recordings past their retention date are due, oldest first", func(t *testing.T) {
		old := dictate(ago(48 * time.Hour))
		recent := dictate(ago(time.Hour))
		future := dictate(later(time.Hour))
		none := dictate(nil)

		due, err := repo.Expired(ctx, time.Now(), 100)
		require.NoError(t, err)
		ids := []uuid.UUID{}
		for _, d := range due {
			ids = append(ids, d.ID)
		}
		require.Equal(t, []uuid.UUID{old.RecordingID, recent.RecordingID}, ids)
		assert.NotContains(t, ids, future.RecordingID)
		assert.NotContains(t, ids, none.RecordingID, "no retention date, never due")

		limited, err := repo.Expired(ctx, time.Now(), 1)
		require.NoError(t, err)
		assert.Len(t, limited, 1)

		// Leave the table as the other subtests expect it.
		for _, id := range []uuid.UUID{old.RecordingID, recent.RecordingID, future.RecordingID, none.RecordingID} {
			_, err := repo.MarkDeleted(ctx, id, time.Now())
			require.NoError(t, err)
			require.NoError(t, store.Delete(ctx, keyOf(id)))
		}
	})

	t.Run("marking a recording deleted happens once and is harmless to repeat", func(t *testing.T) {
		r := dictate(ago(time.Hour))
		changed, err := repo.MarkDeleted(ctx, r.RecordingID, time.Now())
		require.NoError(t, err)
		assert.True(t, changed)
		changed, err = repo.MarkDeleted(ctx, r.RecordingID, time.Now())
		require.NoError(t, err)
		assert.False(t, changed)
		s, at := status(r.RecordingID)
		assert.Equal(t, "DELETED", s)
		assert.NotNil(t, at)

		keys, err := repo.StoredKeys(ctx)
		require.NoError(t, err)
		assert.NotContains(t, keys, keyOf(r.RecordingID), "a deleted recording's audio is not expected to exist")
	})

	t.Run("only one instance cleans at a time", func(t *testing.T) {
		release, ok, err := repo.TryLock(ctx)
		require.NoError(t, err)
		require.True(t, ok)

		_, second, err := repo.TryLock(ctx)
		require.NoError(t, err)
		assert.False(t, second, "a second instance must wait its turn")

		release()
		release2, again, err := repo.TryLock(ctx)
		require.NoError(t, err)
		assert.True(t, again, "the lock is free once the first run is done")
		release2()
	})

	t.Run("a purge removes the audio but keeps the transcript, the answer and the history", func(t *testing.T) {
		kept := dictate(nil)
		expired := dictate(ago(time.Hour))
		answers := answer.NewRepository(testDB.Pool)
		require.NoError(t, answers.Upsert(ctx, &answer.Answer{
			SessionID: sessionID, QuestionID: essay, TextAnswer: "plants make sugar",
			Revision: &answer.RevisionInput{
				Source: answer.SourceSTTConfirmed, TranscriptionID: &expired.TranscriptionID,
				ConfirmationID: "janitor-1", RecordingIDs: []uuid.UUID{expired.RecordingID},
			},
		}))
		before := reviewSeed{pool: testDB.Pool, ctx: ctx, t: t}.snapshot()
		purgedBefore := auditRows("audio.purged")

		j := recording.NewJanitor(repo, store, audit.NewRecorder(testDB.Pool), recording.JanitorOptions{}, &nop)
		report, err := j.RunOnce(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, report.Purged)

		assert.False(t, hasObject(keyOf(expired.RecordingID)), "the audio is gone")
		s, _ := status(expired.RecordingID)
		assert.Equal(t, "DELETED", s)
		assert.True(t, hasObject(keyOf(kept.RecordingID)), "a recording with no retention date is untouched")

		var text string
		require.NoError(t, testDB.Pool.QueryRow(ctx,
			`SELECT text FROM transcriptions WHERE recording_id = $1`, expired.RecordingID).Scan(&text))
		assert.Equal(t, "plants make sugar", text, "the transcript outlives the audio")
		assert.Equal(t, before, reviewSeed{pool: testDB.Pool, ctx: ctx, t: t}.snapshot(), "answers and sessions are untouched")

		assert.Equal(t, purgedBefore+1, auditRows("audio.purged"))
		var actor *uuid.UUID
		var role string
		var resource uuid.UUID
		var meta string
		require.NoError(t, testDB.Pool.QueryRow(ctx, `
			SELECT actor_id, actor_role, resource_id, metadata::text FROM audit_events
			WHERE action = 'audio.purged' ORDER BY id DESC LIMIT 1`).Scan(&actor, &role, &resource, &meta))
		assert.Nil(t, actor, "the system did it, not a person")
		assert.Equal(t, "SYSTEM", role)
		assert.Equal(t, expired.RecordingID, resource)
		assert.JSONEq(t, `{"reason":"retention_expired"}`, meta)

		// What a teacher sees afterwards.
		rv := review.NewRepository(testDB.Pool)
		got, err := rv.GetResponses(ctx, examID, student, true)
		require.NoError(t, err)
		var removed *review.RecordingRecord
		for i, rec := range got.Questions[0].Recordings {
			if rec.ID == expired.RecordingID {
				removed = &got.Questions[0].Recordings[i]
			}
		}
		require.NotNil(t, removed)
		assert.Equal(t, review.RecordingRemoved, removed.Status)
		assert.Equal(t, "plants make sugar", removed.Transcript)
		file, err := rv.GetRecording(ctx, examID, student, expired.RecordingID)
		require.NoError(t, err)
		assert.True(t, file.Removed)
	})

	t.Run("two instances at once purge each recording exactly once", func(t *testing.T) {
		const n = 12
		for i := 0; i < n; i++ {
			dictate(ago(time.Hour))
		}
		purgedBefore := auditRows("audio.purged")

		auditor := audit.NewRecorder(testDB.Pool)
		var wg sync.WaitGroup
		reports := make([]recording.Report, 4)
		for i := range reports {
			wg.Add(1)
			go func() {
				defer wg.Done()
				j := recording.NewJanitor(repo, store, auditor, recording.JanitorOptions{BatchSize: 5}, &nop)
				r, err := j.RunOnce(ctx)
				assert.NoError(t, err)
				reports[i] = r
			}()
		}
		wg.Wait()

		total := 0
		for _, r := range reports {
			total += r.Purged
		}
		assert.Equal(t, n, total, "every recording is purged by someone")
		assert.Equal(t, purgedBefore+n, auditRows("audio.purged"), "and recorded once, not once per instance")
	})

	t.Run("orphaned audio is reported, and removed only when asked", func(t *testing.T) {
		orphan := audiostore.NewKey()
		require.NoError(t, store.Put(ctx, orphan, bytes.NewReader([]byte("lost")), 4))
		store.SetModified(orphan, time.Now().Add(-48*time.Hour))
		stranger := audiostore.NewKey()
		require.NoError(t, store.Put(ctx, stranger, bytes.NewReader([]byte("new")), 3)) // just written

		j := recording.NewJanitor(repo, store, audit.NewRecorder(testDB.Pool), recording.JanitorOptions{}, &nop)
		report, err := j.RunOnce(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, report.Orphans)
		assert.True(t, hasObject(orphan), "reporting does not delete")
		assert.Zero(t, report.MissingAudio, "nothing that should exist is missing")

		j = recording.NewJanitor(repo, store, audit.NewRecorder(testDB.Pool), recording.JanitorOptions{DeleteOrphans: true}, &nop)
		report, err = j.RunOnce(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, report.OrphansRemoved)
		assert.False(t, hasObject(orphan))
		assert.True(t, hasObject(stranger), "audio that may still be being saved is left alone")
		assert.Equal(t, 1, auditRows("audio.orphan_removed"))
	})

	t.Run("a dictation the student never confirmed is deleted after the grace period; a confirmed one is not", func(t *testing.T) {
		answers := answer.NewRepository(testDB.Pool)
		madeAgo := func(id uuid.UUID, d time.Duration) {
			_, err := testDB.Pool.Exec(ctx, `UPDATE audio_recordings SET recorded_at = $2 WHERE id = $1`, id, time.Now().Add(-d))
			require.NoError(t, err)
		}
		denied := dictate(nil) // "no, that is wrong"
		madeAgo(denied.RecordingID, 3*time.Hour)
		fresh := dictate(nil)    // made a moment ago; the student may still confirm it
		approved := dictate(nil) // confirmed below
		madeAgo(approved.RecordingID, 3*time.Hour)
		require.NoError(t, answers.Upsert(ctx, &answer.Answer{
			SessionID: sessionID, QuestionID: essay, TextAnswer: "approved text",
			Revision: &answer.RevisionInput{
				Source: answer.SourceSTTConfirmed, TranscriptionID: &approved.TranscriptionID,
				ConfirmationID: "unconfirmed-test", RecordingIDs: []uuid.UUID{approved.RecordingID},
			},
		}))

		due, err := repo.Unconfirmed(ctx, time.Now().Add(-time.Hour), 100)
		require.NoError(t, err)
		ids := map[uuid.UUID]bool{}
		for _, d := range due {
			ids[d.ID] = true
		}
		assert.True(t, ids[denied.RecordingID])
		assert.False(t, ids[fresh.RecordingID], "not past the grace period")
		assert.False(t, ids[approved.RecordingID], "it supports a confirmed answer")

		j := recording.NewJanitor(repo, store, audit.NewRecorder(testDB.Pool), recording.JanitorOptions{}, &nop)
		report, err := j.RunOnce(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, report.PurgedUnconfirmed)
		assert.False(t, hasObject(keyOf(denied.RecordingID)))
		assert.True(t, hasObject(keyOf(fresh.RecordingID)))
		assert.True(t, hasObject(keyOf(approved.RecordingID)), "the audio behind the saved answer stays")
		s1, _ := status(denied.RecordingID)
		s2, _ := status(approved.RecordingID)
		assert.Equal(t, "DELETED", s1)
		assert.Equal(t, "STORED", s2)

		var reason string
		require.NoError(t, testDB.Pool.QueryRow(ctx, `
			SELECT metadata->>'reason' FROM audit_events WHERE action = 'audio.purged' AND resource_id = $1`,
			denied.RecordingID).Scan(&reason))
		assert.Equal(t, "never_confirmed", reason)
	})

	t.Run("a recording whose audio is gone can no longer be attached to an answer", func(t *testing.T) {
		gone := dictate(nil)
		_, err := repo.MarkDeleted(ctx, gone.RecordingID, time.Now())
		require.NoError(t, err)

		require.NoError(t, answer.NewRepository(testDB.Pool).Upsert(ctx, &answer.Answer{
			SessionID: sessionID, QuestionID: essay, TextAnswer: "late confirmation",
			Revision: &answer.RevisionInput{
				Source: answer.SourceSTTConfirmed, TranscriptionID: &gone.TranscriptionID,
				ConfirmationID: "after-purge", RecordingIDs: []uuid.UUID{gone.RecordingID},
			},
		}))
		var linked *uuid.UUID
		require.NoError(t, testDB.Pool.QueryRow(ctx,
			`SELECT answer_revision_id FROM audio_recordings WHERE id = $1`, gone.RecordingID).Scan(&linked))
		assert.Nil(t, linked, "a revision must not claim audio that no longer exists")
	})

	t.Run("a stream id reused for another question is refused, not answered with the old recording", func(t *testing.T) {
		other := seed.question(examID, 9, "ESSAY", "Another", 1)
		stream := uuid.New()
		in := recording.SaveInput{
			SessionID: sessionID, QuestionID: essay, StudentID: student, StreamID: stream,
			SampleRate: 16000, ChunkCount: 1, PCM: bytes.Repeat([]byte{4, 0}, 800), Transcript: "first",
		}
		first, err := recSvc.Save(ctx, in)
		require.NoError(t, err)

		before := len(store.Keys())
		in.QuestionID, in.Transcript = other, "second"
		_, err = recSvc.Save(ctx, in)
		assert.ErrorIs(t, err, recording.ErrStreamReused)
		assert.Equal(t, before, len(store.Keys()), "the new audio is not left in the store")

		var q uuid.UUID
		require.NoError(t, testDB.Pool.QueryRow(ctx,
			`SELECT question_id FROM audio_recordings WHERE id = $1`, first.RecordingID).Scan(&q))
		assert.Equal(t, essay, q, "the first recording is exactly as it was")
	})
}
