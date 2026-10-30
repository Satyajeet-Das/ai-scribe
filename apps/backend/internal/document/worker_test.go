package document

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract/extracttest"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/pdf/pdftest"
)

func TestProcess_ProducesAReviewableDraft(t *testing.T) {
	h := newHarness(t)
	d := h.upload()
	require.NoError(t, h.svc.Process(context.Background(), d.ID, 1, false))

	got := h.doc(d.ID)
	assert.Equal(t, StatusReviewRequired, got.Status)
	assert.Equal(t, 1, got.DraftVersion)
	assert.Empty(t, string(got.Stage))
	assert.Equal(t, got.ProgressTotal, got.ProgressDone, "progress ends complete")
	assert.Equal(t, "fake", got.Provider)
	assert.Nil(t, got.LeaseExpires, "the worker let go of the document")
	require.Len(t, got.PageSummary, 1)
	assert.Equal(t, "TEXT", got.PageSummary[0].Kind)

	qs := h.w.drafts[d.ID]
	require.Len(t, qs, 2)
	assert.Equal(t, extract.ConfidenceHigh, qs[0].Confidence)
	assert.Empty(t, qs[0].Warnings, "wording read from the page's text layer needs no warning")
	assert.Equal(t, []int{1, 2}, []int{qs[0].Position, qs[1].Position})
}

func TestProcess_ExtractionNeverTouchesTheExam(t *testing.T) {
	h := newHarness(t)
	before := *h.w.exams[h.examID]
	h.processed()
	assert.Empty(t, h.w.questions, "extracted questions are a draft; only an explicit approval adds them")
	assert.Equal(t, 0, h.importer.calls)
	assert.Equal(t, before.Status, h.w.exams[h.examID].Status)
}

func TestProcess_SendsTheModelCleanTextInReadingOrder(t *testing.T) {
	var pages []pdftest.Page
	for i := 1; i <= 3; i++ {
		var texts []pdftest.Text
		texts = append(texts, pdftest.Line(200, 760, "Physics Paper 1"), pdftest.Line(250, 30, fmt.Sprintf("Page %d of 3", i)))
		for j := 0; j < 4; j++ {
			y := 700 - float64(j)*22
			texts = append(texts,
				pdftest.Line(60, y, fmt.Sprintf("%d. Left column question number %d on page %d", j+1, j+1, i)),
				pdftest.Line(340, y, fmt.Sprintf("%d. Right column question number %d on page %d", j+5, j+5, i)))
		}
		pages = append(pages, pdftest.Page{Texts: texts})
	}
	h := newHarness(t)
	h.model.Replies = []extracttest.Reply{{Body: extracttest.ReplyJSON(extracttest.Essay("1", "x y", 1))}}
	h.rebuild()
	d := h.upload(pdftest.Build(pages))
	require.NoError(t, h.svc.Process(context.Background(), d.ID, 1, false))

	require.GreaterOrEqual(t, h.model.Calls(), 1)
	var sent strings.Builder
	for _, req := range h.model.Requests {
		for _, p := range req.Pages {
			sent.WriteString(p.Text + "\n")
		}
	}
	text := sent.String()
	assert.NotContains(t, text, "Physics Paper 1", "running headers are not sent as if they were content")
	assert.NotContains(t, text, "Page 2 of 3")
	left := strings.Index(text, "Left column question number 4 on page 1")
	right := strings.Index(text, "Right column question number 5 on page 1")
	require.True(t, left >= 0 && right >= 0)
	assert.Less(t, left, right, "the whole left column is read before the right one")

	got := h.doc(d.ID)
	require.Len(t, got.PageSummary, 3)
	assert.Equal(t, 2, got.PageSummary[0].Columns)
	assert.Contains(t, got.PageSummary[0].RemovedLines, "Physics Paper 1", "what was set aside is recorded")
}

