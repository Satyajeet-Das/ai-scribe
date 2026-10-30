package extract_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract/extracttest"
)

// replyJSON wraps question objects in the reply envelope.
func replyJSON(questions ...string) string {
	return `{"documentInstructions": null, "questions": [` + strings.Join(questions, ",") + `], "unassigned": []}`
}

func mcqJSON(label, text string, page int, opts ...string) string {
	var os []string
	for i, o := range opts {
		os = append(os, fmt.Sprintf(`{"label": %q, "text": %q}`, string(rune('A'+i)), o))
	}
	return fmt.Sprintf(`{"numberLabel": %q, "type": "MCQ", "instructions": null, "text": %q, "marks": null,
		"options": [%s], "tables": [], "diagrams": [], "pages": [%d], "continuesPrevious": false, "uncertain": false}`,
		label, text, strings.Join(os, ","), page)
}

func essayJSON(label, text string, page int) string {
	return fmt.Sprintf(`{"numberLabel": %q, "type": "ESSAY", "instructions": null, "text": %q, "marks": null,
		"options": [], "tables": [], "diagrams": [], "pages": [%d], "continuesPrevious": false, "uncertain": false}`,
		label, text, page)
}

func textPage(n int, text string) extract.Page {
	return extract.Page{Number: n, Kind: extract.KindText, Text: text}
}

func run(t *testing.T, m *extracttest.Model, opts extract.Options, pages ...extract.Page) (*extract.Result, error) {
	t.Helper()
	return extract.New(m, opts).Extract(context.Background(), extract.Input{Pages: pages}, nil)
}

func codes(q extract.Question) []string {
	var out []string
	for _, w := range q.Warnings {
		out = append(out, w.Code)
	}
	return out
}

const page1 = "1. What is the SI unit of force?\nA. Newton\nB. Joule\nC. Watt\n2. Explain Newton's first law of motion."

func TestExtract_MCQAndDescriptive(t *testing.T) {
	m := &extracttest.Model{Replies: []extracttest.Reply{{Body: replyJSON(
		mcqJSON("1", "What is the SI unit of force?", 1, "Newton", "Joule", "Watt"),
		essayJSON("2", "Explain Newton's first law of motion.", 1),
	)}}}
	res, err := run(t, m, extract.Options{}, textPage(1, page1))
	require.NoError(t, err)
	require.Len(t, res.Questions, 2)

	q := res.Questions[0]
	assert.Equal(t, extract.TypeMCQ, q.Type)
	assert.Equal(t, "1", q.NumberLabel)
	assert.Equal(t, []int{1}, q.SourcePages)
	assert.Equal(t, extract.ConfidenceHigh, q.Confidence)
	assert.Empty(t, q.Warnings, "wording that is on the page raises nothing")
	require.Len(t, q.Options, 3)
	assert.Equal(t, "A", q.Options[0].Label)
	assert.Equal(t, "Newton", q.Options[0].Text)
	for _, o := range q.Options {
		assert.False(t, o.IsCorrect, "extraction never marks a correct answer")
	}
	assert.Equal(t, extract.TypeEssay, res.Questions[1].Type)
	assert.Nil(t, res.Questions[1].Points, "marks are not invented")
}

func TestExtract_NeverMarksACorrectAnswer_EvenIfTheModelSendsOne(t *testing.T) {
	body := `{"documentInstructions": null, "unassigned": [], "questions": [{
		"numberLabel": "1", "type": "MCQ", "instructions": null, "text": "What is the SI unit of force?", "marks": null,
		"options": [{"label": "A", "text": "Newton", "isCorrect": true}, {"label": "B", "text": "Joule", "correct": true}],
		"tables": [], "diagrams": [], "pages": [1], "continuesPrevious": false, "uncertain": false, "answer": "A"}]}`
	res, err := run(t, &extracttest.Model{Replies: []extracttest.Reply{{Body: body}}}, extract.Options{}, textPage(1, page1))
	require.NoError(t, err)
	for _, o := range res.Questions[0].Options {
		assert.False(t, o.IsCorrect)
	}
}

