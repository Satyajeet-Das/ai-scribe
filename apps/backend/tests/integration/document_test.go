package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/document"
	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract/extracttest"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/pdf/pdftest"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
	"github.com/Satyajeet-Das/ai-scribe/migrations"
)

// setupDocumentDB returns a migrated database. It uses the Docker containers of
// SetupTestDB, unless SCRIBE_TEST_PG_ADDR (host:port of a running Postgres, with
// SCRIBE_TEST_PG_USER and optionally SCRIBE_TEST_PG_PASSWORD) is set: then a fresh
// database is made on that server for the test and dropped afterwards. The
// document tests need no Redis, so the override makes them runnable without Docker.
func setupDocumentDB(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	addr := os.Getenv("SCRIBE_TEST_PG_ADDR")
	if addr == "" {
		db, cleanup := SetupTestDB(t)
		return db.Pool, cleanup
	}
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	user, pass := os.Getenv("SCRIBE_TEST_PG_USER"), os.Getenv("SCRIBE_TEST_PG_PASSWORD")
	if user == "" {
		user = "postgres"
	}

	ctx := context.Background()
	admin, err := pgx.Connect(ctx, fmt.Sprintf("postgres://%s:%s@%s/postgres?sslmode=disable", user, pass, addr))
	require.NoError(t, err)
	name := "doc_test_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	_, err = admin.Exec(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)

	cfg := &config.Config{
		Database: config.DatabaseConfig{Host: host, Port: port, User: user, Password: pass, Name: name, SSLMode: "disable",
			MaxOpenConns: 25, MaxIdleConns: 25, ConnMaxLifetime: 300, ConnMaxIdleTime: 300},
		Primary: config.Primary{Env: "test"},
	}
	nop := zerolog.Nop()
	db, err := database.New(cfg, &nop, nil)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(ctx, &nop, cfg), "migrations must apply to an empty database")

	return db.Pool, func() {
		db.Pool.Close()
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		_ = admin.Close(ctx)
	}
}

type docQueue struct {
	mu   sync.Mutex
	jobs []struct {
		ID      uuid.UUID
		Attempt int
	}
}

func (q *docQueue) EnqueueExtract(id uuid.UUID, attempt int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, struct {
		ID      uuid.UUID
		Attempt int
	}{id, attempt})
	return nil
}

type docEnv struct {
	t        *testing.T
	ctx      context.Context
	pool     *pgxpool.Pool
	repo     document.Repository
	svc      document.Service
	exams    exam.Service
	model    *extracttest.Model
	store    *audiostore.Memory
	queue    *docQueue
	teacher  document.Caller
	other    document.Caller
	examID   uuid.UUID
	importer document.QuestionImporter
}

// failingImporter writes the first n questions through the real importer and
// then fails, as a database that goes away part-way through an approval would.
type failingImporter struct {
	real  document.QuestionImporter
	after int
}

func (f failingImporter) ImportQuestions(ctx context.Context, tx database.DBTX, examID uuid.UUID, items []question.ImportItem) ([]question.Question, error) {
	if _, err := f.real.ImportQuestions(ctx, tx, examID, items[:f.after]); err != nil {
		return nil, err
	}
	return nil, errors.New("connection lost part-way through the import")
}

const (
	dq1 = "What is the SI unit of force?"
	dq2 = "Explain Newton's first law of motion."
)

func docPDF(extra ...string) []byte {
	texts := []pdftest.Text{
		pdftest.Line(72, 720, "1. "+dq1), pdftest.Line(90, 700, "A. Newton"), pdftest.Line(90, 685, "B. Joule"),
		pdftest.Line(90, 670, "C. Watt"), pdftest.Line(72, 630, "2. "+dq2+" [5 marks]"),
	}
	for i, e := range extra {
		texts = append(texts, pdftest.Line(72, 600-float64(i)*15, e))
	}
	return pdftest.Build([]pdftest.Page{{Texts: texts}})
}

