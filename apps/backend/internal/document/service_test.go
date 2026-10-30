package document

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract/extracttest"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/pdf/pdftest"
)

func TestUpload_ValidPDFIsStoredQueuedAndAttributed(t *testing.T) {
	h := newHarness(t)
	data := samplePDF()
	d, err := h.svc.Upload(context.Background(), h.examID, "../../etc/Physics Paper.pdf", bytes.NewReader(data), h.teacher)
	require.NoError(t, err)

	assert.Equal(t, StatusQueued, d.Status)
	assert.Equal(t, 1, d.Attempt)
	assert.Equal(t, 1, d.PageCount)
	assert.Equal(t, int64(len(data)), d.SizeBytes)
	assert.Equal(t, h.examID, d.ExamID)
	assert.Equal(t, h.teacher.ID, d.UploadedBy)
	assert.Equal(t, "Physics Paper.pdf", d.OriginalFilename, "the path part of the client's name is dropped")
	assert.Len(t, d.SHA256, 64)

	// Stored under a server-made key that says nothing about the file or teacher.
	assert.True(t, strings.HasSuffix(d.StorageKey, ".pdf"))
	_, parseErr := uuid.Parse(strings.TrimSuffix(d.StorageKey, ".pdf"))
	assert.NoError(t, parseErr)
	assert.NotContains(t, d.StorageKey, "Physics")
	assert.Len(t, h.store.Keys(), 1)

	assert.Equal(t, []enqueued{{d.ID, 1}}, h.queue.all())
	assert.Equal(t, 0, h.model.Calls(), "uploading reads nothing with the model; that is the job's work")
	assert.Empty(t, h.w.questions, "uploading never touches the exam's questions")
}

func TestCleanFilename(t *testing.T) {
	long := strings.Repeat("é", 400) + ".pdf"
	cases := map[string]string{
		"paper.pdf":                  "paper.pdf",
		`C:\Users\sam\exam.pdf`:      "exam.pdf",
		"../../../etc/passwd":        "passwd",
		"a/b/c.pdf":                  "c.pdf",
		"":                           "exam.pdf",
		"..":                         "exam.pdf",
		"  spaced name .pdf ":        "spaced name .pdf",
		"bad\x00name\r\n.pdf":        "badname.pdf",
		"midterm — physics (v2).pdf": "midterm — physics (v2).pdf",
	}
	for in, want := range cases {
		assert.Equal(t, want, cleanFilename(in), fmt.Sprintf("%q", in))
	}
	assert.Equal(t, 255, len([]rune(cleanFilename(long))))
}

func TestUpload_Authorization(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	up := func(c Caller, examID uuid.UUID) error {
		_, err := h.svc.Upload(ctx, examID, "p.pdf", bytes.NewReader(samplePDF()), c)
		return err
	}

	assert.ErrorIs(t, up(h.student, h.examID), ErrForbiddenRole, "a student cannot upload")
	assert.ErrorIs(t, up(h.other, h.examID), ErrNotFound, "another teacher's exam looks like a missing one")
	assert.ErrorIs(t, up(h.teacher, uuid.New()), ErrNotFound)
	assert.Empty(t, h.store.Keys(), "nothing was stored for any refused caller")
	assert.Empty(t, h.queue.all())

	require.NoError(t, up(h.admin, h.examID), "an administrator can upload to any exam")
}

func TestUpload_OnlyForDraftExams(t *testing.T) {
	for _, status := range []exam.Status{exam.StatusPublished, exam.StatusArchived} {
		h := newHarness(t)
		h.setExamStatus(h.examID, status)
		_, err := h.svc.Upload(context.Background(), h.examID, "p.pdf", bytes.NewReader(samplePDF()), h.teacher)
		assert.ErrorIs(t, err, ErrExamNotDraft, string(status))
		assert.Empty(t, h.store.Keys())
	}
}

func TestUpload_RefusedWhileExtractionIsOff(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.Active = false })
	_, err := h.svc.Upload(context.Background(), h.examID, "p.pdf", bytes.NewReader(samplePDF()), h.teacher)
	assert.ErrorIs(t, err, ErrExtractionUnavailable)
	assert.Empty(t, h.store.Keys(), "no exam content is kept or sent anywhere while the provider is off")
}

