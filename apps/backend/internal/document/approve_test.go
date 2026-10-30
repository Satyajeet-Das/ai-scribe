package document

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract/extracttest"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
)

func TestApprove_CopiesTheReviewedDraftIntoTheExamAndNothingMore(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.w.questions = append(h.w.questions, question.Question{ExamID: h.examID, QuestionNumber: 3, Text: "typed by hand", Type: question.TypeEssay})
	d := h.processed()
	draft, _ := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	v := d.DraftVersion

	// The teacher reviews: marks, instructions, a table, a confirmed diagram, and
	// chooses the correct option herself.
	in1 := inputOf(draft.Questions[0])
	in1.Points = ptr(2)
	in1.Instructions = "Answer all questions."
	in1.Tables = []TableInput{{Page: 1, Markdown: "| V | I |\n|---|---|\n| 2 | 0.5 |"}}
	in1.Diagrams = []DiagramInput{{Page: 1, Region: "top", Description: "A circuit with one cell and a lamp.", Confirmed: true}}
	in1.Options[1].IsCorrect = true
	r1, err := h.svc.ReplaceQuestion(ctx, h.examID, d.ID, draft.Questions[0].ID, ReplaceQuestionRequest{ExpectedDraftVersion: v, Question: in1}, h.teacher)
	require.NoError(t, err)
	in2 := inputOf(draft.Questions[1])
	in2.Points = ptr(5)
	r2, err := h.svc.ReplaceQuestion(ctx, h.examID, d.ID, draft.Questions[1].ID, ReplaceQuestionRequest{ExpectedDraftVersion: r1.DraftVersion, Question: in2}, h.teacher)
	require.NoError(t, err)

	res, err := h.svc.Approve(ctx, h.examID, d.ID, r2.DraftVersion, h.teacher)
	require.NoError(t, err)

	require.Len(t, res.Questions, 2)
	q1, q2 := res.Questions[0], res.Questions[1]
	assert.Equal(t, 4, q1.QuestionNumber, "added after the question already in the exam")
	assert.Equal(t, 5, q2.QuestionNumber)
	assert.Equal(t, question.TypeMCQ, q1.Type)
	assert.Equal(t, question.TypeEssay, q2.Type)
	assert.Equal(t, 2, q1.Points)
	assert.Equal(t, 5, q2.Points)
	assert.Equal(t, "Answer all questions.\n\n"+q1Text+"\n\nTable (page 1):\nV | I\n2 | 0.5\n\nDiagram (page 1): A circuit with one cell and a lamp.", q1.Text,
		"everything the candidate needs is in the one text a question has")
	require.Len(t, q1.Options, 3)
	assert.Equal(t, []string{"A", "B", "C"}, []string{q1.Options[0].OptionKey, q1.Options[1].OptionKey, q1.Options[2].OptionKey})
	assert.True(t, q1.Options[1].IsCorrect, "the correct option is the one the teacher chose")
	assert.False(t, q1.Options[0].IsCorrect)
	assert.Equal(t, d.ID, *q1.SourceDocumentID)
	assert.Equal(t, draft.Questions[0].ID, *q1.SourceDraftQuestionID)

	got := h.doc(d.ID)
	assert.Equal(t, StatusCompleted, got.Status)
	require.NotNil(t, got.ApprovedBy)
	assert.Equal(t, h.teacher.ID, *got.ApprovedBy)
	assert.NotNil(t, got.ApprovedAt)

	// Approval fills a draft exam; it does not publish or assign anything.
	assert.Equal(t, exam.StatusDraft, h.w.exams[h.examID].Status)
	assert.Nil(t, h.w.exams[h.examID].PublishedAt)
	assert.Equal(t, 1, h.importer.calls)
}

func TestApprove_AdminMayApproveAnotherTeachersDraft(t *testing.T) {
	h := newHarness(t)
	d := h.processed()
	h.makeApprovable(d.ID)
	_, err := h.svc.Approve(context.Background(), h.examID, d.ID, h.doc(d.ID).DraftVersion, h.admin)
	require.NoError(t, err)
	assert.Equal(t, h.admin.ID, *h.doc(d.ID).ApprovedBy)
}

