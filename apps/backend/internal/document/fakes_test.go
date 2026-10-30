package document

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract/extracttest"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/model"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/pdf/pdftest"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
)

// world is all the state the fakes share: documents, drafts, exams and the
// questions table. A transaction snapshots it and puts it back on failure, as a
// database would, and holds txMu for its whole length, as row locks would.
type world struct {
	mu        sync.Mutex
	txMu      sync.Mutex
	docs      map[uuid.UUID]*Document
	drafts    map[uuid.UUID][]DraftQuestion
	exams     map[uuid.UUID]*exam.Exam
	questions []question.Question

	// Failure injection.
	saveDraftErr   error
	importErr      error
	importFailAt   int // fail while importing the n-th item (1-based); 0 = never
	heartbeatErr   error
	updatesBlocked bool
}

func newWorld() *world {
	return &world{docs: map[uuid.UUID]*Document{}, drafts: map[uuid.UUID][]DraftQuestion{}, exams: map[uuid.UUID]*exam.Exam{}}
}

func cloneDQ(q DraftQuestion) DraftQuestion {
	q.Options = append([]extract.Option(nil), q.Options...)
	q.Tables = append([]extract.Table(nil), q.Tables...)
	q.Diagrams = append([]extract.Diagram(nil), q.Diagrams...)
	q.Warnings = append([]extract.Warning(nil), q.Warnings...)
	q.SourcePages = append([]int(nil), q.SourcePages...)
	if q.Points != nil {
		p := *q.Points
		q.Points = &p
	}
	return q
}

func cloneDoc(d *Document) *Document {
	c := *d
	c.PageSummary = append([]PageInfo(nil), d.PageSummary...)
	c.Unassigned = append([]extract.Unassigned(nil), d.Unassigned...)
	return &c
}

type snapshot struct {
	docs      map[uuid.UUID]*Document
	drafts    map[uuid.UUID][]DraftQuestion
	exams     map[uuid.UUID]exam.Exam
	questions []question.Question
}

func (w *world) snapshot() snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := snapshot{docs: map[uuid.UUID]*Document{}, drafts: map[uuid.UUID][]DraftQuestion{}, exams: map[uuid.UUID]exam.Exam{},
		questions: append([]question.Question(nil), w.questions...)}
	for k, d := range w.docs {
		s.docs[k] = cloneDoc(d)
	}
	for k, qs := range w.drafts {
		for _, q := range qs {
			s.drafts[k] = append(s.drafts[k], cloneDQ(q))
		}
	}
	for k, e := range w.exams {
		s.exams[k] = *e
	}
	return s
}

func (w *world) restore(s snapshot) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.docs, w.drafts, w.questions = s.docs, s.drafts, s.questions
	for k, e := range s.exams {
		c := e
		w.exams[k] = &c
	}
}

// ---- transactions ----

type fakeTx struct{ w *world }

func (f fakeTx) WithTx(_ context.Context, fn func(database.DBTX) error) error {
	f.w.txMu.Lock()
	defer f.w.txMu.Unlock()
	snap := f.w.snapshot()
	if err := fn(nil); err != nil {
		f.w.restore(snap)
		return err
	}
	return nil
}

// ---- repository ----

type fakeRepo struct{ w *world }

func (r fakeRepo) Insert(_ context.Context, d *Document, _ ...database.DBTX) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for _, o := range r.w.docs {
		if o.ExamID == d.ExamID && o.SHA256 == d.SHA256 && o.Status.Live() {
			return &DuplicateError{ExistingID: o.ID}
		}
	}
	r.w.docs[d.ID] = cloneDoc(d)
	return nil
}

func (r fakeRepo) Get(_ context.Context, id uuid.UUID, _ ...database.DBTX) (*Document, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	d, ok := r.w.docs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneDoc(d), nil
}