func TestUpload_RejectsBadFilesAndStoresNothing(t *testing.T) {
	enc, err := pdftest.Encrypt(samplePDF())
	require.NoError(t, err)
	many := pdftest.Build(func() []pdftest.Page {
		ps := make([]pdftest.Page, 11)
		for i := range ps {
			ps[i] = pdftest.Page{Texts: []pdftest.Text{pdftest.Line(72, 700, "text")}}
		}
		return ps
	}())

	cases := []struct {
		name string
		data []byte
		want error
		why  string
	}{
		{"text file", []byte("just some words"), ErrUnsupportedType, ""},
		{"image renamed to pdf", append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 200)...), ErrUnsupportedType, ""},
		{"empty", nil, ErrUnsupportedType, ""},
		{"truncated", samplePDF()[:400], nil, ReasonMalformed},
		{"garbage inside", []byte("%PDF-1.4\n" + strings.Repeat("\xde\xad\xbe\xef", 300) + "\n%%EOF\n"), nil, ReasonMalformed},
		{"password protected", enc, nil, ReasonEncrypted},
		{"too many pages", many, nil, ReasonTooManyPages},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			_, err := h.svc.Upload(context.Background(), h.examID, "p.pdf", bytes.NewReader(tc.data), h.teacher)
			require.Error(t, err)
			if tc.want != nil {
				assert.ErrorIs(t, err, tc.want)
			} else {
				var ie *InvalidPDFError
				require.ErrorAs(t, err, &ie)
				assert.Equal(t, tc.why, ie.Reason)
			}
			assert.Empty(t, h.store.Keys())
			assert.Empty(t, h.queue.all())
			docs, _ := h.svc.List(context.Background(), h.examID, h.teacher)
			assert.Empty(t, docs, "a refused file leaves no record")
		})
	}
}

// countingReader fails the test if more than limit bytes are read from it.
type countingReader struct {
	t     *testing.T
	n     int64
	limit int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	if c.n > c.limit {
		c.t.Errorf("read %d bytes, past the limit of %d", c.n, c.limit)
		return 0, io.EOF
	}
	for i := range p {
		p[i] = 'x'
	}
	c.n += int64(len(p))
	return len(p), nil
}

func TestUpload_OversizedFileStopsReadingAtTheLimit(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.MaxBytes = 1 << 20 })
	r := &countingReader{t: t, limit: 1<<20 + 64<<10} // endless stream
	_, err := h.svc.Upload(context.Background(), h.examID, "big.pdf", r, h.teacher)
	assert.ErrorIs(t, err, ErrFileTooLarge)
	assert.LessOrEqual(t, r.n, int64(1<<20)+64<<10, "an endless upload is not read to the end")
	assert.Empty(t, h.store.Keys())
}

func TestUpload_SameFileTwiceIsRefusedUnlessTheFirstFailed(t *testing.T) {
	h := newHarness(t)
	first := h.upload()

	_, err := h.svc.Upload(context.Background(), h.examID, "again.pdf", bytes.NewReader(samplePDF()), h.teacher)
	var dup *DuplicateError
	require.ErrorAs(t, err, &dup)
	assert.Equal(t, first.ID, dup.ExistingID, "the response says which upload it duplicates")
	assert.Len(t, h.store.Keys(), 1, "the refused copy's file was removed again")
	assert.Len(t, h.queue.all(), 1)

	// A failed upload does not block trying again.
	_, err = h.svc.Cancel(context.Background(), h.examID, first.ID, h.teacher)
	require.NoError(t, err)
	_, err = h.svc.Upload(context.Background(), h.examID, "again.pdf", bytes.NewReader(samplePDF()), h.teacher)
	assert.NoError(t, err)
}

func TestUpload_QueueDownRollsEverythingBack(t *testing.T) {
	h := newHarness(t)
	h.queue.err = errors.New("redis down")
	_, err := h.svc.Upload(context.Background(), h.examID, "p.pdf", bytes.NewReader(samplePDF()), h.teacher)
	assert.ErrorIs(t, err, ErrQueueUnavailable)
	assert.Empty(t, h.store.Keys(), "no orphaned file")
	docs, _ := h.svc.List(context.Background(), h.examID, h.teacher)
	assert.Empty(t, docs, "no orphaned record that will never be processed")
}