func TestExtract_WordingNotOnThePageIsFlaggedNotRemoved(t *testing.T) {
	m := &extracttest.Model{Replies: []extracttest.Reply{{Body: replyJSON(
		mcqJSON("1", "What is the SI unit of force?", 1, "Newton", "Joule", "Kilogram per second squared"),
	)}}}
	res, err := run(t, m, extract.Options{}, textPage(1, page1))
	require.NoError(t, err)

	q := res.Questions[0]
	assert.Contains(t, codes(q), extract.WarnUnverifiedText)
	assert.Equal(t, extract.ConfidenceLow, q.Confidence)
	require.Len(t, q.Options, 3, "the doubtful option is kept for the teacher to judge")
	var blocking bool
	for _, w := range q.Warnings {
		if w.Code == extract.WarnUnverifiedText {
			blocking = w.Blocking
			assert.Contains(t, w.Message, "option C")
			assert.NotContains(t, w.Message, "Kilogram", "a warning never repeats content")
		}
	}
	assert.True(t, blocking, "unverified wording must be acknowledged before approval")
}

func TestExtract_ReflowedHyphenatedAndMathsTextStillMatches(t *testing.T) {
	page := "3. Show that the area of the square with side x cm is x² square\ncentimetres, and that the perimeter is\nextra-\nordinary only when x = 0."
	m := &extracttest.Model{Replies: []extracttest.Reply{{Body: replyJSON(
		essayJSON("3", "Show that the area of the square with side x cm is x2 square centimetres, and that the perimeter is extraordinary only when x = 0.", 1),
	)}}}
	res, err := run(t, m, extract.Options{}, textPage(1, page))
	require.NoError(t, err)
	assert.Empty(t, res.Questions[0].Warnings, "line breaks, hyphenation and x² vs x2 are not differences in wording")
}

func TestExtract_UnicodeIsPreserved(t *testing.T) {
	page := "1. Résolvez l'équation 2x² − 3x + 1 = 0 pour x ∈ ℝ."
	m := &extracttest.Model{Replies: []extracttest.Reply{{Body: replyJSON(
		essayJSON("1", "Résolvez l'équation 2x² − 3x + 1 = 0 pour x ∈ ℝ.", 1))}}}
	res, err := run(t, m, extract.Options{}, textPage(1, page))
	require.NoError(t, err)
	assert.Equal(t, "Résolvez l'équation 2x² − 3x + 1 = 0 pour x ∈ ℝ.", res.Questions[0].Text)
	assert.Empty(t, res.Questions[0].Warnings)
}

func TestExtract_FoundOnAnotherPageIsAMismatchNotInvention(t *testing.T) {
	m := &extracttest.Model{Replies: []extracttest.Reply{{Body: replyJSON(
		essayJSON("2", "Describe the water cycle in your own words.", 1))}}}
	res, err := run(t, m, extract.Options{},
		textPage(1, "1. Define evaporation and condensation precisely."),
		textPage(2, "2. Describe the water cycle in your own words."))
	require.NoError(t, err)
	assert.Contains(t, codes(res.Questions[0]), extract.WarnPageMismatch)
	assert.NotContains(t, codes(res.Questions[0]), extract.WarnUnverifiedText)
}

func TestExtract_MarksNotOnThePageAreDropped(t *testing.T) {
	body := `{"documentInstructions": null, "unassigned": [], "questions": [` +
		strings.Replace(essayJSON("1", "Explain Newton's first law of motion.", 1), `"marks": null`, `"marks": 8`, 1) + `]}`
	res, err := run(t, &extracttest.Model{Replies: []extracttest.Reply{{Body: body}}}, extract.Options{},
		textPage(1, "1. Explain Newton's first law of motion."))
	require.NoError(t, err)
	assert.Nil(t, res.Questions[0].Points)
	assert.Contains(t, codes(res.Questions[0]), extract.WarnMarksNotOnPage)

	res, err = run(t, &extracttest.Model{Replies: []extracttest.Reply{{Body: body}}}, extract.Options{},
		textPage(1, "1. Explain Newton's first law of motion. [8 marks]"))
	require.NoError(t, err)
	require.NotNil(t, res.Questions[0].Points)
	assert.Equal(t, 8, *res.Questions[0].Points, "marks that the paper prints are kept")
}