func newDocEnv(t *testing.T, wrap ...func(document.QuestionImporter) document.QuestionImporter) *docEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	pool, cleanup := setupDocumentDB(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	nop := zerolog.Nop()
	seed := reviewSeed{pool: pool, ctx: ctx, t: t}
	teacherID := seed.user("TEACHER", "Terry", "Teacher", "")
	otherID := seed.user("TEACHER", "Olive", "Other", "")

	db := &database.Database{Pool: pool}
	examSvc := exam.NewService(exam.NewRepository(pool), &nop)
	qSvc := question.NewService(question.NewRepository(pool), examSvc, db, &nop)
	var importer document.QuestionImporter = qSvc
	for _, w := range wrap {
		importer = w(importer)
	}

	env := &docEnv{
		t: t, ctx: ctx, pool: pool, repo: document.NewRepository(pool), exams: examSvc,
		model: &extracttest.Model{Replies: []extracttest.Reply{{Body: extracttest.ReplyJSON(
			extracttest.MCQ("1", dq1, 1, "Newton", "Joule", "Watt"), extracttest.Essay("2", dq2, 1))}}},
		store: audiostore.NewMemory(audiostore.WithKind(audiostore.KindPDF)), queue: &docQueue{},
		teacher: document.Caller{ID: teacherID, Role: platformauth.RoleTeacher},
		other:   document.Caller{ID: otherID, Role: platformauth.RoleTeacher},
		examID:  draftExam(t, seed, teacherID), importer: importer,
	}
	env.svc = document.NewService(document.Deps{
		Repo: env.repo, Exams: examSvc, Questions: importer, Tx: db, Store: env.store,
		Extractor: extract.New(env.model, extract.Options{}), Queue: env.queue,
		Options: document.Options{MaxBytes: 5 << 20, MaxPages: 10, Lease: 3 * time.Second, MaxConcurrent: 2, Active: true},
		Logger:  &nop,
	})
	return env
}

func (e *docEnv) upload(pdf ...[]byte) *document.Document {
	e.t.Helper()
	data := docPDF()
	if len(pdf) > 0 {
		data = pdf[0]
	}
	d, err := e.svc.Upload(e.ctx, e.examID, "paper.pdf", bytes.NewReader(data), e.teacher)
	require.NoError(e.t, err)
	return d
}

func (e *docEnv) processed() *document.Document {
	e.t.Helper()
	d := e.upload()
	require.NoError(e.t, e.svc.Process(e.ctx, d.ID, d.Attempt, false))
	got, err := e.svc.Get(e.ctx, e.examID, d.ID, e.teacher)
	require.NoError(e.t, err)
	require.Equal(e.t, document.StatusReviewRequired, got.Status)
	return got
}

// makeApprovable sets marks on every draft question, as a teacher would.
func (e *docEnv) makeApprovable(docID uuid.UUID) int {
	e.t.Helper()
	draft, err := e.svc.GetDraft(e.ctx, e.examID, docID, e.teacher)
	require.NoError(e.t, err)
	v := draft.Document.DraftVersion
	for _, q := range draft.Questions {
		in := document.QuestionInput{NumberLabel: q.NumberLabel, Type: q.Type, Text: q.Text, Points: ptrInt(3)}
		for _, o := range q.Options {
			in.Options = append(in.Options, document.OptionInput{Label: o.Label, Text: o.Text, IsCorrect: o.IsCorrect})
		}
		res, err := e.svc.ReplaceQuestion(e.ctx, e.examID, docID, q.ID, document.ReplaceQuestionRequest{ExpectedDraftVersion: v, Question: in}, e.teacher)
		require.NoError(e.t, err)
		v = res.DraftVersion
	}
	return v
}

func ptrInt(v int) *int { return &v }

func (e *docEnv) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	require.NoError(e.t, e.pool.QueryRow(e.ctx, sql, args...).Scan(&n))
	return n
}

// ---- schema ----

