package document

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract/extracttest"
)

func ptr[T any](v T) *T { return &v }

func validInput() QuestionInput {
	return QuestionInput{
		NumberLabel: "3", Type: extract.TypeMCQ, Text: "Which of these is a vector quantity?", Points: ptr(2),
		Options: []OptionInput{{Label: "a", Text: "Mass"}, {Label: "b", Text: "Velocity"}},
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not reached in time")
}

// inputOf turns a stored draft question back into the complete state a client
// would send to keep it as it is.
func inputOf(q DraftQuestion) QuestionInput {
	in := QuestionInput{NumberLabel: q.NumberLabel, Type: q.Type, Instructions: q.Instructions, Text: q.Text, Points: q.Points}
	for _, o := range q.Options {
		in.Options = append(in.Options, OptionInput{Label: o.Label, Text: o.Text, IsCorrect: o.IsCorrect})
	}
	for _, t := range q.Tables {
		in.Tables = append(in.Tables, TableInput{Page: t.Page, Markdown: t.Markdown})
	}
	for _, d := range q.Diagrams {
		in.Diagrams = append(in.Diagrams, DiagramInput{Page: d.Page, Region: d.Region, Description: d.Description, Confirmed: d.Confirmed})
	}
	for _, w := range q.Warnings {
		if w.Blocking {
			in.AcknowledgedWarnings = append(in.AcknowledgedWarnings, w.Code)
		}
	}
	return in
}

// makeApprovable gives every draft question marks and acknowledges its warnings,
// as a teacher finishing a review would.
func (h *harness) makeApprovable(docID uuid.UUID) {
	h.t.Helper()
	ctx := context.Background()
	draft, err := h.svc.GetDraft(ctx, h.examID, docID, h.teacher)
	require.NoError(h.t, err)
	v := draft.Document.DraftVersion
	for _, q := range draft.Questions {
		in := inputOf(q)
		if in.Points == nil {
			in.Points = ptr(2)
		}
		res, err := h.svc.ReplaceQuestion(ctx, h.examID, docID, q.ID, ReplaceQuestionRequest{ExpectedDraftVersion: v, Question: in}, h.teacher)
		require.NoError(h.t, err)
		v = res.DraftVersion
	}
}

func TestGetDraft_OnlyOnceThereIsOne(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.upload()
	_, err := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	assert.ErrorIs(t, err, ErrInvalidState, "queued: nothing to show yet")

	require.NoError(t, h.svc.Process(ctx, d.ID, 1, false))
	draft, err := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	require.NoError(t, err)
	require.Len(t, draft.Questions, 2)
	assert.Equal(t, StatusReviewRequired, draft.Document.Status)

	q := draft.Questions[0]
	assert.Equal(t, 1, q.Position)
	assert.Equal(t, extract.TypeMCQ, q.Type)
	assert.Equal(t, OriginExtracted, q.Origin)
	assert.Equal(t, []int{1}, q.SourcePages, "each question keeps the page it came from")
	assert.Nil(t, q.Points, "marks are the teacher's to set")
	for _, o := range q.Options {
		assert.False(t, o.IsCorrect, "no answer key is inferred")
	}
}

func TestAddQuestion_AtTheEndAndInTheMiddle(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.processed()
	v := d.DraftVersion

	end, err := h.svc.AddQuestion(ctx, h.examID, d.ID, AddQuestionRequest{ExpectedDraftVersion: v, Question: validInput()}, h.teacher)
	require.NoError(t, err)
	assert.Equal(t, 3, end.Question.Position)
	assert.Equal(t, OriginTeacher, end.Question.Origin)
	assert.False(t, end.Question.Edited)
	assert.Equal(t, v+1, end.DraftVersion)
	assert.Equal(t, "A", end.Question.Options[0].Label, "labels are stored in upper case")

	in := validInput()
	in.Text = "A question squeezed in at the front"
	mid, err := h.svc.AddQuestion(ctx, h.examID, d.ID, AddQuestionRequest{ExpectedDraftVersion: end.DraftVersion, Position: 1, Question: in}, h.teacher)
	require.NoError(t, err)
	assert.Equal(t, 1, mid.Question.Position)

	draft, _ := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	var texts []string
	for i, q := range draft.Questions {
		assert.Equal(t, i+1, q.Position, "positions stay 1..n")
		texts = append(texts, q.Text)
	}
	assert.Equal(t, "A question squeezed in at the front", texts[0])
	assert.Equal(t, q1Text, texts[1])
	assert.Empty(t, h.w.questions, "a draft is not part of the exam")
}

func TestEditing_NeedsTheCurrentVersion(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.processed()
	draft, _ := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	qid := draft.Questions[0].ID
	v := d.DraftVersion

	_, err := h.svc.AddQuestion(ctx, h.examID, d.ID, AddQuestionRequest{ExpectedDraftVersion: v - 1, Question: validInput()}, h.teacher)
	assert.ErrorIs(t, err, ErrStaleVersion)
	_, err = h.svc.ReplaceQuestion(ctx, h.examID, d.ID, qid, ReplaceQuestionRequest{ExpectedDraftVersion: v + 5, Question: validInput()}, h.teacher)
	assert.ErrorIs(t, err, ErrStaleVersion)
	_, err = h.svc.DeleteQuestion(ctx, h.examID, d.ID, qid, v-1, h.teacher)
	assert.ErrorIs(t, err, ErrStaleVersion)
	_, err = h.svc.Reorder(ctx, h.examID, d.ID, ReorderRequest{ExpectedDraftVersion: v - 1}, h.teacher)
	assert.ErrorIs(t, err, ErrStaleVersion)

	after, _ := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	assert.Equal(t, v, after.Document.DraftVersion, "refused edits change nothing, including the version")
	assert.Len(t, after.Questions, 2)
}

func TestEditing_ConcurrentEditsOnTheSameVersionLetExactlyOneWin(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.processed()
	draft, _ := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	qid := draft.Questions[0].ID

	var wins, stale int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := validInput()
			in.Text = strings.Repeat("x", i+2)
			_, err := h.svc.ReplaceQuestion(ctx, h.examID, d.ID, qid, ReplaceQuestionRequest{ExpectedDraftVersion: d.DraftVersion, Question: in}, h.teacher)
			switch err {
			case nil:
				atomic.AddInt32(&wins, 1)
			case ErrStaleVersion:
				atomic.AddInt32(&stale, 1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	assert.EqualValues(t, 1, wins, "the others are told the draft moved on instead of silently overwriting")
	assert.EqualValues(t, 9, stale)
}

func TestReplaceQuestion_ValidatesEverythingOnTheServer(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.processed()
	draft, _ := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	qid := draft.Questions[0].ID

	mutate := func(f func(*QuestionInput)) QuestionInput { in := validInput(); f(&in); return in }
	cases := map[string]struct {
		in    QuestionInput
		field string
	}{
		"text too short":        {mutate(func(i *QuestionInput) { i.Text = " a " }), "text"},
		"text too long":         {mutate(func(i *QuestionInput) { i.Text = strings.Repeat("é", 8001) }), "text"},
		"unknown type":          {mutate(func(i *QuestionInput) { i.Type = "VOICE" }), "type"},
		"negative points":       {mutate(func(i *QuestionInput) { i.Points = ptr(-1) }), "points"},
		"points too high":       {mutate(func(i *QuestionInput) { i.Points = ptr(1001) }), "points"},
		"options on an essay":   {mutate(func(i *QuestionInput) { i.Type = extract.TypeEssay }), "options"},
		"empty option label":    {mutate(func(i *QuestionInput) { i.Options[0].Label = " " }), "options[0].label"},
		"label with symbols":    {mutate(func(i *QuestionInput) { i.Options[0].Label = "A)" }), "options[0].label"},
		"label too long":        {mutate(func(i *QuestionInput) { i.Options[0].Label = "ABCDEFGHIJK" }), "options[0].label"},
		"duplicate labels":      {mutate(func(i *QuestionInput) { i.Options[1].Label = "A" }), "options[1].label"},
		"empty option text":     {mutate(func(i *QuestionInput) { i.Options[1].Text = "" }), "options[1].text"},
		"two correct options":   {mutate(func(i *QuestionInput) { i.Options[0].IsCorrect, i.Options[1].IsCorrect = true, true }), "options"},
		"too many options":      {mutate(func(i *QuestionInput) { i.Options = make([]OptionInput, 13) }), "options"},
		"table on a bad page":   {mutate(func(i *QuestionInput) { i.Tables = []TableInput{{Page: 9, Markdown: "| a |"}} }), "tables[0].page"},
		"empty table":           {mutate(func(i *QuestionInput) { i.Tables = []TableInput{{Page: 1, Markdown: "  "}} }), "tables[0].markdown"},
		"diagram on a bad page": {mutate(func(i *QuestionInput) { i.Diagrams = []DiagramInput{{Page: 0, Description: "x"}} }), "diagrams[0].page"},
		"confirming nothing":    {mutate(func(i *QuestionInput) { i.Diagrams = []DiagramInput{{Page: 1, Confirmed: true}} }), "diagrams[0].confirmed"},
		"label too long (num)":  {mutate(func(i *QuestionInput) { i.NumberLabel = strings.Repeat("9", 31) }), "numberLabel"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := h.svc.ReplaceQuestion(ctx, h.examID, d.ID, qid, ReplaceQuestionRequest{ExpectedDraftVersion: d.DraftVersion, Question: tc.in}, h.teacher)
			var ve *ValidationError
			require.ErrorAs(t, err, &ve)
			assert.Equal(t, tc.field, ve.Field)
		})
	}
	after, _ := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	assert.Equal(t, d.DraftVersion, after.Document.DraftVersion, "no invalid edit was applied or counted")
	assert.Equal(t, q1Text, after.Questions[0].Text)
}

func TestReplaceQuestion_UnknownQuestionIsNotFound(t *testing.T) {
	h := newHarness(t)
	d := h.processed()
	_, err := h.svc.ReplaceQuestion(context.Background(), h.examID, d.ID, uuid.New(),
		ReplaceQuestionRequest{ExpectedDraftVersion: d.DraftVersion, Question: validInput()}, h.teacher)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestReplaceQuestion_KeepsWhatTheSystemKnowsAndMarksTheEdit(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	// A reply whose wording is not on the page, so the question carries a blocking warning.
	h.model.Replies = []extracttest.Reply{{Body: extracttest.ReplyJSON(extracttest.Essay("1", "Describe photosynthesis in great detail.", 1))}}
	h.rebuild()
	d := h.processed()
	draft, _ := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	before := draft.Questions[0]
	require.Contains(t, warningCodes(before), extract.WarnUnverifiedText)

	in := inputOf(before)
	in.Text = "Describe the process of photosynthesis."
	in.AcknowledgedWarnings = nil
	res, err := h.svc.ReplaceQuestion(ctx, h.examID, d.ID, before.ID, ReplaceQuestionRequest{ExpectedDraftVersion: d.DraftVersion, Question: in}, h.teacher)
	require.NoError(t, err)
	q := res.Question
	assert.True(t, q.Edited)
	assert.Equal(t, before.SourcePages, q.SourcePages, "source pages cannot be edited")
	assert.Equal(t, before.Origin, q.Origin)
	assert.Equal(t, before.Confidence, q.Confidence)
	require.Equal(t, len(before.Warnings), len(q.Warnings), "a teacher cannot remove a warning, only acknowledge it")
	for _, w := range q.Warnings {
		assert.False(t, w.Acknowledged, "not acknowledged unless the request says so")
	}

	// Acknowledging names the warning's code.
	in.AcknowledgedWarnings = []string{extract.WarnUnverifiedText, "NOT_A_WARNING_ON_THIS_QUESTION"}
	res, err = h.svc.ReplaceQuestion(ctx, h.examID, d.ID, before.ID, ReplaceQuestionRequest{ExpectedDraftVersion: res.DraftVersion, Question: in}, h.teacher)
	require.NoError(t, err)
	for _, w := range res.Question.Warnings {
		assert.Equal(t, w.Code == extract.WarnUnverifiedText, w.Acknowledged)
	}
	assert.Len(t, res.Question.Warnings, len(before.Warnings), "an unknown code adds nothing")
}

func warningCodes(q DraftQuestion) []string {
	var out []string
	for _, w := range q.Warnings {
		out = append(out, w.Code)
	}
	return out
}

func TestReplaceQuestion_DiagramDescriptionSourceFollowsWhoWroteIt(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	body := `{"numberLabel": "1", "type": "ESSAY", "instructions": null, "text": "` + q2Text + `", "marks": null,
		"options": [], "tables": [], "diagrams": [{"page": 1, "region": "right", "description": "A block on a slope."}],
		"pages": [1], "continuesPrevious": false, "uncertain": false}`
	h.model.Replies = []extracttest.Reply{{Body: extracttest.ReplyJSON(body)}}
	h.rebuild()
	d := h.processed()
	draft, _ := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	q := draft.Questions[0]
	require.Equal(t, "MODEL", q.Diagrams[0].DescriptionSource)
	require.False(t, q.Diagrams[0].Confirmed)

	in := inputOf(q)
	in.Diagrams[0].Confirmed = true
	res, err := h.svc.ReplaceQuestion(ctx, h.examID, d.ID, q.ID, ReplaceQuestionRequest{ExpectedDraftVersion: d.DraftVersion, Question: in}, h.teacher)
	require.NoError(t, err)
	assert.Equal(t, "MODEL", res.Question.Diagrams[0].DescriptionSource, "confirming the model's wording does not make it the teacher's")
	assert.True(t, res.Question.Diagrams[0].Confirmed)

	in.Diagrams[0].Description = "A 2 kg block on a 30 degree slope."
	res, err = h.svc.ReplaceQuestion(ctx, h.examID, d.ID, q.ID, ReplaceQuestionRequest{ExpectedDraftVersion: res.DraftVersion, Question: in}, h.teacher)
	require.NoError(t, err)
	assert.Equal(t, "TEACHER", res.Question.Diagrams[0].DescriptionSource)
}

func TestDeleteQuestion_ClosesTheGap(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.processed()
	draft, _ := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)

	after, err := h.svc.DeleteQuestion(ctx, h.examID, d.ID, draft.Questions[0].ID, d.DraftVersion, h.teacher)
	require.NoError(t, err)
	require.Len(t, after.Questions, 1)
	assert.Equal(t, 1, after.Questions[0].Position)
	assert.Equal(t, q2Text, after.Questions[0].Text)
	assert.Equal(t, d.DraftVersion+1, after.Document.DraftVersion)

	_, err = h.svc.DeleteQuestion(ctx, h.examID, d.ID, uuid.New(), after.Document.DraftVersion, h.teacher)
	assert.ErrorIs(t, err, ErrNotFound)
	check, _ := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	assert.Equal(t, after.Document.DraftVersion, check.Document.DraftVersion, "a failed delete does not bump the version")
}

func TestReorder(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.processed()
	draft, _ := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	a, b := draft.Questions[0].ID, draft.Questions[1].ID

	got, err := h.svc.Reorder(ctx, h.examID, d.ID, ReorderRequest{ExpectedDraftVersion: d.DraftVersion, QuestionIDs: []uuid.UUID{b, a}}, h.teacher)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{b, a}, []uuid.UUID{got.Questions[0].ID, got.Questions[1].ID})
	assert.Equal(t, []int{1, 2}, []int{got.Questions[0].Position, got.Questions[1].Position})

	v := got.Document.DraftVersion
	bad := map[string][]uuid.UUID{
		"missing one": {a},
		"duplicate":   {a, a},
		"a stranger":  {a, uuid.New()},
		"one extra":   {a, b, uuid.New()},
		"empty":       {},
		"nil":         nil,
	}
	for name, ids := range bad {
		_, err := h.svc.Reorder(ctx, h.examID, d.ID, ReorderRequest{ExpectedDraftVersion: v, QuestionIDs: ids}, h.teacher)
		var ve *ValidationError
		assert.ErrorAs(t, err, &ve, name)
	}
	check, _ := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	assert.Equal(t, b, check.Questions[0].ID, "invalid orders change nothing")
	assert.Equal(t, v, check.Document.DraftVersion)
}

func TestEditing_NotWhileProcessingOrAfterApproval(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.upload()
	_, err := h.svc.AddQuestion(ctx, h.examID, d.ID, AddQuestionRequest{Question: validInput()}, h.teacher)
	assert.ErrorIs(t, err, ErrInvalidState, "no draft yet")

	require.NoError(t, h.svc.Process(ctx, d.ID, 1, false))
	h.makeApprovable(d.ID)
	cur := h.doc(d.ID)
	_, err = h.svc.Approve(ctx, h.examID, d.ID, cur.DraftVersion, h.teacher)
	require.NoError(t, err)

	done := h.doc(d.ID)
	_, err = h.svc.AddQuestion(ctx, h.examID, d.ID, AddQuestionRequest{ExpectedDraftVersion: done.DraftVersion, Question: validInput()}, h.teacher)
	assert.ErrorIs(t, err, ErrAlreadyApproved, "an approved draft is a record, not a workspace")
	draft, err := h.svc.GetDraft(ctx, h.examID, d.ID, h.teacher)
	require.NoError(t, err, "but it can still be read")
	assert.Len(t, draft.Questions, 2)
}

func TestAddQuestion_DraftSizeIsBounded(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.processed()
	v := d.DraftVersion
	for i := 0; i < maxDraftQuestions-2; i++ {
		r, err := h.svc.AddQuestion(ctx, h.examID, d.ID, AddQuestionRequest{ExpectedDraftVersion: v, Question: validInput()}, h.teacher)
		require.NoError(t, err)
		v = r.DraftVersion
	}
	_, err := h.svc.AddQuestion(ctx, h.examID, d.ID, AddQuestionRequest{ExpectedDraftVersion: v, Question: validInput()}, h.teacher)
	assert.ErrorIs(t, err, ErrDraftTooLarge)
}