func TestApprove_IncompleteDraftsListEveryProblemAndImportNothing(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	diagram := `{"numberLabel": "2", "type": "ESSAY", "instructions": null, "text": "Use the figure to find the current.", "marks": null,
		"options": [], "tables": [], "diagrams": [{"page": 1, "region": null, "description": null}, {"page": 1, "region": null, "description": "A circuit."}],
		"pages": [1], "continuesPrevious": false, "uncertain": false}`
	h.model.Replies = []extracttest.Reply{{Body: extracttest.ReplyJSON(
		extracttest.MCQ("1", q1Text, 1, "Newton"), // one option; wording on the page
		diagram, // text not on the page; two diagrams, one blank, one unconfirmed
	)}}
	h.rebuild()
	d := h.processed()

	_, err := h.svc.Approve(ctx, h.examID, d.ID, d.DraftVersion, h.teacher)
	var inc *IncompleteError
	require.ErrorAs(t, err, &inc)

	got := map[string]bool{}
	for _, p := range inc.Problems {
		got[p.Field] = true
		assert.NotEqual(t, uuid.Nil, p.QuestionID, "each problem says which question")
		assert.NotEmpty(t, p.Message)
	}
	for _, want := range []string{
		"points",                                 // no marks on either question
		"options",                                // MCQ with a single option
		"diagrams[0].description",                // blank description
		"diagrams[1].confirmed",                  // description not confirmed
		"warnings." + extract.WarnUnverifiedText, // wording not found on the page, not acknowledged
	} {
		assert.True(t, got[want], "expected a problem for %s, got %v", want, got)
	}
	assert.Empty(t, h.w.questions, "nothing was added")
	assert.Equal(t, StatusReviewRequired, h.doc(d.ID).Status, "and the draft is still there to fix")
	assert.Equal(t, 0, h.importer.calls)
}

func TestApprove_AnEmptyDraftIsNotApproved(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.processed()
	draft, _ := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	v := d.DraftVersion
	for _, q := range draft.Questions {
		after, err := h.svc.DeleteQuestion(ctx, h.examID, d.ID, q.ID, v, h.teacher)
		require.NoError(t, err)
		v = after.Document.DraftVersion
	}
	_, err := h.svc.Approve(ctx, h.examID, d.ID, v, h.teacher)
	var inc *IncompleteError
	require.ErrorAs(t, err, &inc)
	assert.Equal(t, "questions", inc.Problems[0].Field)
}

func TestApprove_StaleVersionIsRefused(t *testing.T) {
	h := newHarness(t)
	d := h.processed()
	h.makeApprovable(d.ID)
	cur := h.doc(d.ID).DraftVersion
	_, err := h.svc.Approve(context.Background(), h.examID, d.ID, cur-1, h.teacher)
	assert.ErrorIs(t, err, ErrStaleVersion, "approving what the teacher did not see is refused")
	assert.Empty(t, h.w.questions)
}

func TestApprove_NotWhileQueuedOrProcessing(t *testing.T) {
	h := newHarness(t)
	d := h.upload()
	_, err := h.svc.Approve(context.Background(), h.examID, d.ID, 0, h.teacher)
	assert.ErrorIs(t, err, ErrInvalidState)
}

func TestApprove_OnlyIntoADraftExam(t *testing.T) {
	for _, status := range []exam.Status{exam.StatusPublished, exam.StatusArchived} {
		t.Run(string(status), func(t *testing.T) {
			h := newHarness(t)
			d := h.processed()
			h.makeApprovable(d.ID)
			h.setExamStatus(h.examID, status) // published or archived after the review began
			_, err := h.svc.Approve(context.Background(), h.examID, d.ID, h.doc(d.ID).DraftVersion, h.teacher)
			assert.ErrorIs(t, err, ErrExamNotDraft)
			assert.Empty(t, h.w.questions, "a live exam never receives unreviewed or late content")
			assert.Equal(t, StatusReviewRequired, h.doc(d.ID).Status)
		})
	}
}