func TestDocumentSchema_Constraints(t *testing.T) {
	e := newDocEnv(t)
	d := e.upload()

	exec := func(sql string, args ...any) error {
		_, err := e.pool.Exec(e.ctx, sql, args...)
		return err
	}

	t.Run("a completed document must say who approved it and when", func(t *testing.T) {
		assert.Error(t, exec(`UPDATE exam_documents SET status = 'COMPLETED' WHERE id = $1`, d.ID))
	})
	t.Run("a failed document must say why", func(t *testing.T) {
		assert.Error(t, exec(`UPDATE exam_documents SET status = 'FAILED' WHERE id = $1`, d.ID))
	})
	t.Run("a document that is not failed carries no failure code", func(t *testing.T) {
		assert.Error(t, exec(`UPDATE exam_documents SET failure_code = 'X' WHERE id = $1`, d.ID))
	})
	t.Run("unknown statuses and stages are refused", func(t *testing.T) {
		assert.Error(t, exec(`UPDATE exam_documents SET status = 'PUBLISHED' WHERE id = $1`, d.ID))
		assert.Error(t, exec(`UPDATE exam_documents SET stage = 'DANCING' WHERE id = $1`, d.ID))
	})
	t.Run("the same file cannot be live twice in one exam, but can be again once it failed", func(t *testing.T) {
		second := *d
		second.ID, second.StorageKey = uuid.New(), audiostore.NewKeyFor(audiostore.KindPDF)
		err := e.repo.Insert(e.ctx, &second)
		var dup *document.DuplicateError
		require.ErrorAs(t, err, &dup)
		assert.Equal(t, d.ID, dup.ExistingID)

		ok, err := e.repo.Fail(e.ctx, d.ID, 1, "X", "failed")
		require.NoError(t, err)
		require.True(t, ok)
		require.NoError(t, e.repo.Insert(e.ctx, &second), "a failed upload does not block the same file")
	})
	t.Run("draft question types and confidence are constrained", func(t *testing.T) {
		assert.Error(t, exec(`INSERT INTO exam_document_questions (document_id, position, type, text) VALUES ($1, 1, 'VOICE', 'x')`, d.ID))
		assert.Error(t, exec(`INSERT INTO exam_document_questions (document_id, position, type, text, confidence) VALUES ($1, 1, 'ESSAY', 'x', 'CERTAIN')`, d.ID))
		assert.Error(t, exec(`INSERT INTO exam_document_questions (document_id, position, type, text, points) VALUES ($1, 1, 'ESSAY', 'x', -1)`, d.ID))
	})
	t.Run("a document needs an exam and a teacher that exist", func(t *testing.T) {
		bad := *d
		bad.ID, bad.StorageKey, bad.SHA256, bad.ExamID = uuid.New(), audiostore.NewKeyFor(audiostore.KindPDF), strings.Repeat("a", 64), uuid.New()
		assert.Error(t, e.repo.Insert(e.ctx, &bad))
	})
}

func TestDocumentSchema_DraftPositionsCanBeSwappedInOneStatement(t *testing.T) {
	e := newDocEnv(t)
	d := e.processed()
	draft, err := e.svc.GetDraft(e.ctx, e.examID, d.ID, e.teacher)
	require.NoError(t, err)
	a, b := draft.Questions[0].ID, draft.Questions[1].ID

	require.NoError(t, e.repo.SetOrder(e.ctx, d.ID, []uuid.UUID{b, a}), "positions swap without tripping the unique position")
	got, err := e.repo.ListDraft(e.ctx, d.ID)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{b, a}, []uuid.UUID{got[0].ID, got[1].ID})

	_, err = e.pool.Exec(e.ctx, `UPDATE exam_document_questions SET position = 1 WHERE id = $1`, a)
	assert.Error(t, err, "but two questions still cannot share a position")
}

func TestDocumentSchema_DeletingTheExamOrDocumentCascadesCorrectly(t *testing.T) {
	e := newDocEnv(t)
	d := e.processed()
	e.makeApprovable(d.ID)
	cur, _ := e.svc.Get(e.ctx, e.examID, d.ID, e.teacher)
	_, err := e.svc.Approve(e.ctx, e.examID, d.ID, cur.DraftVersion, e.teacher)
	require.NoError(t, err)
	require.Equal(t, 2, e.count(`SELECT COUNT(*) FROM questions WHERE source_document_id = $1`, d.ID))

	require.NoError(t, e.svc.Delete(e.ctx, e.examID, d.ID, e.teacher))
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM exam_documents WHERE id = $1`, d.ID))
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM exam_document_questions WHERE document_id = $1`, d.ID), "the draft goes with its document")
	assert.Equal(t, 2, e.count(`SELECT COUNT(*) FROM questions WHERE exam_id = $1`, e.examID), "the exam keeps its questions")
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM questions WHERE exam_id = $1 AND source_document_id IS NOT NULL`, e.examID), "only the link is cleared")
	assert.Equal(t, 2, e.count(`SELECT COUNT(*) FROM questions WHERE exam_id = $1 AND source_draft_question_id IS NOT NULL`, e.examID),
		"the draft id stays, so the same draft can never be imported twice")

	d2 := e.upload(docPDF("another paper"))
	_, err = e.pool.Exec(e.ctx, `DELETE FROM exams WHERE id = $1`, e.examID)
	// Deleting an exam is restricted elsewhere (sessions/assignments); with none it cascades.
	require.NoError(t, err)
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM exam_documents WHERE id = $1`, d2.ID), "an exam's documents go with it")
}