func TestExtract_InvalidReplies(t *testing.T) {
	cases := map[string]string{
		"not json":              "I could not find any questions, sorry.",
		"empty":                 "",
		"truncated json":        `{"documentInstructions": null, "questions": [{"numberLabel": "1"`,
		"no questions list":     `{"documentInstructions": null, "unassigned": []}`,
		"questions wrong shape": `{"questions": "none"}`,
		"array":                 `[1,2,3]`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			m := &extracttest.Model{Replies: []extracttest.Reply{{Body: body}}}
			_, err := run(t, m, extract.Options{}, textPage(1, page1))
			require.Error(t, err)
			code, transient := extract.CodeOf(err)
			assert.Equal(t, extract.CodeInvalidOutput, code)
			assert.False(t, transient, "a model that cannot follow the format will not do better on retry")
			assert.ErrorIs(t, err, extract.ErrPermanent)
			assert.Equal(t, 2, m.Calls(), "an unusable reply is asked for once more, no more")
			assert.NotContains(t, err.Error(), "sorry", "provider output is not put in errors")
		})
	}
}

func TestExtract_AnUnusableReplyIsRetriedOnce(t *testing.T) {
	m := &extracttest.Model{Replies: []extracttest.Reply{
		{Body: "Here you go:"},
		{Body: "```json\n" + replyJSON(essayJSON("2", "Explain Newton's first law of motion.", 1)) + "\n```"},
	}}
	res, err := run(t, m, extract.Options{}, textPage(1, page1))
	require.NoError(t, err)
	assert.Len(t, res.Questions, 1)
	assert.Equal(t, 2, m.Calls())
}

func TestExtract_QuestionProblemsBecomeWarningsNotLoss(t *testing.T) {
	body := replyJSON(
		// Unknown type with options: inferred, and said so.
		`{"numberLabel": "1", "type": "FILL_IN", "instructions": null, "text": "What is the SI unit of force?", "marks": null,
		  "options": [{"label": "(a)", "text": "Newton"}, {"label": "a.", "text": "Joule"}],
		  "tables": [], "diagrams": [{"page": 99, "region": null, "description": null}], "pages": [42], "continuesPrevious": false, "uncertain": true}`,
	)
	res, err := run(t, &extracttest.Model{Replies: []extracttest.Reply{{Body: body}}}, extract.Options{}, textPage(1, page1))
	require.NoError(t, err)
	q := res.Questions[0]
	assert.Equal(t, extract.TypeMCQ, q.Type)
	assert.Equal(t, []int{1}, q.SourcePages, "a page the batch did not cover is replaced, with a warning")
	assert.Equal(t, 1, q.Diagrams[0].Page)
	assert.Empty(t, q.Diagrams[0].Description)
	assert.False(t, q.Diagrams[0].Confirmed)
	assert.Equal(t, "A", q.Options[0].Label, "(a) is stored as A")
	assert.ElementsMatch(t, []string{
		extract.WarnTypeUncertain, extract.WarnPageUnknown, extract.WarnDuplicateOption,
		extract.WarnDiagramNeedsText, extract.WarnModelUncertain,
	}, codes(q))
	assert.Equal(t, extract.ConfidenceMedium, q.Confidence)
}

