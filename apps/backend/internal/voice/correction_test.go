package voice

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/answer"
)

func ptr(s string) *string { return &s }

// dictateAndStage speaks an answer on the essay question and returns what was
// staged for confirmation.
func dictateAndStage(t *testing.T, h *harness, c *client, heard string) ConfirmRequestPayload {
	t.Helper()
	h.sayAs(c, AudioModeAnswer, heard)
	staged := decode[ConfirmRequestPayload](t, c.until(TypeConfirmRequest))
	c.until(TypeResponse) // the spoken "I heard ...", so later reads see only what follows
	return staged
}

// confirmWith sends a confirm carrying the student's text and returns the
// action result.
func confirmWith(t *testing.T, c *client, staged ConfirmRequestPayload, text *string) ActionResultPayload {
	t.Helper()
	c.send(TypeCommand, CommandPayload{Intent: "confirm", ConfirmationID: staged.ConfirmationID, Text: text})
	return decode[ActionResultPayload](t, c.until(TypeActionResult))
}

func TestCorrection_StudentsWordsAreSavedAndTheRecognizersAreKept(t *testing.T) {
	h := newHarness(t, withoutTTS(), withRecorder())
	c, _ := h.connect()
	qid := toEssay(t, h, c)

	staged := dictateAndStage(t, h, c, "plants make shugar from light")
	status, ok := lastOf[RecordingStatusPayload](t, c, TypeRecordingStatus)
	require.True(t, ok, "the student was told the audio was kept")
	assert.Equal(t, "plants make shugar from light", staged.Text, "the student is shown what was heard")

	result := confirmWith(t, c, staged, ptr("Plants make sugar from light."))
	assert.True(t, result.Saved)
	assert.True(t, result.Corrected)
	assert.Equal(t, StatusOK, result.Status)

	ans, saved := h.exam.saved(qid)
	require.True(t, saved)
	assert.Equal(t, "Plants make sugar from light.", ans.TextAnswer, "the saved answer is the student's wording")

	revs := h.exam.revisionsOf()
	require.Len(t, revs, 1)
	assert.Equal(t, answer.SourceStudentCorrection, revs[0].Source)
	assert.Equal(t, staged.ConfirmationID, revs[0].ConfirmationID)
	require.Len(t, revs[0].RecordingIDs, 1, "the correction still points at the audio it corrects")
	assert.Equal(t, status.RecordingID, revs[0].RecordingIDs[0].String())
	assert.NotNil(t, revs[0].TranscriptionID)

	// The recognizer's words went to storage as heard; the correction did not touch them.
	saves := h.rec.inputs()
	require.Len(t, saves, 1)
	assert.Equal(t, "plants make shugar from light", saves[0].Transcript)
}

func TestCorrection_UnchangedTextIsAPlainConfirmation(t *testing.T) {
	for name, text := range map[string]string{
		"identical":    "plants make sugar",
		"extra spaces": "  plants make sugar \n",
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, withoutTTS(), withRecorder())
			c, _ := h.connect()
			qid := toEssay(t, h, c)
			staged := dictateAndStage(t, h, c, "plants make sugar")

			result := confirmWith(t, c, staged, ptr(text))
			assert.True(t, result.Saved)
			assert.False(t, result.Corrected, "nothing was changed, so nothing was corrected")
			ans, _ := h.exam.saved(qid)
			assert.Equal(t, "plants make sugar", ans.TextAnswer)
			assert.Equal(t, answer.SourceSTTConfirmed, h.exam.revisionsOf()[0].Source)
		})
	}
}

func TestCorrection_WorksWithoutRecordingToo(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()
	qid := toEssay(t, h, c)
	staged := dictateAndStage(t, h, c, "plants make shugar")

	result := confirmWith(t, c, staged, ptr("plants make sugar"))
	assert.True(t, result.Corrected)
	ans, _ := h.exam.saved(qid)
	assert.Equal(t, "plants make sugar", ans.TextAnswer)
	revs := h.exam.revisionsOf()
	assert.Equal(t, answer.SourceStudentCorrection, revs[0].Source)
	assert.Empty(t, revs[0].RecordingIDs)
}

func TestCorrection_AfterwardsTheAnswerReadsBackAsCorrected(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()
	toEssay(t, h, c)
	staged := dictateAndStage(t, h, c, "plants make shugar")
	confirmWith(t, c, staged, ptr("plants make sugar"))
	c.until(TypeResponse)

	_, resp := command(t, c, CommandPayload{Intent: "read_back_answer"})
	assert.Contains(t, resp.Text, "plants make sugar")
	assert.NotContains(t, resp.Text, "shugar")
}