func TestApprove_TwiceAndConcurrentlyCreatesTheQuestionsOnce(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.processed()
	h.makeApprovable(d.ID)
	v := h.doc(d.ID).DraftVersion

	var ok, already int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.svc.Approve(ctx, h.examID, d.ID, v, h.teacher)
			switch {
			case err == nil:
				atomic.AddInt32(&ok, 1)
			case errors.Is(err, ErrAlreadyApproved):
				atomic.AddInt32(&already, 1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, ok)
	assert.EqualValues(t, 7, already)
	assert.Len(t, h.w.questions, 2, "no duplicates")
	assert.Equal(t, 1, h.importer.calls)

	// And again, later, with the version it now has.
	_, err := h.svc.Approve(ctx, h.examID, d.ID, h.doc(d.ID).DraftVersion, h.teacher)
	assert.ErrorIs(t, err, ErrAlreadyApproved)
	assert.Len(t, h.w.questions, 2)
}

func TestApprove_IsAllOrNothing(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.processed()
	h.makeApprovable(d.ID)
	v := h.doc(d.ID).DraftVersion

	h.w.importFailAt = 2 // the database goes away after the first question is written
	_, err := h.svc.Approve(ctx, h.examID, d.ID, v, h.teacher)
	require.Error(t, err)
	assert.Empty(t, h.w.questions, "the first question was rolled back with the rest")
	got := h.doc(d.ID)
	assert.Equal(t, StatusReviewRequired, got.Status, "the document is not marked approved")
	assert.Nil(t, got.ApprovedAt)

	h.w.importFailAt = 0
	_, err = h.svc.Approve(ctx, h.examID, d.ID, v, h.teacher)
	require.NoError(t, err, "the same approval works once the database is back")
	assert.Len(t, h.w.questions, 2, "and creates each question once")
}

func TestApprove_AQuestionAddedByHandAtTheSameTimeIsAConflictNotACrash(t *testing.T) {
	h := newHarness(t)
	d := h.processed()
	h.makeApprovable(d.ID)
	h.w.importErr = uniqueViolation()
	_, err := h.svc.Approve(context.Background(), h.examID, d.ID, h.doc(d.ID).DraftVersion, h.teacher)
	assert.ErrorIs(t, err, ErrConcurrentChange)
	assert.Equal(t, StatusReviewRequired, h.doc(d.ID).Status)
}

func TestApprove_ComposedTextIsWhatTheCandidateHears(t *testing.T) {
	// The exam room reads the question text aloud. A reviewed question with a
	// table and a diagram must carry both in that text, in a stable order.
	ctx := context.Background()
	h := newHarness(t)
	body := `{"numberLabel": "4", "type": "ESSAY", "instructions": "Show your working.", "text": "` + q2Text + `", "marks": 5,
		"options": [], "tables": [{"page": 1, "markdown": "| Time | Speed |\n| --- | --- |\n| 1 | 4 |\n| 2 | 8 |"}],
		"diagrams": [{"page": 1, "region": "below", "description": "A speed time graph rising in a straight line."}],
		"pages": [1], "continuesPrevious": false, "uncertain": false}`
	h.model.Replies = []extracttest.Reply{{Body: extracttest.ReplyJSON(body)}}
	h.rebuild()
	d := h.processed()
	draft, _ := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	in := inputOf(draft.Questions[0])
	in.Diagrams[0].Confirmed = true
	res, err := h.svc.ReplaceQuestion(ctx, h.examID, d.ID, draft.Questions[0].ID, ReplaceQuestionRequest{ExpectedDraftVersion: d.DraftVersion, Question: in}, h.teacher)
	require.NoError(t, err)

	out, err := h.svc.Approve(ctx, h.examID, d.ID, res.DraftVersion, h.teacher)
	require.NoError(t, err)
	assert.Equal(t, "Show your working.\n\n"+q2Text+"\n\nTable (page 1):\nTime | Speed\n1 | 4\n2 | 8\n\nDiagram (page 1): A speed time graph rising in a straight line.", out.Questions[0].Text)
	assert.Equal(t, 5, out.Questions[0].Points, "marks printed on the paper are kept")
}