func (r fakeRepo) GetForUpdate(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (*Document, error) {
	return r.Get(ctx, id, tx...)
}

func (r fakeRepo) List(_ context.Context, examID uuid.UUID) ([]Document, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	out := []Document{}
	for _, d := range r.w.docs {
		if d.ExamID == examID {
			out = append(out, *cloneDoc(d))
		}
	}
	return out, nil
}

func (r fakeRepo) CountLive(_ context.Context, examID uuid.UUID) (int, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	n := 0
	for _, d := range r.w.docs {
		if d.ExamID == examID && d.Status.Live() {
			n++
		}
	}
	return n, nil
}

func (r fakeRepo) Delete(_ context.Context, id uuid.UUID, _ ...database.DBTX) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	delete(r.w.docs, id)
	delete(r.w.drafts, id)
	// ON DELETE SET NULL: approved questions stay, unlinked.
	for i := range r.w.questions {
		if r.w.questions[i].SourceDocumentID != nil && *r.w.questions[i].SourceDocumentID == id {
			r.w.questions[i].SourceDocumentID = nil
		}
	}
	return nil
}

func (r fakeRepo) Claim(_ context.Context, id uuid.UUID, attempt int, lease time.Duration) (ClaimResult, *Document, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	d, ok := r.w.docs[id]
	if !ok {
		return ClaimStale, nil, nil
	}
	expired := d.LeaseExpires != nil && d.LeaseExpires.Before(time.Now())
	if d.Attempt == attempt && (d.Status == StatusQueued || (d.Status == StatusProcessing && expired)) {
		exp := time.Now().Add(lease)
		d.Status, d.Stage, d.ProgressDone, d.ProgressTotal, d.LeaseExpires = StatusProcessing, StageReadingPages, 0, d.PageCount, &exp
		return Claimed, cloneDoc(d), nil
	}
	if d.Attempt == attempt && d.Status == StatusProcessing {
		return ClaimLeaseHeld, cloneDoc(d), nil
	}
	return ClaimStale, cloneDoc(d), nil
}

func (r fakeRepo) owned(id uuid.UUID, attempt int) *Document {
	d, ok := r.w.docs[id]
	if !ok || d.Attempt != attempt || d.Status != StatusProcessing {
		return nil
	}
	return d
}

func (r fakeRepo) Heartbeat(_ context.Context, id uuid.UUID, attempt int, lease time.Duration) (bool, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if r.w.heartbeatErr != nil {
		return false, r.w.heartbeatErr
	}
	d := r.owned(id, attempt)
	if d == nil {
		return false, nil
	}
	exp := time.Now().Add(lease)
	d.LeaseExpires = &exp
	return true, nil
}

func (r fakeRepo) SetProgress(_ context.Context, id uuid.UUID, attempt int, stage Stage, done, total int) (bool, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	d := r.owned(id, attempt)
	if d == nil {
		return false, nil
	}
	d.Stage, d.ProgressDone, d.ProgressTotal = stage, done, total
	return true, nil
}

func (r fakeRepo) SaveDraft(_ context.Context, id uuid.UUID, attempt int, data DraftData) (bool, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if r.w.saveDraftErr != nil {
		return false, r.w.saveDraftErr
	}
	d := r.owned(id, attempt)
	if d == nil {
		return false, nil
	}
	qs := make([]DraftQuestion, len(data.Questions))
	for i, q := range data.Questions {
		q.DocumentID, q.Position = id, i+1
		qs[i] = cloneDQ(q)
	}
	r.w.drafts[id] = qs
	d.Status, d.Stage, d.LeaseExpires, d.ProgressDone = StatusReviewRequired, "", nil, d.ProgressTotal
	d.Provider, d.PromptVersion, d.PageSummary = data.Provider, data.PromptVersion, data.PageSummary
	d.DocumentInstructions, d.Unassigned = data.DocumentInstructions, data.Unassigned
	d.DraftVersion++
	d.FailureCode, d.FailureMessage = "", ""
	return true, nil
}

func (r fakeRepo) Fail(_ context.Context, id uuid.UUID, attempt int, code, message string) (bool, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	d, ok := r.w.docs[id]
	if !ok || d.Attempt != attempt || (d.Status != StatusQueued && d.Status != StatusProcessing) {
		return false, nil
	}
	d.Status, d.Stage, d.LeaseExpires, d.FailureCode, d.FailureMessage = StatusFailed, "", nil, code, message
	return true, nil
}

func (r fakeRepo) Requeue(_ context.Context, id uuid.UUID, attempt int) (bool, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	d := r.owned(id, attempt)
	if d == nil {
		return false, nil
	}
	d.Status, d.Stage, d.LeaseExpires = StatusQueued, "", nil
	return true, nil
}