// ---- persistence ----

func TestDocumentPipeline_DraftRoundTripsThroughPostgres(t *testing.T) {
	e := newDocEnv(t)
	body := `{"documentInstructions": "Answer ALL questions.", "questions": [
	  {"numberLabel": "1", "type": "MCQ", "instructions": "Choose one.", "text": "` + dq1 + `", "marks": null,
	   "options": [{"label": "(a)", "text": "Newton"}, {"label": "B.", "text": "Joule"}, {"label": "C", "text": "Watt"}],
	   "tables": [{"page": 1, "markdown": "| a | b |\n|---|---|\n| 1 | 2 |"}],
	   "diagrams": [{"page": 1, "region": "right", "description": "A force arrow on a box."}],
	   "pages": [1], "continuesPrevious": false, "uncertain": true},
	  {"numberLabel": "2", "type": "ESSAY", "instructions": null, "text": "` + dq2 + `", "marks": 5, "options": [], "tables": [], "diagrams": [],
	   "pages": [1], "continuesPrevious": false, "uncertain": false}],
	  "unassigned": [{"page": 1, "text": "Formula: F = ma", "reason": "Not a question"}]}`
	e.model.Replies = []extracttest.Reply{{Body: body}}

	d := e.upload(docPDF("Choose one."))
	require.Equal(t, 1, d.ProgressTotal)
	require.NoError(t, e.svc.Process(e.ctx, d.ID, 1, false))

	got, err := e.repo.Get(e.ctx, d.ID)
	require.NoError(t, err)
	assert.Equal(t, document.StatusReviewRequired, got.Status)
	assert.Equal(t, 1, got.DraftVersion)
	assert.Nil(t, got.LeaseExpires)
	assert.Equal(t, "Answer ALL questions.", got.DocumentInstructions)
	require.Len(t, got.Unassigned, 1)
	assert.Equal(t, "Formula: F = ma", got.Unassigned[0].Text)
	require.Len(t, got.PageSummary, 1)
	assert.Equal(t, "TEXT", got.PageSummary[0].Kind)
	assert.Equal(t, "fake", got.Provider)

	qs, err := e.repo.ListDraft(e.ctx, d.ID)
	require.NoError(t, err)
	require.Len(t, qs, 2)
	q := qs[0]
	assert.Equal(t, 1, q.Position)
	assert.Equal(t, "Choose one.", q.Instructions)
	assert.Equal(t, []int{1}, q.SourcePages, "int[] round trips")
	assert.Equal(t, []string{"A", "B", "C"}, []string{q.Options[0].Label, q.Options[1].Label, q.Options[2].Label})
	assert.Equal(t, "| a | b |\n|---|---|\n| 1 | 2 |", q.Tables[0].Markdown)
	assert.Equal(t, "MODEL", q.Diagrams[0].DescriptionSource)
	assert.False(t, q.Diagrams[0].Confirmed)
	assert.Equal(t, extract.ConfidenceMedium, q.Confidence)
	require.NotEmpty(t, q.Warnings)
	assert.Equal(t, document.OriginExtracted, q.Origin)
	assert.Nil(t, q.Points)
	require.NotNil(t, qs[1].Points)
	assert.Equal(t, 5, *qs[1].Points, "marks printed on the paper are kept")
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM questions WHERE exam_id = $1`, e.examID), "the draft is not the exam")
}

func TestDocumentPipeline_RetryReplacesTheDraft(t *testing.T) {
	e := newDocEnv(t)
	d := e.processed()
	cur, _ := e.svc.Get(e.ctx, e.examID, d.ID, e.teacher)

	_, err := e.svc.Retry(e.ctx, e.examID, d.ID, document.RetryRequest{DiscardDraft: true, ExpectedDraftVersion: cur.DraftVersion}, e.teacher)
	require.NoError(t, err)
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM exam_document_questions WHERE document_id = $1`, d.ID), "the old draft is gone at once")
	require.NoError(t, e.svc.Process(e.ctx, d.ID, 2, false))
	assert.Equal(t, 2, e.count(`SELECT COUNT(*) FROM exam_document_questions WHERE document_id = $1`, d.ID), "and the new one is the only one")
}