func TestExtract_DiagramDescriptionIsNeverConfirmed(t *testing.T) {
	body := replyJSON(`{"numberLabel": "4", "type": "ESSAY", "instructions": null, "text": "Use Figure 1 to find the current.", "marks": null,
		"options": [], "tables": [{"page": 1, "markdown": "| V | I |\n|---|---|\n| 2 | 0.5 |"}],
		"diagrams": [{"page": 1, "region": "top right", "description": "A circuit with a 4 ohm resistor and a cell."}],
		"pages": [1], "continuesPrevious": false, "uncertain": false}`)
	res, err := run(t, &extracttest.Model{Replies: []extracttest.Reply{{Body: body}}}, extract.Options{},
		textPage(1, "4. Use Figure 1 to find the current.\nFigure 1"))
	require.NoError(t, err)
	d := res.Questions[0].Diagrams[0]
	assert.Equal(t, "MODEL", d.DescriptionSource)
	assert.False(t, d.Confirmed, "only a teacher confirms a diagram description")
	assert.Equal(t, "top right", d.Region)
	assert.Contains(t, res.Questions[0].Tables[0].Markdown, "| V | I |")
}

func TestExtract_QuestionRunningAcrossBatchesIsJoined(t *testing.T) {
	first := "5. Read the passage and answer.\nThe passage describes a long journey across the desert and the people met along the way."
	second := "A. The desert\nB. The journey\nC. The people"
	body1 := replyJSON(essayJSON("5", "Read the passage and answer.", 1))
	body2 := replyJSON(`{"numberLabel": "", "type": "MCQ", "instructions": null, "text": "", "marks": null,
		"options": [{"label": "A", "text": "The desert"}, {"label": "B", "text": "The journey"}, {"label": "C", "text": "The people"}],
		"tables": [], "diagrams": [], "pages": [2], "continuesPrevious": true, "uncertain": false}`)
	m := &extracttest.Model{Replies: []extracttest.Reply{{Body: body1}, {Body: body2}}}

	// A tiny request size puts each page in its own call.
	res, err := run(t, m, extract.Options{MaxCharsPerRequest: 50}, textPage(1, first), textPage(2, second))
	require.NoError(t, err)
	require.Equal(t, 2, m.Calls())
	require.Len(t, res.Questions, 1, "the continuation is part of question 5, not a question of its own")
	q := res.Questions[0]
	assert.Len(t, q.Options, 3)
	assert.Equal(t, []int{1, 2}, q.SourcePages)
	assert.Equal(t, "5", q.NumberLabel)
}

func TestExtract_ContinuationWithNothingBeforeIt(t *testing.T) {
	body := replyJSON(`{"numberLabel": "", "type": "ESSAY", "instructions": null, "text": "and give two examples.", "marks": null,
		"options": [], "tables": [], "diagrams": [], "pages": [1], "continuesPrevious": true, "uncertain": false}`)
	res, err := run(t, &extracttest.Model{Replies: []extracttest.Reply{{Body: body}}}, extract.Options{},
		textPage(1, "and give two examples."))
	require.NoError(t, err)
	require.Len(t, res.Questions, 1)
	assert.Contains(t, codes(res.Questions[0]), extract.WarnContinuationOrphan)
}

func TestExtract_NumberingProblemsAreFlagged(t *testing.T) {
	page := "1. First question here.\n2. Second question here.\n2. Another second question.\n5. Fifth question here."
	m := &extracttest.Model{Replies: []extracttest.Reply{{Body: replyJSON(
		essayJSON("1", "First question here.", 1), essayJSON("2", "Second question here.", 1),
		essayJSON("2", "Another second question.", 1), essayJSON("5", "Fifth question here.", 1),
	)}}}
	res, err := run(t, m, extract.Options{}, textPage(1, page))
	require.NoError(t, err)
	require.Len(t, res.Questions, 4, "nothing is dropped")
	assert.Contains(t, codes(res.Questions[2]), extract.WarnDuplicateNumber)
	assert.Contains(t, codes(res.Questions[3]), extract.WarnNumberingGap)
}

func TestExtract_QuestionWithoutWordingGoesToUnassigned(t *testing.T) {
	body := replyJSON(
		`{"numberLabel": "7", "type": "MCQ", "instructions": null, "text": "  ", "marks": null,
		  "options": [{"label": "A", "text": "Red"}, {"label": "B", "text": "Blue"}],
		  "tables": [], "diagrams": [], "pages": [1], "continuesPrevious": false, "uncertain": false}`,
		essayJSON("8", "Explain Newton's first law of motion.", 1),
	)
	res, err := run(t, &extracttest.Model{Replies: []extracttest.Reply{{Body: body}}}, extract.Options{}, textPage(1, page1))
	require.NoError(t, err)
	require.Len(t, res.Questions, 1)
	require.NotEmpty(t, res.Unassigned)
	last := res.Unassigned[len(res.Unassigned)-1]
	assert.Contains(t, last.Text, "Red")
	assert.Equal(t, 1, last.Page)
}