func TestProcess_ScannedPages(t *testing.T) {
	scan := pdftest.Page{Images: []pdftest.Image{{X: 30, Y: 30, W: 550, H: 730, PxW: 200, PxH: 280}}}
	textPage := pdftest.Page{Texts: []pdftest.Text{pdftest.Line(72, 720, "1. "+q1Text), pdftest.Line(90, 700, "A. Newton"), pdftest.Line(90, 685, "B. Joule")}}
	data := pdftest.Build([]pdftest.Page{textPage, scan})

	t.Run("without a vision model the page is flagged, not guessed", func(t *testing.T) {
		h := newHarness(t)
		h.model.Replies = []extracttest.Reply{{Body: extracttest.ReplyJSON(extracttest.MCQ("1", q1Text, 1, "Newton", "Joule"))}}
		h.rebuild()
		d := h.upload(data)
		require.NoError(t, h.svc.Process(context.Background(), d.ID, 1, false))

		got := h.doc(d.ID)
		assert.Equal(t, 1, h.model.Calls(), "the picture page was not sent anywhere")
		require.Len(t, got.PageSummary, 2)
		assert.Equal(t, "IMAGE_ONLY", got.PageSummary[1].Kind)
		assert.Contains(t, got.PageSummary[1].Note, "no vision model")
		require.NotEmpty(t, got.Unassigned)
		assert.Equal(t, 2, got.Unassigned[0].Page, "the teacher is told page 2 was not read")
	})

	t.Run("with a vision model its pictures are read and marked as read from a picture", func(t *testing.T) {
		h := newHarness(t)
		h.model.Vision = true
		h.model.Replies = []extracttest.Reply{
			{Body: extracttest.ReplyJSON(extracttest.MCQ("1", q1Text, 1, "Newton", "Joule"))},
			{Body: extracttest.ReplyJSON(extracttest.Essay("2", "Describe the carbon cycle.", 2))},
		}
		h.rebuild()
		d := h.upload(data)
		require.NoError(t, h.svc.Process(context.Background(), d.ID, 1, false))

		require.Equal(t, 2, h.model.Calls())
		vision := h.model.Requests[1]
		assert.True(t, vision.Vision)
		require.Len(t, vision.Pages[0].Images, 1)
		assert.NotEmpty(t, vision.Pages[0].Images[0].Data)

		qs := h.w.drafts[d.ID]
		require.Len(t, qs, 2)
		assert.Contains(t, warningCodes(qs[1]), extract.WarnReadFromImage)
		assert.NotEqual(t, extract.ConfidenceHigh, qs[1].Confidence)
		assert.Equal(t, []int{2}, qs[1].SourcePages)
		assert.Equal(t, 1, h.doc(d.ID).PageSummary[1].Images)
	})
}

// ---- duplicate, concurrent and stale jobs ----

func TestProcess_ASecondDeliveryOfAFinishedJobDoesNothing(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.upload()
	require.NoError(t, h.svc.Process(ctx, d.ID, 1, false))
	calls := h.model.Calls()

	require.NoError(t, h.svc.Process(ctx, d.ID, 1, false))
	assert.Equal(t, calls, h.model.Calls(), "the model is not called again")
	assert.Len(t, h.w.drafts[d.ID], 2, "and the draft is not duplicated or replaced")
	assert.Equal(t, 1, h.doc(d.ID).DraftVersion)
}

func TestProcess_TwoWorkersAtOnceOnlyOneRuns(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.upload()
	release := make(chan struct{})
	h.model.Block = release
	h.rebuild()

	first := make(chan error, 1)
	go func() { first <- h.svc.Process(ctx, d.ID, 1, false) }()
	waitFor(t, func() bool { return h.model.Calls() == 1 })

	err := h.svc.Process(ctx, d.ID, 1, false)
	assert.ErrorIs(t, err, ErrLeaseHeld, "the second delivery is asked to come back later, not run in parallel")
	assert.Equal(t, 1, h.model.Calls())

	close(release)
	require.NoError(t, <-first)
	assert.Equal(t, 1, h.model.Calls())
	assert.Len(t, h.w.drafts[d.ID], 2)
}

func TestProcess_ADeadWorkersDocumentIsTakenOverAfterItsLease(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.upload()

	// A worker claimed the document and died: PROCESSING with a lease that ran out.
	past := time.Now().Add(-time.Minute)
	h.w.docs[d.ID].Status, h.w.docs[d.ID].LeaseExpires = StatusProcessing, &past

	require.NoError(t, h.svc.Process(ctx, d.ID, 1, false), "redelivery takes over")
	assert.Equal(t, StatusReviewRequired, h.doc(d.ID).Status)

	// But a live lease is respected.
	d2 := h.upload(pdftest.Build([]pdftest.Page{{Texts: []pdftest.Text{pdftest.Line(72, 700, "1. "+q1Text)}}}))
	future := time.Now().Add(time.Minute)
	h.w.docs[d2.ID].Status, h.w.docs[d2.ID].LeaseExpires = StatusProcessing, &future
	assert.ErrorIs(t, h.svc.Process(ctx, d2.ID, 1, false), ErrLeaseHeld)
}