// ---- job safety ----

func TestDocumentJobs_OnlyOneWorkerWinsTheClaim(t *testing.T) {
	e := newDocEnv(t)
	d := e.upload()

	var claimed, held, stale int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, _, err := e.repo.Claim(e.ctx, d.ID, 1, 30*time.Second)
			assert.NoError(t, err)
			switch res {
			case document.Claimed:
				atomic.AddInt32(&claimed, 1)
			case document.ClaimLeaseHeld:
				atomic.AddInt32(&held, 1)
			default:
				atomic.AddInt32(&stale, 1)
			}
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, claimed, "exactly one worker owns the document")
	assert.EqualValues(t, 11, held)
	assert.EqualValues(t, 0, stale)
}

func TestDocumentJobs_AnExpiredLeaseCanBeTakenOverAndAStaleWorkerCannotWrite(t *testing.T) {
	e := newDocEnv(t)
	d := e.upload()

	res, _, err := e.repo.Claim(e.ctx, d.ID, 1, 1*time.Second)
	require.NoError(t, err)
	require.Equal(t, document.Claimed, res)

	res, _, _ = e.repo.Claim(e.ctx, d.ID, 1, time.Second)
	assert.Equal(t, document.ClaimLeaseHeld, res, "while the lease is live")

	time.Sleep(1300 * time.Millisecond)
	res, _, err = e.repo.Claim(e.ctx, d.ID, 1, 30*time.Second)
	require.NoError(t, err)
	assert.Equal(t, document.Claimed, res, "once it has run out, another worker takes over")

	// The document is restarted as attempt 2 by the teacher; the worker that
	// held attempt 1 now tries to save its result.
	_, err = e.repo.Cancel(e.ctx, d.ID)
	require.NoError(t, err)
	_, err = e.svc.Retry(e.ctx, e.examID, d.ID, document.RetryRequest{}, e.teacher)
	require.NoError(t, err)
	ok, err := e.repo.SaveDraft(e.ctx, d.ID, 1, document.DraftData{Questions: []document.DraftQuestion{{Type: "ESSAY", Text: "stale", Confidence: "HIGH", Origin: "EXTRACTED"}}})
	require.NoError(t, err)
	assert.False(t, ok, "a stale attempt writes nothing")
	for name, f := range map[string]func() (bool, error){
		"progress":  func() (bool, error) { return e.repo.SetProgress(e.ctx, d.ID, 1, document.StageExtracting, 1, 1) },
		"heartbeat": func() (bool, error) { return e.repo.Heartbeat(e.ctx, d.ID, 1, time.Minute) },
		"fail":      func() (bool, error) { return e.repo.Fail(e.ctx, d.ID, 1, "X", "x") },
		"requeue":   func() (bool, error) { return e.repo.Requeue(e.ctx, d.ID, 1) },
	} {
		ok, err := f()
		require.NoError(t, err, name)
		assert.False(t, ok, name)
	}
	got, _ := e.repo.Get(e.ctx, d.ID)
	assert.Equal(t, document.StatusQueued, got.Status)
	assert.Equal(t, 2, got.Attempt)
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM exam_document_questions WHERE document_id = $1`, d.ID))
}

func TestDocumentJobs_CancellingStopsARunningJob(t *testing.T) {
	e := newDocEnv(t)
	d := e.upload()
	e.model.Block = make(chan struct{}) // only cancellation can end the call
	result := make(chan error, 1)
	go func() { result <- e.svc.Process(e.ctx, d.ID, 1, false) }()
	require.Eventually(t, func() bool { return e.model.Calls() == 1 }, 10*time.Second, 20*time.Millisecond)

	_, err := e.svc.Cancel(e.ctx, e.examID, d.ID, e.teacher)
	require.NoError(t, err)
	select {
	case err := <-result:
		assert.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("the job kept running after the document was cancelled")
	}
	got, _ := e.repo.Get(e.ctx, d.ID)
	assert.Equal(t, document.StatusCancelled, got.Status)
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM exam_document_questions WHERE document_id = $1`, d.ID))
}

// ---- review ----