func (r fakeRepo) Cancel(_ context.Context, id uuid.UUID) (bool, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	d, ok := r.w.docs[id]
	if !ok || (d.Status != StatusQueued && d.Status != StatusProcessing) {
		return false, nil
	}
	d.Status, d.Stage, d.LeaseExpires = StatusCancelled, "", nil
	return true, nil
}

func (r fakeRepo) Restart(_ context.Context, id uuid.UUID, _ ...database.DBTX) (int, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	d := r.w.docs[id]
	for _, o := range r.w.docs {
		if o.ID != id && o.ExamID == d.ExamID && o.SHA256 == d.SHA256 && o.Status.Live() {
			return 0, &DuplicateError{ExistingID: o.ID}
		}
	}
	delete(r.w.drafts, id)
	d.Status, d.Stage, d.LeaseExpires, d.ProgressDone, d.ProgressTotal = StatusQueued, "", nil, 0, d.PageCount
	d.Attempt++
	d.FailureCode, d.FailureMessage = "", ""
	d.PageSummary, d.DocumentInstructions, d.Unassigned = nil, "", nil
	d.DraftVersion++
	return d.Attempt, nil
}

func (r fakeRepo) ListDraft(_ context.Context, id uuid.UUID, _ ...database.DBTX) ([]DraftQuestion, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	out := []DraftQuestion{}
	for _, q := range r.w.drafts[id] {
		out = append(out, cloneDQ(q))
	}
	return out, nil
}

func (r fakeRepo) GetDraftQuestion(_ context.Context, docID, qid uuid.UUID, _ ...database.DBTX) (*DraftQuestion, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for _, q := range r.w.drafts[docID] {
		if q.ID == qid {
			c := cloneDQ(q)
			return &c, nil
		}
	}
	return nil, ErrNotFound
}

func (r fakeRepo) InsertDraftQuestion(_ context.Context, q *DraftQuestion, _ ...database.DBTX) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for _, e := range r.w.drafts[q.DocumentID] {
		if e.Position == q.Position {
			return errors.New("duplicate position")
		}
	}
	r.w.drafts[q.DocumentID] = append(r.w.drafts[q.DocumentID], cloneDQ(*q))
	r.sortLocked(q.DocumentID)
	return nil
}

func (r fakeRepo) sortLocked(id uuid.UUID) {
	qs := r.w.drafts[id]
	for i := 1; i < len(qs); i++ {
		for j := i; j > 0 && qs[j].Position < qs[j-1].Position; j-- {
			qs[j], qs[j-1] = qs[j-1], qs[j]
		}
	}
}

func (r fakeRepo) ShiftPositions(_ context.Context, id uuid.UUID, from int, _ ...database.DBTX) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for i := range r.w.drafts[id] {
		if r.w.drafts[id][i].Position >= from {
			r.w.drafts[id][i].Position++
		}
	}
	return nil
}

func (r fakeRepo) UpdateDraftQuestion(_ context.Context, q *DraftQuestion, _ ...database.DBTX) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for i, e := range r.w.drafts[q.DocumentID] {
		if e.ID == q.ID {
			c := cloneDQ(*q)
			c.Position, c.SourcePages, c.Confidence, c.Origin = e.Position, e.SourcePages, e.Confidence, e.Origin
			r.w.drafts[q.DocumentID][i] = c
			return nil
		}
	}
	return ErrNotFound
}

func (r fakeRepo) DeleteDraftQuestion(_ context.Context, docID, qid uuid.UUID, _ ...database.DBTX) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	qs := r.w.drafts[docID]
	for i, q := range qs {
		if q.ID == qid {
			qs = append(qs[:i], qs[i+1:]...)
			for j := range qs {
				qs[j].Position = j + 1
			}
			r.w.drafts[docID] = qs
			return nil
		}
	}
	return ErrNotFound
}

func (r fakeRepo) SetOrder(_ context.Context, docID uuid.UUID, ids []uuid.UUID, _ ...database.DBTX) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for pos, id := range ids {
		for i := range r.w.drafts[docID] {
			if r.w.drafts[docID][i].ID == id {
				r.w.drafts[docID][i].Position = pos + 1
			}
		}
	}
	r.sortLocked(docID)
	return nil
}