func TestProcess_AWorkerThatWasReplacedCannotOverwriteTheNewerAttempt(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.upload()
	release := make(chan struct{})
	h.model.Block = release
	h.rebuild()

	old := make(chan error, 1)
	go func() { old <- h.svc.Process(ctx, d.ID, 1, false) }()
	waitFor(t, func() bool { return h.model.Calls() == 1 })

	// While the first worker is still reading, the teacher cancels and retries.
	_, err := h.svc.Cancel(ctx, h.examID, d.ID, h.teacher)
	require.NoError(t, err)
	_, err = h.svc.Retry(ctx, h.examID, d.ID, RetryRequest{}, h.teacher)
	require.NoError(t, err)

	close(release) // the old worker now finishes its model call
	assert.NoError(t, <-old, "it notices the document is no longer its own and stops quietly")

	got := h.doc(d.ID)
	assert.Equal(t, 2, got.Attempt)
	assert.Equal(t, StatusQueued, got.Status, "the stale result did not move the newer attempt on")
	assert.Empty(t, h.w.drafts[d.ID], "and wrote no draft")

	// The new attempt runs normally and produces exactly one draft.
	h.model.Block = nil
	require.NoError(t, h.svc.Process(ctx, d.ID, 2, false))
	assert.Len(t, h.w.drafts[d.ID], 2)
	assert.Equal(t, StatusReviewRequired, h.doc(d.ID).Status)
}

func TestProcess_AStaleAttemptNumberDoesNothing(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.upload()
	_, err := h.svc.Cancel(ctx, h.examID, d.ID, h.teacher)
	require.NoError(t, err)
	_, err = h.svc.Retry(ctx, h.examID, d.ID, RetryRequest{}, h.teacher)
	require.NoError(t, err)

	require.NoError(t, h.svc.Process(ctx, d.ID, 1, false), "the first attempt's queued job is stale")
	assert.Equal(t, 0, h.model.Calls())
	assert.Equal(t, StatusQueued, h.doc(d.ID).Status)
}

func TestProcess_CancelledAndDeletedDocumentsAreSkipped(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.upload()
	_, err := h.svc.Cancel(ctx, h.examID, d.ID, h.teacher)
	require.NoError(t, err)
	require.NoError(t, h.svc.Process(ctx, d.ID, 1, false))
	assert.Equal(t, 0, h.model.Calls(), "no model call is spent on a cancelled document")
	assert.Equal(t, StatusCancelled, h.doc(d.ID).Status)

	d2 := h.upload(pdftest.Build([]pdftest.Page{{Texts: []pdftest.Text{pdftest.Line(72, 700, "other paper entirely")}}}))
	require.NoError(t, h.svc.Delete(ctx, h.examID, d2.ID, h.teacher))
	require.NoError(t, h.svc.Process(ctx, d2.ID, 1, false))
	assert.Equal(t, 0, h.model.Calls())
}

func TestProcess_CancellingWhileRunningStopsTheJobAndTheModelCall(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.upload()
	h.model.Block = make(chan struct{}) // never released: only cancellation can end the call
	h.rebuild()

	result := make(chan error, 1)
	go func() { result <- h.svc.Process(ctx, d.ID, 1, false) }()
	waitFor(t, func() bool { return h.model.Calls() == 1 })

	_, err := h.svc.Cancel(ctx, h.examID, d.ID, h.teacher)
	require.NoError(t, err)

	select {
	case err := <-result:
		assert.NoError(t, err, "a cancelled job ends without an error")
	case <-time.After(10 * time.Second):
		t.Fatal("the job kept running after the document was cancelled")
	}
	got := h.doc(d.ID)
	assert.Equal(t, StatusCancelled, got.Status)
	assert.Empty(t, h.w.drafts[d.ID])
}

// ---- failures ----