func TestDocumentDraft_ConcurrentEditsOnOneVersionLetOneWin(t *testing.T) {
	e := newDocEnv(t)
	d := e.processed()
	draft, _ := e.svc.GetDraft(e.ctx, e.examID, d.ID, e.teacher)
	qid := draft.Questions[1].ID

	var wins, stale int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := e.svc.ReplaceQuestion(e.ctx, e.examID, d.ID, qid, document.ReplaceQuestionRequest{
				ExpectedDraftVersion: d.DraftVersion,
				Question:             document.QuestionInput{Type: "ESSAY", Text: fmt.Sprintf("edit number %d", i), Points: ptrInt(i)},
			}, e.teacher)
			switch {
			case err == nil:
				atomic.AddInt32(&wins, 1)
			case errors.Is(err, document.ErrStaleVersion):
				atomic.AddInt32(&stale, 1)
			default:
				t.Errorf("unexpected: %v", err)
			}
		}(i)
	}
	wg.Wait()
	assert.EqualValues(t, 1, wins)
	assert.EqualValues(t, 9, stale)
	got, _ := e.repo.Get(e.ctx, d.ID)
	assert.Equal(t, d.DraftVersion+1, got.DraftVersion, "the version rose exactly once")
}

func TestDocumentDraft_AddDeleteAndReorderKeepPositionsContiguous(t *testing.T) {
	e := newDocEnv(t)
	d := e.processed()
	v := d.DraftVersion
	add := func(pos int, text string) document.DraftQuestion {
		r, err := e.svc.AddQuestion(e.ctx, e.examID, d.ID, document.AddQuestionRequest{ExpectedDraftVersion: v, Position: pos,
			Question: document.QuestionInput{Type: "ESSAY", Text: text, Points: ptrInt(1)}}, e.teacher)
		require.NoError(t, err)
		v = r.DraftVersion
		return r.Question
	}
	front := add(1, "inserted at the front")
	_ = add(0, "appended at the end")
	_ = add(3, "inserted in the middle")

	draft, _ := e.svc.GetDraft(e.ctx, e.examID, d.ID, e.teacher)
	var positions []int
	for _, q := range draft.Questions {
		positions = append(positions, q.Position)
	}
	assert.Equal(t, []int{1, 2, 3, 4, 5}, positions)
	assert.Equal(t, "inserted at the front", draft.Questions[0].Text)
	assert.Equal(t, "inserted in the middle", draft.Questions[2].Text)

	after, err := e.svc.DeleteQuestion(e.ctx, e.examID, d.ID, front.ID, v, e.teacher)
	require.NoError(t, err)
	positions = nil
	for _, q := range after.Questions {
		positions = append(positions, q.Position)
	}
	assert.Equal(t, []int{1, 2, 3, 4}, positions, "no gap is left")
	v = after.Document.DraftVersion

	ids := []uuid.UUID{}
	for i := len(after.Questions) - 1; i >= 0; i-- {
		ids = append(ids, after.Questions[i].ID)
	}
	rev, err := e.svc.Reorder(e.ctx, e.examID, d.ID, document.ReorderRequest{ExpectedDraftVersion: v, QuestionIDs: ids}, e.teacher)
	require.NoError(t, err)
	for i, q := range rev.Questions {
		assert.Equal(t, ids[i], q.ID)
		assert.Equal(t, i+1, q.Position)
	}
}

// ---- approval ----