func TestExtract_DocumentInstructionsAndUnassignedContent(t *testing.T) {
	body := `{"documentInstructions": "Answer ALL questions. Time: 2 hours.", "questions": [` +
		essayJSON("1", "Explain Newton's first law of motion.", 1) + `],
		"unassigned": [{"page": 1, "text": "Formula sheet: v = u + at", "reason": "Not a question"}, {"page": 77, "text": "", "reason": "empty"}]}`
	res, err := run(t, &extracttest.Model{Replies: []extracttest.Reply{{Body: body}}}, extract.Options{}, textPage(1, page1))
	require.NoError(t, err)
	assert.Equal(t, "Answer ALL questions. Time: 2 hours.", res.DocumentInstructions)
	require.Len(t, res.Unassigned, 1, "an empty note is dropped")
	assert.Equal(t, "Formula sheet: v = u + at", res.Unassigned[0].Text)
}

func TestExtract_NoQuestionsIsAFailureNotAnEmptyDraft(t *testing.T) {
	_, err := run(t, &extracttest.Model{Replies: []extracttest.Reply{{Body: replyJSON()}}}, extract.Options{}, textPage(1, "Cover sheet"))
	code, transient := extract.CodeOf(err)
	assert.Equal(t, extract.CodeNothingFound, code)
	assert.False(t, transient)
}

func TestExtract_ScannedPages(t *testing.T) {
	scan := extract.Page{Number: 2, Kind: extract.KindImageOnly, Images: []extract.Image{{Format: "png", Data: []byte{1, 2, 3}}}}
	reply := replyJSON(essayJSON("9", "Describe the carbon cycle in detail.", 2))

	t.Run("without a vision model the page is reported, not read", func(t *testing.T) {
		m := &extracttest.Model{Replies: []extracttest.Reply{{Body: replyJSON(essayJSON("2", "Explain Newton's first law of motion.", 1))}}}
		res, err := run(t, m, extract.Options{}, textPage(1, page1), scan)
		require.NoError(t, err)
		assert.Equal(t, 1, m.Calls(), "the picture page was not sent anywhere")
		require.Len(t, res.Unassigned, 1)
		assert.Equal(t, 2, res.Unassigned[0].Page)
		assert.Contains(t, res.Unassigned[0].Reason, "no vision model")
	})

	t.Run("with a vision model it is read and marked as such", func(t *testing.T) {
		m := &extracttest.Model{Vision: true, Replies: []extracttest.Reply{{Body: replyJSON(essayJSON("2", "Explain Newton's first law of motion.", 1))}, {Body: reply}}}
		res, err := run(t, m, extract.Options{}, textPage(1, page1), scan)
		require.NoError(t, err)
		require.Equal(t, 2, m.Calls())
		assert.True(t, m.Requests[1].Vision)
		require.Len(t, res.Questions, 2)
		q := res.Questions[1]
		assert.Contains(t, codes(q), extract.WarnReadFromImage)
		assert.NotEqual(t, extract.ConfidenceHigh, q.Confidence, "a reading of a picture is never high confidence")
	})

	t.Run("only a scan: nothing readable", func(t *testing.T) {
		_, err := run(t, &extracttest.Model{}, extract.Options{}, scan)
		code, _ := extract.CodeOf(err)
		assert.Equal(t, extract.CodeNothingFound, code)
	})
}