func TestProcess_ProviderOutageIsRetriedThenSucceeds(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.model.Replies = []extracttest.Reply{
		{Err: extract.NewError(extract.CodeUnavailable, true, "503")},
		{Body: defaultReply()},
	}
	h.rebuild()
	d := h.upload()

	err := h.svc.Process(ctx, d.ID, 1, false)
	require.Error(t, err, "the job reports failure so the queue retries it")
	assert.Equal(t, StatusQueued, h.doc(d.ID).Status, "and the teacher sees it waiting, not failed")
	assert.Empty(t, h.doc(d.ID).FailureCode)

	require.NoError(t, h.svc.Process(ctx, d.ID, 1, false))
	assert.Equal(t, StatusReviewRequired, h.doc(d.ID).Status)
	assert.Len(t, h.w.drafts[d.ID], 2, "one draft after a retry, never two")
}

func TestProcess_ProviderFailuresOnTheLastDeliveryFailTheDocument(t *testing.T) {
	cases := []struct {
		err  error
		code string
	}{
		{extract.NewError(extract.CodeRateLimited, true, "429"), extract.CodeRateLimited},
		{extract.NewError(extract.CodeUnavailable, true, "503"), extract.CodeUnavailable},
		{extract.NewError(extract.CodeTimeout, true, "slow"), extract.CodeTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			h := newHarness(t)
			h.model.Replies = []extracttest.Reply{{Err: tc.err}}
			h.rebuild()
			d := h.upload()
			require.NoError(t, h.svc.Process(context.Background(), d.ID, 1, true))

			got := h.doc(d.ID)
			assert.Equal(t, StatusFailed, got.Status)
			assert.Equal(t, tc.code, got.FailureCode)
			assert.Equal(t, failureMessage(tc.code), got.FailureMessage)
			assert.NotContains(t, got.FailureMessage, "503", "the message is ours, not the provider's")
			assert.Empty(t, h.w.drafts[d.ID], "a failed run leaves no partial draft")
			assert.Empty(t, h.w.questions)
		})
	}
}

func TestProcess_UnusableOutputFailsAtOnceWithoutRetries(t *testing.T) {
	h := newHarness(t)
	h.model.Replies = []extracttest.Reply{{Body: "I'm sorry, I can't help with that."}}
	h.rebuild()
	d := h.upload()
	require.NoError(t, h.svc.Process(context.Background(), d.ID, 1, false), "nothing for the queue to retry")

	got := h.doc(d.ID)
	assert.Equal(t, StatusFailed, got.Status)
	assert.Equal(t, extract.CodeInvalidOutput, got.FailureCode)
	assert.NotContains(t, got.FailureMessage, "sorry")
	assert.Equal(t, 2, h.model.Calls(), "asked once more, then given up on")
}

func TestProcess_ModelThatFindsNoQuestionsFails(t *testing.T) {
	h := newHarness(t)
	h.model.Replies = []extracttest.Reply{{Body: extracttest.ReplyJSON()}}
	h.rebuild()
	d := h.upload()
	require.NoError(t, h.svc.Process(context.Background(), d.ID, 1, false))
	got := h.doc(d.ID)
	assert.Equal(t, StatusFailed, got.Status)
	assert.Equal(t, extract.CodeNothingFound, got.FailureCode)
}

func TestProcess_PermanentProviderErrorsFailAtOnce(t *testing.T) {
	for _, code := range []string{extract.CodeAuth, extract.CodeModelNotFound, extract.CodeRejected, extract.CodeTruncated} {
		h := newHarness(t)
		h.model.Replies = []extracttest.Reply{{Err: extract.NewError(code, false, "")}}
		h.rebuild()
		d := h.upload()
		require.NoError(t, h.svc.Process(context.Background(), d.ID, 1, false), code)
		assert.Equal(t, StatusFailed, h.doc(d.ID).Status, code)
		assert.Equal(t, code, h.doc(d.ID).FailureCode)
		assert.Equal(t, 1, h.model.Calls(), "%s is not retried", code)
	}
}

func TestProcess_TimeoutIsRetriedAndThenFails(t *testing.T) {
	h := newHarness(t)
	h.model.Block = make(chan struct{}) // the provider never answers
	h.rebuild()
	d := h.upload()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	err := h.svc.Process(ctx, d.ID, 1, false)
	require.Error(t, err)
	assert.Equal(t, StatusQueued, h.doc(d.ID).Status, "put back so the retry can claim it")

	ctx2, cancel2 := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel2()
	require.NoError(t, h.svc.Process(ctx2, d.ID, 1, true))
	got := h.doc(d.ID)
	assert.Equal(t, StatusFailed, got.Status)
	assert.Equal(t, extract.CodeTimeout, got.FailureCode)
}