func TestUpload_StoreDownLeavesNoRecord(t *testing.T) {
	h := newHarness(t)
	h.store.putErr = audiostore.ErrUnavailable
	_, err := h.svc.Upload(context.Background(), h.examID, "p.pdf", bytes.NewReader(samplePDF()), h.teacher)
	assert.ErrorIs(t, err, ErrStorageUnavailable)
	assert.Empty(t, h.queue.all())
	docs, _ := h.svc.List(context.Background(), h.examID, h.teacher)
	assert.Empty(t, docs)
}

func TestUpload_LimitOfLiveDocumentsPerExam(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.MaxLivePerExam = 2 })
	for i := 0; i < 2; i++ {
		page := pdftest.Page{Texts: []pdftest.Text{pdftest.Line(72, 700, fmt.Sprintf("paper number %d", i))}}
		h.upload(pdftest.Build([]pdftest.Page{page}))
	}
	_, err := h.svc.Upload(context.Background(), h.examID, "p.pdf", bytes.NewReader(samplePDF()), h.teacher)
	assert.ErrorIs(t, err, ErrTooManyDocuments)
}

func TestUpload_ConcurrentSameFileStoresOneDocument(t *testing.T) {
	h := newHarness(t)
	var ok, dup int32
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_, err := h.svc.Upload(context.Background(), h.examID, "p.pdf", bytes.NewReader(samplePDF()), h.teacher)
			var d *DuplicateError
			switch {
			case err == nil:
				atomic.AddInt32(&ok, 1)
			case errors.As(err, &d):
				atomic.AddInt32(&dup, 1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	assert.EqualValues(t, 1, ok)
	assert.EqualValues(t, 7, dup)
	assert.Len(t, h.store.Keys(), 1, "the losers' files were removed")
	assert.Len(t, h.queue.all(), 1)
}

// ---- reading, ownership ----

func TestAccess_OtherTeachersAndOtherExamsSeeNothing(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	d := h.processed()
	otherExam := h.newExam(h.teacher.ID, exam.StatusDraft)

	checks := map[string]func(c Caller, examID uuid.UUID) error{
		"get":     func(c Caller, e uuid.UUID) error { _, err := h.svc.Get(ctx, e, d.ID, c); return err },
		"list":    func(c Caller, e uuid.UUID) error { _, err := h.svc.List(ctx, e, c); return err },
		"file":    func(c Caller, e uuid.UUID) error { _, _, _, err := h.svc.OpenFile(ctx, e, d.ID, c); return err },
		"draft":   func(c Caller, e uuid.UUID) error { _, err := h.svc.GetDraft(ctx, e, d.ID, c); return err },
		"cancel":  func(c Caller, e uuid.UUID) error { _, err := h.svc.Cancel(ctx, e, d.ID, c); return err },
		"retry":   func(c Caller, e uuid.UUID) error { _, err := h.svc.Retry(ctx, e, d.ID, RetryRequest{}, c); return err },
		"delete":  func(c Caller, e uuid.UUID) error { return h.svc.Delete(ctx, e, d.ID, c) },
		"approve": func(c Caller, e uuid.UUID) error { _, err := h.svc.Approve(ctx, e, d.ID, 1, c); return err },
		"add": func(c Caller, e uuid.UUID) error {
			_, err := h.svc.AddQuestion(ctx, e, d.ID, AddQuestionRequest{ExpectedDraftVersion: 1, Question: validInput()}, c)
			return err
		},
	}
	for name, call := range checks {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, call(h.other, h.examID), ErrNotFound, "another teacher")
			assert.ErrorIs(t, call(h.student, h.examID), ErrForbiddenRole, "a student")
			if name != "list" { // listing your own other exam is fine; the rest name a document that is not in it
				assert.ErrorIs(t, call(h.teacher, otherExam), ErrNotFound, "the owner, but through a different exam's URL")
			}
		})
	}
	assert.Equal(t, StatusReviewRequired, h.doc(d.ID).Status, "none of the refused calls changed anything")
	assert.Empty(t, h.w.questions)

	got, err := h.svc.Get(ctx, h.examID, d.ID, h.admin)
	require.NoError(t, err)
	assert.Equal(t, d.ID, got.ID, "an administrator can see any exam's documents")
}