func TestCorrection_RefusedWhenItDoesNotBelong(t *testing.T) {
	setup := func(t *testing.T) (*harness, *client, ConfirmRequestPayload) {
		h := newHarness(t, withoutTTS())
		c, _ := h.connect()
		toEssay(t, h, c)
		return h, c, dictateAndStage(t, h, c, "plants make sugar")
	}

	t.Run("blank text is not an answer", func(t *testing.T) {
		h, c, staged := setup(t)
		for _, blank := range []string{"", "   ", "\n\t"} {
			c.send(TypeCommand, CommandPayload{Intent: "confirm", ConfirmationID: staged.ConfirmationID, Text: ptr(blank)})
			assert.Equal(t, CodeInvalidMessage, c.untilError().Code)
		}
		assert.Zero(t, h.exam.count("SubmitAnswer"))

		// The staged answer is still waiting, and can still be confirmed as heard.
		result := confirmWith(t, c, staged, nil)
		assert.True(t, result.Saved)
	})

	t.Run("text is only valid when confirming", func(t *testing.T) {
		h, c, staged := setup(t)
		for _, intent := range []string{"deny", "repeat_question", "next_question"} {
			c.send(TypeCommand, CommandPayload{Intent: intent, ConfirmationID: staged.ConfirmationID, Text: ptr("sneaky")})
			assert.Equal(t, CodeUnsupportedIntent, c.untilError().Code, intent)
		}
		assert.Zero(t, h.exam.count("SubmitAnswer"))
	})

	t.Run("text must name the staged answer it corrects", func(t *testing.T) {
		h, c, _ := setup(t)
		c.send(TypeCommand, CommandPayload{Intent: "confirm", Text: ptr("no confirmation id")})
		assert.Equal(t, CodeUnsupportedIntent, c.untilError().Code)
		assert.Zero(t, h.exam.count("SubmitAnswer"))
	})

	t.Run("a different confirmation id saves nothing", func(t *testing.T) {
		h, c, _ := setup(t)
		c.send(TypeCommand, CommandPayload{Intent: "confirm", ConfirmationID: "someone-elses", Text: ptr("different")})
		resp := decode[ResponsePayload](t, c.until(TypeResponse))
		assert.Equal(t, CodeConfirmationUnknown, resp.Code)
		assert.Zero(t, h.exam.count("SubmitAnswer"))
	})

	t.Run("text a database cannot store is refused with a reason", func(t *testing.T) {
		h, c, staged := setup(t)
		nul := "plants\x00make sugar"
		c.send(TypeCommand, CommandPayload{Intent: "confirm", ConfirmationID: staged.ConfirmationID, Text: &nul})
		e := c.untilError()
		assert.Equal(t, CodeInvalidMessage, e.Code)
		assert.Contains(t, e.Message, "cannot be saved", "the student is told why, not shown a save failure")
		assert.Zero(t, h.exam.count("SubmitAnswer"))
	})

	t.Run("overlong text is rejected", func(t *testing.T) {
		h, c, staged := setup(t)
		tooLong := strings.Repeat("x", h.svc.Config.Intent.MaxTranscriptChars+1)
		c.send(TypeCommand, CommandPayload{Intent: "confirm", ConfirmationID: staged.ConfirmationID, Text: &tooLong})
		assert.Equal(t, CodeInvalidMessage, c.untilError().Code)
		assert.Zero(t, h.exam.count("SubmitAnswer"))
	})
}

func TestCorrection_ACardOptionHasNothingToCorrect(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect() // question 1 is multiple choice
	c.send(TypeCommand, CommandPayload{Intent: "select_option", OptionKey: "B"})
	staged := decode[ConfirmRequestPayload](t, c.until(TypeConfirmRequest))
	c.until(TypeResponse)

	c.send(TypeCommand, CommandPayload{Intent: "confirm", ConfirmationID: staged.ConfirmationID, Text: ptr("Paris, definitely")})
	resp := decode[ResponsePayload](t, c.until(TypeResponse))
	assert.Equal(t, CodeUnsupportedIntent, resp.Code)
	assert.Zero(t, h.exam.count("SubmitAnswer"), "nothing is saved when the correction does not apply")

	// The option is still staged and confirms normally.
	result := confirmWith(t, c, staged, nil)
	assert.True(t, result.Saved)
	assert.False(t, result.Corrected)
}