func TestProcess_DatabaseFailureWhileSavingLeavesNoPartialDraft(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.upload()
	h.w.saveDraftErr = errors.New("connection reset")

	err := h.svc.Process(ctx, d.ID, 1, false)
	require.Error(t, err, "the queue is told to retry")
	assert.Equal(t, StatusQueued, h.doc(d.ID).Status)
	assert.Empty(t, h.w.drafts[d.ID], "no half-written draft")
	assert.Empty(t, h.w.questions)

	h.w.saveDraftErr = nil
	require.NoError(t, h.svc.Process(ctx, d.ID, 1, false))
	assert.Len(t, h.w.drafts[d.ID], 2)
}

func TestProcess_FileProblems(t *testing.T) {
	t.Run("the stored file is missing", func(t *testing.T) {
		h := newHarness(t)
		d := h.upload()
		require.NoError(t, h.store.Delete(context.Background(), d.StorageKey))
		require.NoError(t, h.svc.Process(context.Background(), d.ID, 1, false))
		got := h.doc(d.ID)
		assert.Equal(t, StatusFailed, got.Status)
		assert.Equal(t, codeFileMissing, got.FailureCode)
		assert.Equal(t, 0, h.model.Calls())
	})
	t.Run("the stored file is not the one that was uploaded", func(t *testing.T) {
		h := newHarness(t)
		d := h.upload()
		require.NoError(t, h.store.Delete(context.Background(), d.StorageKey))
		other := pdftest.Build([]pdftest.Page{{Texts: []pdftest.Text{pdftest.Line(72, 700, "a different paper")}}})
		require.NoError(t, h.store.Put(context.Background(), d.StorageKey, bytesReader(other), int64(len(other))))

		require.NoError(t, h.svc.Process(context.Background(), d.ID, 1, false))
		got := h.doc(d.ID)
		assert.Equal(t, StatusFailed, got.Status)
		assert.Equal(t, extract.CodeInvalidPDF, got.FailureCode)
		assert.Equal(t, 0, h.model.Calls(), "a tampered file never reaches the model")
	})
}

func TestProcess_ADocumentWithTooManyBatchesIsRefusedBeforeAnyCall(t *testing.T) {
	var pages []pdftest.Page
	for i := 0; i < 6; i++ {
		pages = append(pages, pdftest.Page{Texts: []pdftest.Text{pdftest.Line(72, 700, fmt.Sprintf("%d. ", i+1)+strings.Repeat(fmt.Sprintf("topic%d words ", i), 10))}})
	}
	h := newHarness(t)
	h.rebuild()
	svc := h.svc.(*service)
	svc.extractor = extract.New(h.model, extract.Options{MaxCharsPerRequest: 100, MaxBatches: 3})
	d := h.upload(pdftest.Build(pages))
	require.NoError(t, h.svc.Process(context.Background(), d.ID, 1, false))
	got := h.doc(d.ID)
	assert.Equal(t, StatusFailed, got.Status)
	assert.Equal(t, extract.CodeTooManyBatches, got.FailureCode)
	assert.Equal(t, 0, h.model.Calls(), "no money is spent on a document that cannot finish")
}

func TestProcess_FailureMessagesAreOursAndCoverEveryCode(t *testing.T) {
	codes := []string{
		extract.CodeRateLimited, extract.CodeUnavailable, extract.CodeTimeout, extract.CodeAuth, extract.CodeRejected,
		extract.CodeModelNotFound, extract.CodeTruncated, extract.CodeInvalidOutput, extract.CodeNothingFound,
		extract.CodeInvalidPDF, extract.CodeQueueDown, extract.CodeStorageDown, extract.CodeTooManyBatches,
		codeFileMissing, codeInterrupted, extract.CodeInternal,
	}
	seen := map[string]bool{}
	for _, c := range codes {
		m := failureMessage(c)
		assert.NotEmpty(t, m, c)
		assert.LessOrEqual(t, len(m), 500, c)
		seen[m] = true
	}
	assert.Greater(t, len(seen), 12, "codes have messages of their own")
}