func TestDocumentApprove_CreatesQuestionsOnceAndLeavesTheExamADraft(t *testing.T) {
	e := newDocEnv(t)
	seed := reviewSeed{pool: e.pool, ctx: e.ctx, t: t}
	seed.question(e.examID, 1, "ESSAY", "A question typed by hand", 4)

	d := e.processed()
	v := e.makeApprovable(d.ID)

	var ok, already int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.Approve(e.ctx, e.examID, d.ID, v, e.teacher)
			switch {
			case err == nil:
				atomic.AddInt32(&ok, 1)
			case errors.Is(err, document.ErrAlreadyApproved):
				atomic.AddInt32(&already, 1)
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, ok)
	assert.EqualValues(t, 7, already)

	assert.Equal(t, 3, e.count(`SELECT COUNT(*) FROM questions WHERE exam_id = $1`, e.examID), "the hand-made question plus exactly two imported ones")
	rows, err := e.pool.Query(e.ctx, `SELECT question_number, type, points, source_document_id, source_draft_question_id FROM questions WHERE exam_id = $1 ORDER BY question_number`, e.examID)
	require.NoError(t, err)
	defer rows.Close()
	var nums []int
	for rows.Next() {
		var n, pts int
		var typ string
		var doc, draft *uuid.UUID
		require.NoError(t, rows.Scan(&n, &typ, &pts, &doc, &draft))
		nums = append(nums, n)
		if n > 1 {
			require.NotNil(t, doc)
			assert.Equal(t, d.ID, *doc)
			assert.NotNil(t, draft)
			assert.Equal(t, 3, pts)
		}
	}
	assert.Equal(t, []int{1, 2, 3}, nums, "numbered after the question already there")
	assert.Equal(t, 3, e.count(`SELECT COUNT(*) FROM question_options WHERE question_id IN (SELECT id FROM questions WHERE exam_id = $1 AND type = 'MCQ')`, e.examID))
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM question_options WHERE is_correct`), "no option is marked correct unless a teacher did it")

	var status string
	require.NoError(t, e.pool.QueryRow(e.ctx, `SELECT status FROM exams WHERE id = $1`, e.examID).Scan(&status))
	assert.Equal(t, "DRAFT", status, "approval does not publish")
	var published *time.Time
	require.NoError(t, e.pool.QueryRow(e.ctx, `SELECT published_at FROM exams WHERE id = $1`, e.examID).Scan(&published))
	assert.Nil(t, published)

	// The unique draft id is the backstop: even a second import attempt that got
	// past every check could not create the same question again.
	_, err = e.pool.Exec(e.ctx, `INSERT INTO questions (exam_id, question_number, text, type, points, source_draft_question_id)
		SELECT exam_id, 99, 'dup', type, points, source_draft_question_id FROM questions WHERE exam_id = $1 AND source_draft_question_id IS NOT NULL LIMIT 1`, e.examID)
	assert.Error(t, err)
}

func TestDocumentApprove_IsAllOrNothing(t *testing.T) {
	e := newDocEnv(t, func(real document.QuestionImporter) document.QuestionImporter {
		return failingImporter{real: real, after: 1}
	})
	d := e.processed()
	v := e.makeApprovable(d.ID)

	_, err := e.svc.Approve(e.ctx, e.examID, d.ID, v, e.teacher)
	require.Error(t, err)
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM questions WHERE exam_id = $1`, e.examID), "the question written before the failure was rolled back")
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM question_options`), "and its options")
	got, _ := e.repo.Get(e.ctx, d.ID)
	assert.Equal(t, document.StatusReviewRequired, got.Status, "the document is still waiting, not half-approved")
	assert.Nil(t, got.ApprovedAt)
	assert.Equal(t, v, got.DraftVersion, "and the draft is untouched")
}

func TestDocumentApprove_NeverIntoAnExamPublishedInTheMeantime(t *testing.T) {
	// Run approve and publish against each other repeatedly. Whatever the order,
	// the questions must never land in an exam that is already published.
	e := newDocEnv(t) // one database for every round
	for round := 0; round < 12; round++ {
		e.examID = draftExam(t, reviewSeed{pool: e.pool, ctx: e.ctx, t: t}, e.teacher.ID)
		d := e.processed()
		v := e.makeApprovable(d.ID)

		var approveErr, publishErr error
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, approveErr = e.svc.Approve(e.ctx, e.examID, d.ID, v, e.teacher)
		}()
		go func() {
			defer wg.Done()
			<-start
			_, publishErr = e.exams.PublishExam(e.ctx, e.examID, exam.Caller{ID: e.teacher.ID, Role: platformauth.RoleTeacher})
		}()
		close(start)
		wg.Wait()

		require.NoError(t, publishErr)
		imported := e.count(`SELECT COUNT(*) FROM questions WHERE exam_id = $1 AND source_document_id IS NOT NULL`, e.examID)
		switch {
		case approveErr == nil:
			assert.Equal(t, 2, imported, "round %d: approved before the publish, completely", round)
		case errors.Is(approveErr, document.ErrExamNotDraft):
			assert.Equal(t, 0, imported, "round %d: refused because the exam was published, and nothing was written", round)
		default:
			t.Fatalf("round %d: unexpected error %v", round, approveErr)
		}
		got, _ := e.repo.Get(e.ctx, d.ID)
		if approveErr != nil {
			assert.Equal(t, document.StatusReviewRequired, got.Status)
		}
	}
}

func TestDocumentApprove_ARefusedApprovalChangesNothing(t *testing.T) {
	e := newDocEnv(t)
	d := e.processed() // marks are not set yet
	cur, _ := e.repo.Get(e.ctx, d.ID)

	_, err := e.svc.Approve(e.ctx, e.examID, d.ID, cur.DraftVersion, e.teacher)
	var inc *document.IncompleteError
	require.ErrorAs(t, err, &inc)
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM questions WHERE exam_id = $1`, e.examID))

	_, err = e.svc.Approve(e.ctx, e.examID, d.ID, cur.DraftVersion, e.other)
	assert.ErrorIs(t, err, document.ErrNotFound, "another teacher cannot approve")
}