func TestOpenFile_ReturnsTheOriginalBytesToTheOwner(t *testing.T) {
	h := newHarness(t)
	data := samplePDF()
	d := h.upload(data)
	rc, size, doc, err := h.svc.OpenFile(context.Background(), h.examID, d.ID, h.teacher)
	require.NoError(t, err)
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	assert.Equal(t, data, got)
	assert.Equal(t, int64(len(data)), size)
	assert.Equal(t, "paper.pdf", doc.OriginalFilename)
}

// ---- cancel, retry, delete ----

func TestCancel(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.upload()

	got, err := h.svc.Cancel(ctx, h.examID, d.ID, h.teacher)
	require.NoError(t, err)
	assert.Equal(t, StatusCancelled, got.Status)

	again, err := h.svc.Cancel(ctx, h.examID, d.ID, h.teacher)
	require.NoError(t, err, "cancelling twice is not an error")
	assert.Equal(t, StatusCancelled, again.Status)

	// A document waiting for review, or already approved, is not "processing".
	done := h.processed2()
	_, err = h.svc.Cancel(ctx, h.examID, done.ID, h.teacher)
	assert.ErrorIs(t, err, ErrInvalidState)
}

// processed2 uploads and processes a second, different document.
func (h *harness) processed2() *Document {
	h.t.Helper()
	d := h.upload(pdftest.Build([]pdftest.Page{{Texts: []pdftest.Text{
		pdftest.Line(72, 720, "1. "+q1Text), pdftest.Line(90, 700, "A. Newton"), pdftest.Line(90, 685, "B. Joule"),
		pdftest.Line(90, 670, "C. Watt"), pdftest.Line(72, 630, "2. "+q2Text+" [5 marks]"), pdftest.Line(72, 600, "second copy"),
	}}}))
	require.NoError(h.t, h.svc.Process(context.Background(), d.ID, d.Attempt, false))
	return h.doc(d.ID)
}

func TestRetry_FromFailedAndCancelledStartsANewAttemptWithoutADraft(t *testing.T) {
	ctx := context.Background()
	for _, from := range []string{"failed", "cancelled"} {
		t.Run(from, func(t *testing.T) {
			h := newHarness(t)
			d := h.upload()
			if from == "failed" {
				h.model.Replies = []extracttest.Reply{{Body: "nonsense"}}
				require.NoError(t, h.svc.Process(ctx, d.ID, 1, false))
				require.Equal(t, StatusFailed, h.doc(d.ID).Status)
			} else {
				_, err := h.svc.Cancel(ctx, h.examID, d.ID, h.teacher)
				require.NoError(t, err)
			}

			h.model.Replies = []extracttest.Reply{{Body: defaultReply()}}
			got, err := h.svc.Retry(ctx, h.examID, d.ID, RetryRequest{}, h.teacher)
			require.NoError(t, err)
			assert.Equal(t, StatusQueued, got.Status)
			assert.Equal(t, 2, got.Attempt)
			assert.Empty(t, got.FailureCode)
			assert.Equal(t, enqueued{d.ID, 2}, h.queue.all()[len(h.queue.all())-1])

			require.NoError(t, h.svc.Process(ctx, d.ID, 2, false))
			draft, err := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
			require.NoError(t, err)
			assert.Len(t, draft.Questions, 2, "retrying does not duplicate questions")
		})
	}
}