func (r fakeRepo) BumpDraftVersion(_ context.Context, id uuid.UUID, _ ...database.DBTX) (int, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.docs[id].DraftVersion++
	return r.w.docs[id].DraftVersion, nil
}

func (r fakeRepo) MarkApproved(_ context.Context, id, by uuid.UUID, _ ...database.DBTX) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	now := time.Now()
	d := r.w.docs[id]
	d.Status, d.ApprovedBy, d.ApprovedAt = StatusCompleted, &by, &now
	d.DraftVersion++
	return nil
}

func (r fakeRepo) LockExam(_ context.Context, examID uuid.UUID, _ ...database.DBTX) (*ExamState, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	e, ok := r.w.exams[examID]
	if !ok {
		return nil, ErrNotFound
	}
	return &ExamState{Status: e.Status, CreatedBy: e.CreatedBy, Deleted: e.DeletedAt != nil}, nil
}

// ---- exams ----

type fakeExams struct{ w *world }

func (f fakeExams) GetExamForCaller(_ context.Context, id uuid.UUID, c exam.Caller) (*exam.Exam, error) {
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	e, ok := f.w.exams[id]
	if !ok {
		return nil, exam.ErrExamNotFound
	}
	if c.IsAdmin() {
		c2 := *e
		return &c2, nil
	}
	if c.IsTeacher() {
		if !c.CanManage(e.CreatedBy) {
			return nil, exam.ErrUnauthorizedCreator
		}
		c2 := *e
		return &c2, nil
	}
	return nil, exam.ErrExamNotFound
}

// ---- importer ----

type fakeImporter struct {
	w     *world
	calls int
}

func (f *fakeImporter) ImportQuestions(_ context.Context, _ database.DBTX, examID uuid.UUID, items []question.ImportItem) ([]question.Question, error) {
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	f.calls++
	if f.w.importErr != nil {
		return nil, f.w.importErr
	}
	for _, it := range items {
		if err := it.Validate(); err != nil {
			return nil, err
		}
	}
	maxNum := 0
	for _, q := range f.w.questions {
		if q.ExamID == examID && q.QuestionNumber > maxNum {
			maxNum = q.QuestionNumber
		}
	}
	var created []question.Question
	for i, it := range items {
		if f.w.importFailAt == i+1 {
			return nil, errors.New("database went away")
		}
		doc, draft := it.SourceDocumentID, it.SourceDraftQuestionID
		q := question.Question{
			Base:   model.Base{BaseWithId: model.BaseWithId{ID: uuid.New()}},
			ExamID: examID, QuestionNumber: maxNum + i + 1, Text: it.Text, Type: it.Type, Points: it.Points,
			SourceDocumentID: &doc, SourceDraftQuestionID: &draft,
		}
		for j, o := range it.Options {
			q.Options = append(q.Options, question.QuestionOption{OptionKey: o.Key, OptionText: o.Text, DisplayOrder: j + 1, IsCorrect: o.IsCorrect})
		}
		f.w.questions = append(f.w.questions, q)
		created = append(created, q)
	}
	return created, nil
}

func uniqueViolation() error { return &pgconn.PgError{Code: "23505"} }

// ---- queue ----

type enqueued struct {
	ID      uuid.UUID
	Attempt int
}

type fakeQueue struct {
	mu   sync.Mutex
	jobs []enqueued
	err  error
}

func (q *fakeQueue) EnqueueExtract(id uuid.UUID, attempt int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	q.jobs = append(q.jobs, enqueued{id, attempt})
	return nil
}

func (q *fakeQueue) all() []enqueued {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]enqueued(nil), q.jobs...)
}

// ---- harness ----

type harness struct {
	t           *testing.T
	w           *world
	svc         Service
	store       *failingStore
	model       *extracttest.Model
	queue       *fakeQueue
	importer    *fakeImporter
	opts        Options
	extractOpts extract.Options

	teacher, other, admin, student Caller
	examID                         uuid.UUID
}

// failingStore is an in-memory store whose Put can be made to fail.
type failingStore struct {
	putErr error
	*audiostore.Memory
}