func TestExtract_ProviderFailuresKeepTheirClass(t *testing.T) {
	cases := []struct {
		err       error
		code      string
		transient bool
	}{
		{extract.NewError(extract.CodeRateLimited, true, "429"), extract.CodeRateLimited, true},
		{extract.NewError(extract.CodeUnavailable, true, "503"), extract.CodeUnavailable, true},
		{extract.NewError(extract.CodeModelNotFound, false, "gone"), extract.CodeModelNotFound, false},
		{context.DeadlineExceeded, extract.CodeTimeout, true},
		{errors.New("something unexpected"), extract.CodeInternal, false},
	}
	for _, tc := range cases {
		m := &extracttest.Model{Replies: []extracttest.Reply{{Err: tc.err}}}
		_, err := run(t, m, extract.Options{}, textPage(1, page1))
		require.Error(t, err)
		code, transient := extract.CodeOf(err)
		assert.Equal(t, tc.code, code)
		assert.Equal(t, tc.transient, transient)
		assert.Equal(t, 1, m.Calls(), "a failing call is not repeated here; the job's retry policy decides")
	}
}

func TestExtract_StopsWhenCancelled(t *testing.T) {
	block := make(chan struct{})
	m := &extracttest.Model{Block: block, Replies: []extracttest.Reply{{Body: replyJSON(essayJSON("1", "x", 1))}}}
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := extract.New(m, extract.Options{}).Extract(ctx, extract.Input{Pages: []extract.Page{textPage(1, page1)}}, nil)
		errc <- err
	}()
	cancel()
	assert.ErrorIs(t, <-errc, context.Canceled)
}

func TestExtract_ReportsProgressAndBoundsTheWork(t *testing.T) {
	m := &extracttest.Model{Replies: []extracttest.Reply{{Body: replyJSON(essayJSON("1", "Explain Newton's first law of motion.", 1))}}}
	var seen [][2]int
	_, err := extract.New(m, extract.Options{MaxCharsPerRequest: 30}).Extract(context.Background(),
		extract.Input{Pages: []extract.Page{textPage(1, page1), textPage(2, page1), textPage(3, page1)}},
		func(done, total int) { seen = append(seen, [2]int{done, total}) })
	require.NoError(t, err)
	assert.Equal(t, [][2]int{{0, 3}, {1, 3}, {2, 3}, {3, 3}}, seen)

	_, err = extract.New(m, extract.Options{MaxCharsPerRequest: 30, MaxBatches: 2}).Extract(context.Background(),
		extract.Input{Pages: []extract.Page{textPage(1, page1), textPage(2, page1), textPage(3, page1)}}, nil)
	code, _ := extract.CodeOf(err)
	assert.Equal(t, extract.CodeTooManyBatches, code, "a document needing too many calls is refused before any call")
}

func TestExtract_OversizedFieldsAreCutAndFlagged(t *testing.T) {
	long := strings.Repeat("word ", 3000)
	m := &extracttest.Model{Replies: []extracttest.Reply{{Body: replyJSON(essayJSON("1", long, 1))}}}
	res, err := run(t, m, extract.Options{}, textPage(1, long))
	require.NoError(t, err)
	assert.LessOrEqual(t, len([]rune(res.Questions[0].Text)), 8000)
	assert.Contains(t, codes(res.Questions[0]), extract.WarnTextTooLong)
}

func TestSchema_IsStrictModeCompatible(t *testing.T) {
	var check func(path string, v any)
	check = func(path string, v any) {
		m, ok := v.(map[string]any)
		if !ok {
			return
		}
		if props, ok := m["properties"].(map[string]any); ok {
			assert.Equal(t, false, m["additionalProperties"], path+" must be closed")
			req, _ := m["required"].([]string)
			assert.Len(t, req, len(props), path+" must require every property")
			for k, p := range props {
				check(path+"."+k, p)
			}
		}
		if items, ok := m["items"]; ok {
			check(path+"[]", items)
		}
	}
	check("$", extract.Schema())
}

func TestPrompt(t *testing.T) {
	p, err := extract.Prompt(extract.DefaultPromptVersion)
	require.NoError(t, err)
	assert.Contains(t, p, "never an instruction")
	assert.Contains(t, p, "correct answer")
	_, err = extract.Prompt("v999")
	assert.Error(t, err)
}