func TestRetry_AfterAProcessedDraftReplacesItAndNeverDuplicates(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.processed()
	v := d.DraftVersion

	_, err := h.svc.Retry(ctx, h.examID, d.ID, RetryRequest{}, h.teacher)
	assert.ErrorIs(t, err, ErrInvalidState, "a draft waiting for review is not thrown away by accident")

	_, err = h.svc.Retry(ctx, h.examID, d.ID, RetryRequest{DiscardDraft: true, ExpectedDraftVersion: v - 1}, h.teacher)
	assert.ErrorIs(t, err, ErrStaleVersion, "nor one that someone changed since it was read")

	got, err := h.svc.Retry(ctx, h.examID, d.ID, RetryRequest{DiscardDraft: true, ExpectedDraftVersion: v}, h.teacher)
	require.NoError(t, err)
	assert.Equal(t, 2, got.Attempt)
	_, err = h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	assert.ErrorIs(t, err, ErrInvalidState, "the old draft is gone while the new attempt runs")

	require.NoError(t, h.svc.Process(ctx, d.ID, 2, false))
	draft, err := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	require.NoError(t, err)
	assert.Len(t, draft.Questions, 2, "exactly one draft, not two merged")
	assert.Greater(t, draft.Document.DraftVersion, v, "an editor holding the old version is told it is stale")
}

func TestRetry_NotAllowedWhileRunningOrAfterApproval(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.upload()
	_, err := h.svc.Retry(ctx, h.examID, d.ID, RetryRequest{}, h.teacher)
	assert.ErrorIs(t, err, ErrInvalidState, "queued")

	h.w.docs[d.ID].Status = StatusProcessing
	_, err = h.svc.Retry(ctx, h.examID, d.ID, RetryRequest{}, h.teacher)
	assert.ErrorIs(t, err, ErrInvalidState, "processing")

	h.w.docs[d.ID].Status = StatusCompleted
	_, err = h.svc.Retry(ctx, h.examID, d.ID, RetryRequest{DiscardDraft: true}, h.teacher)
	assert.ErrorIs(t, err, ErrInvalidState, "completed")
}

func TestRetry_QueueDownMarksTheDocumentFailed(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.upload()
	_, err := h.svc.Cancel(ctx, h.examID, d.ID, h.teacher)
	require.NoError(t, err)

	h.queue.err = errors.New("redis down")
	_, err = h.svc.Retry(ctx, h.examID, d.ID, RetryRequest{}, h.teacher)
	assert.ErrorIs(t, err, ErrQueueUnavailable)
	got := h.doc(d.ID)
	assert.Equal(t, StatusFailed, got.Status, "not left QUEUED with nothing to run it")
	assert.Equal(t, extract.CodeQueueDown, got.FailureCode)
}

func TestRetry_RefusedWhileExtractionIsOff(t *testing.T) {
	h := newHarness(t)
	d := h.upload()
	_, err := h.svc.Cancel(context.Background(), h.examID, d.ID, h.teacher)
	require.NoError(t, err)
	h.opts.Active = false
	h.rebuild()
	_, err = h.svc.Retry(context.Background(), h.examID, d.ID, RetryRequest{}, h.teacher)
	assert.ErrorIs(t, err, ErrExtractionUnavailable)
}

func TestDelete_RemovesRecordAndFileButKeepsApprovedQuestions(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.processed()
	h.makeApprovable(d.ID)
	res, err := h.svc.Approve(ctx, h.examID, d.ID, h.doc(d.ID).DraftVersion, h.teacher)
	require.NoError(t, err)
	require.Len(t, res.Questions, 2)

	require.NoError(t, h.svc.Delete(ctx, h.examID, d.ID, h.teacher))
	_, err = h.svc.Get(ctx, h.examID, d.ID, h.teacher)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Empty(t, h.store.Keys(), "the PDF is deleted with its record")
	require.Len(t, h.w.questions, 2, "the exam keeps the questions it was given")
	assert.Nil(t, h.w.questions[0].SourceDocumentID, "only their link to the document goes")
}

func TestDelete_WhileProcessingStopsTheJobWithoutAnError(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.upload()
	release := make(chan struct{})
	h.model.Block = release
	h.rebuild()

	result := make(chan error, 1)
	go func() { result <- h.svc.Process(ctx, d.ID, 1, false) }()
	waitFor(t, func() bool { return h.model.Calls() == 1 })

	require.NoError(t, h.svc.Delete(ctx, h.examID, d.ID, h.teacher))
	close(release)
	assert.NoError(t, <-result, "the job finds its document gone and stops quietly")
	assert.Empty(t, h.w.drafts, "no draft was written for a deleted document")
}