// draftExam creates a draft exam, which is what a teacher uploads papers to.
// (The shared seed helper makes published exams.)
func draftExam(t *testing.T, seed reviewSeed, owner uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	seed.exec(`INSERT INTO exams (id, title, subject, duration_mins, status, created_by)
	           VALUES ($1, 'Physics', 'Physics', 60, 'DRAFT', $2)`, id, owner)
	return id
}

func TestDocumentSchema_MigrationCanBeRolledBackAndReapplied(t *testing.T) {
	e := newDocEnv(t)
	raw, err := migrations.FS.ReadFile("009_exam_documents.sql")
	require.NoError(t, err)
	up, down, ok := strings.Cut(string(raw), "---- create above / drop below ----")
	require.True(t, ok, "the migration has a rollback section")

	_, err = e.pool.Exec(e.ctx, down)
	require.NoError(t, err, "the rollback applies")
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name IN ('exam_documents', 'exam_document_questions')`))
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'questions' AND column_name LIKE 'source_%'`),
		"the columns added to questions are removed again")
	assert.Equal(t, 1, e.count(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'questions' AND column_name = 'text'`), "questions itself is untouched")

	_, err = e.pool.Exec(e.ctx, up)
	require.NoError(t, err, "and it can be applied again")
	assert.Equal(t, 2, e.count(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name IN ('exam_documents', 'exam_document_questions')`))
	_, err = e.pool.Exec(e.ctx, up)
	assert.NoError(t, err, "applying it twice is harmless")
}

func TestQuestions_HandMadeAndImportedBothReadBackWithProvenance(t *testing.T) {
	e := newDocEnv(t)
	nop := zerolog.Nop()
	qsvc := question.NewService(question.NewRepository(e.pool), e.exams, &database.Database{Pool: e.pool}, &nop)
	qCaller := question.Caller{ID: e.teacher.ID, Role: platformauth.RoleTeacher}

	// A question made by hand, as before this sprint.
	made, err := qsvc.CreateQuestion(e.ctx, e.examID, question.CreateQuestionRequest{
		Text: "A question typed by hand", Type: question.TypeMCQ, Points: 2,
		Options: []question.CreateOptionRequest{{OptionKey: "a", OptionText: "one"}, {OptionKey: "b", OptionText: "two", IsCorrect: true}},
	}, qCaller)
	require.NoError(t, err)
	assert.Nil(t, made.SourceDocumentID)

	d := e.processed()
	v := e.makeApprovable(d.ID)
	_, err = e.svc.Approve(e.ctx, e.examID, d.ID, v, e.teacher)
	require.NoError(t, err)

	list, err := qsvc.ListQuestionsByExamForCaller(e.ctx, e.examID, qCaller)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, []int{1, 2, 3}, []int{list[0].QuestionNumber, list[1].QuestionNumber, list[2].QuestionNumber})
	assert.Nil(t, list[0].SourceDocumentID, "the hand-made question has no source")
	require.NotNil(t, list[1].SourceDocumentID)
	assert.Equal(t, d.ID, *list[1].SourceDocumentID)
	require.NotNil(t, list[1].SourceDraftQuestionID)
	assert.Len(t, list[1].Options, 3)

	one, err := qsvc.GetQuestionForCaller(e.ctx, list[1].ID, qCaller)
	require.NoError(t, err)
	assert.Equal(t, list[1].Text, one.Text)
	assert.Equal(t, d.ID, *one.SourceDocumentID)

	// Once approved, imported questions are ordinary questions: editable by hand.
	newText := "Edited after import"
	_, err = qsvc.UpdateQuestion(e.ctx, one.ID, question.UpdateQuestionRequest{Text: &newText}, qCaller)
	require.NoError(t, err)
}