func newHarness(t *testing.T, mutate ...func(*Options)) *harness {
	t.Helper()
	w := newWorld()
	h := &harness{
		t: t, w: w, queue: &fakeQueue{}, importer: &fakeImporter{w: w},
		store:   &failingStore{Memory: audiostore.NewMemory(audiostore.WithKind(audiostore.KindPDF))},
		model:   &extracttest.Model{Replies: []extracttest.Reply{{Body: defaultReply()}}},
		teacher: Caller{ID: uuid.New(), Role: platformauth.RoleTeacher},
		other:   Caller{ID: uuid.New(), Role: platformauth.RoleTeacher},
		admin:   Caller{ID: uuid.New(), Role: platformauth.RoleAdmin},
		student: Caller{ID: uuid.New(), Role: platformauth.RoleStudent},
		opts: Options{MaxBytes: 5 << 20, MaxPages: 10, ParseTimeout: 20 * time.Second, Lease: 3 * time.Second,
			MaxConcurrent: 2, Active: true, MaxLivePerExam: 5},
	}
	for _, m := range mutate {
		m(&h.opts)
	}
	h.examID = h.newExam(h.teacher.ID, exam.StatusDraft)
	h.rebuild()
	return h
}

// rebuild creates the service from the harness's current parts.
func (h *harness) rebuild() {
	nop := zerolog.Nop()
	h.svc = NewService(Deps{
		Repo: fakeRepo{h.w}, Exams: fakeExams{h.w}, Questions: h.importer, Tx: fakeTx{h.w},
		Store: h.store, Extractor: extract.New(h.model, h.extractOpts), Queue: h.queue,
		Options: h.opts, Logger: &nop,
	})
}

func (h *harness) newExam(owner uuid.UUID, status exam.Status) uuid.UUID {
	id := uuid.New()
	h.w.mu.Lock()
	defer h.w.mu.Unlock()
	h.w.exams[id] = &exam.Exam{Base: model.Base{BaseWithId: model.BaseWithId{ID: id}}, Title: "Physics", Subject: "Physics",
		DurationMins: 60, Status: status, CreatedBy: owner}
	return id
}

func (h *harness) setExamStatus(id uuid.UUID, s exam.Status) {
	h.w.mu.Lock()
	defer h.w.mu.Unlock()
	h.w.exams[id].Status = s
}

const (
	q1Text = "What is the SI unit of force?"
	q2Text = "Explain Newton's first law of motion."
)

// samplePDF is a one-page MCQ and descriptive paper whose text matches defaultReply.
func samplePDF() []byte {
	return pdftest.Build([]pdftest.Page{{Texts: []pdftest.Text{
		pdftest.Line(72, 720, "1. "+q1Text),
		pdftest.Line(90, 700, "A. Newton"),
		pdftest.Line(90, 685, "B. Joule"),
		pdftest.Line(90, 670, "C. Watt"),
		pdftest.Line(72, 630, "2. "+q2Text+" [5 marks]"),
	}}})
}

func defaultReply() string {
	return extracttest.ReplyJSON(
		extracttest.MCQ("1", q1Text, 1, "Newton", "Joule", "Watt"),
		extracttest.Essay("2", q2Text, 1),
	)
}

// upload stores samplePDF (or data) as the teacher and returns the document.
func (h *harness) upload(data ...[]byte) *Document {
	h.t.Helper()
	pdfBytes := samplePDF()
	if len(data) > 0 {
		pdfBytes = data[0]
	}
	d, err := h.svc.Upload(context.Background(), h.examID, "paper.pdf", bytesReader(pdfBytes), h.teacher)
	require.NoError(h.t, err)
	return d
}

// processed uploads a document and runs its job to a draft.
func (h *harness) processed() *Document {
	h.t.Helper()
	d := h.upload()
	require.NoError(h.t, h.svc.Process(context.Background(), d.ID, d.Attempt, false))
	got, err := h.svc.Get(context.Background(), h.examID, d.ID, h.teacher)
	require.NoError(h.t, err)
	require.Equal(h.t, StatusReviewRequired, got.Status)
	return got
}

func (h *harness) doc(id uuid.UUID) *Document {
	h.t.Helper()
	d, err := fakeRepo{h.w}.Get(context.Background(), id)
	require.NoError(h.t, err)
	return d
}

func (s *failingStore) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	if s.putErr != nil {
		return s.putErr
	}
	return s.Memory.Put(ctx, key, r, size)
}

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }
