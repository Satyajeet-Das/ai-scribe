package voice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
	"github.com/Satyajeet-Das/ai-scribe/internal/voice/intent"
)

// command sends a typed intent and returns the response that answers it.
func command(t *testing.T, c *client, p CommandPayload) (string, ResponsePayload) {
	t.Helper()
	id := c.send(TypeCommand, p)
	for {
		env := c.until(TypeResponse)
		if env.ReplyTo == id {
			return id, decode[ResponsePayload](t, env)
		}
	}
}

// lastOf returns the payload of the most recent message of a type.
func lastOf[T any](t *testing.T, c *client, typ string) (T, bool) {
	t.Helper()
	for i := len(c.seen) - 1; i >= 0; i-- {
		if f := c.seen[i]; f.env != nil && f.env.Type == typ {
			return decode[T](t, *f.env), true
		}
	}
	var zero T
	return zero, false
}

func TestAnswer_SpokenSelectionRequiresConfirmation(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()
	qid := h.exam.currentQuestionID()

	h.say(c, "option B")
	confirm := decode[ConfirmRequestPayload](t, c.until(TypeConfirmRequest))
	resp := decode[ResponsePayload](t, c.until(TypeResponse))

	assert.Equal(t, "select_option", confirm.Action)
	assert.Equal(t, "B", confirm.OptionKey)
	assert.Equal(t, "Paris", confirm.OptionText)
	assert.Equal(t, qid, confirm.QuestionID)
	assert.Equal(t, kindConfirmation, resp.Kind)
	assert.Contains(t, resp.Text, "option B: Paris")

	// Interpretation alone persists nothing.
	_, saved := h.exam.saved(qid)
	assert.False(t, saved)
	assert.Zero(t, h.exam.count("SubmitAnswer"))

	h.say(c, "yes")
	result := decode[ActionResultPayload](t, c.until(TypeActionResult))
	resp = decode[ResponsePayload](t, c.until(TypeResponse))

	assert.Equal(t, StatusOK, result.Status)
	assert.True(t, result.Saved)
	assert.Equal(t, confirm.ConfirmationID, result.ConfirmationID)
	require.NotNil(t, result.AnswerID)
	assert.Equal(t, kindSaved, resp.Kind)

	ans, saved := h.exam.saved(qid)
	require.True(t, saved)
	require.NotNil(t, ans.SelectedOptionID)
	assert.Equal(t, h.exam.questions[0].Options[1].ID, *ans.SelectedOptionID)
	assert.Equal(t, *result.AnswerID, ans.ID)
	assert.True(t, h.cls.lastInput().AwaitingConfirmation, "the classifier is told a confirmation is pending")
}

func TestAnswer_DenyDiscardsSelection(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()

	command(t, c, CommandPayload{Intent: "select_option", OptionKey: "a"})
	_, resp := command(t, c, CommandPayload{Intent: "deny"})

	result, _ := lastOf[ActionResultPayload](t, c, TypeActionResult)
	assert.Equal(t, StatusCancelled, result.Status)
	assert.False(t, result.Saved)
	assert.Equal(t, msgCancelled, resp.Text)
	assert.Zero(t, h.exam.count("SubmitAnswer"))

	// The discarded selection cannot be confirmed afterwards.
	_, resp = command(t, c, CommandPayload{Intent: "confirm"})
	assert.Equal(t, CodeNothingToConfirm, resp.Code)
	assert.Zero(t, h.exam.count("SubmitAnswer"))
}

func TestAnswer_AmbiguousOrInvalidInterpretationIsNeverSaved(t *testing.T) {
	cases := []struct {
		name string
		out  func(ctx context.Context, in intent.Input) (intent.Intent, error)
		code string
	}{
		{"low confidence selection", func(context.Context, intent.Input) (intent.Intent, error) {
			return intent.Intent{Name: intent.SelectOption, OptionKey: "B", Confidence: 0.4}, nil
		}, CodeNotUnderstood},
		{"option that does not exist", func(context.Context, intent.Input) (intent.Intent, error) {
			return intent.Intent{Name: intent.SelectOption, OptionKey: "Z", Confidence: 0.99}, nil
		}, CodeNotUnderstood},
		{"selection without an option", func(context.Context, intent.Input) (intent.Intent, error) {
			return intent.Intent{Name: intent.SelectOption, Confidence: 0.99}, nil
		}, CodeNotUnderstood},
		{"intent outside the permitted set", func(context.Context, intent.Input) (intent.Intent, error) {
			return intent.Intent{Name: "submit_exam", Confidence: 1}, nil
		}, CodeNotUnderstood},
		{"confidence out of range", func(context.Context, intent.Input) (intent.Intent, error) {
			return intent.Intent{Name: intent.NextQuestion, Confidence: 7}, nil
		}, CodeNotUnderstood},
		{"explicitly unknown", func(context.Context, intent.Input) (intent.Intent, error) {
			return intent.Intent{Name: intent.Unknown, Confidence: 1}, nil
		}, CodeNotUnderstood},
		{"classifier outage", func(context.Context, intent.Input) (intent.Intent, error) {
			return intent.Intent{}, errors.New("upstream 503")
		}, CodeIntentUnavailable},
		{"classifier timeout", func(ctx context.Context, _ intent.Input) (intent.Intent, error) {
			<-ctx.Done()
			return intent.Intent{}, ctx.Err()
		}, CodeIntentUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, withoutTTS(), withConfig(func(cfg *config.VoiceConfig) {
				cfg.Intent.Timeout = 100 * time.Millisecond
			}))
			h.cls.set(tc.out)
			c, _ := h.connect()
			from := len(c.seen)

			h.say(c, "um the second one I think")
			resp := decode[ResponsePayload](t, c.until(TypeResponse))

			assert.Equal(t, tc.code, resp.Code)
			assert.NotContains(t, c.types(from), TypeConfirmRequest)
			assert.NotContains(t, c.types(from), TypeActionResult)
			for _, m := range []string{"SubmitAnswer", "NextQuestion", "PreviousQuestion"} {
				assert.Zero(t, h.exam.count(m), "%s must not be called", m)
			}

			// Even a "yes" now saves nothing: no selection was ever staged.
			h.cls.set(nil)
			h.say(c, "yes")
			resp = decode[ResponsePayload](t, c.until(TypeResponse))
			assert.Equal(t, CodeNothingToConfirm, resp.Code)
			assert.Zero(t, h.exam.count("SubmitAnswer"))
		})
	}
}

func TestAnswer_ClassifierSeesNoAnswerKey(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()

	h.say(c, "option B")
	c.until(TypeResponse)

	in := h.cls.lastInput()
	assert.Equal(t, "option B", in.Transcript)
	assert.Equal(t, "MCQ", in.QuestionType)
	assert.Equal(t, []intent.Option{{Key: "A", Text: "London"}, {Key: "B", Text: "Paris"}, {Key: "C", Text: "Rome"}}, in.Options)
}

func TestAnswer_ConfirmationExpires(t *testing.T) {
	h := newHarness(t, withoutTTS(), withConfig(func(cfg *config.VoiceConfig) {
		cfg.ConfirmationTTL = 50 * time.Millisecond
	}))
	c, _ := h.connect()

	command(t, c, CommandPayload{Intent: "select_option", OptionKey: "B"})
	time.Sleep(120 * time.Millisecond)
	_, resp := command(t, c, CommandPayload{Intent: "confirm"})

	assert.Equal(t, CodeConfirmationExpired, resp.Code)
	assert.Zero(t, h.exam.count("SubmitAnswer"))
}

func TestAnswer_ConfirmationIsBoundToItsQuestion(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()
	first := h.exam.currentQuestionID()

	command(t, c, CommandPayload{Intent: "select_option", OptionKey: "B"})
	// The exam moves on behind the connection's back (e.g. via the REST API).
	_, err := h.exam.NextQuestion(context.Background(), h.sessionID, h.student)
	require.NoError(t, err)

	_, resp := command(t, c, CommandPayload{Intent: "confirm"})
	assert.Equal(t, CodeConfirmationExpired, resp.Code)
	_, saved := h.exam.saved(first)
	assert.False(t, saved)
	assert.Zero(t, h.exam.count("SubmitAnswer"))
}

func TestAnswer_NavigationDiscardsUnconfirmedSelection(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()

	command(t, c, CommandPayload{Intent: "select_option", OptionKey: "B"})
	_, resp := command(t, c, CommandPayload{Intent: "next_question"})

	assert.Contains(t, resp.Text, msgDiscarded)
	assert.Contains(t, resp.Text, "red planet")
	assert.Zero(t, h.exam.count("SubmitAnswer"))

	_, resp = command(t, c, CommandPayload{Intent: "confirm"})
	assert.Equal(t, CodeNothingToConfirm, resp.Code)
}

func TestAnswer_DuplicateConfirmWritesOnce(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()

	command(t, c, CommandPayload{Intent: "select_option", OptionKey: "B"})
	confirm, _ := lastOf[ConfirmRequestPayload](t, c, TypeConfirmRequest)

	command(t, c, CommandPayload{Intent: "confirm", ConfirmationID: confirm.ConfirmationID})
	first, _ := lastOf[ActionResultPayload](t, c, TypeActionResult)
	require.True(t, first.Saved)

	// The same confirmation arrives again under a new request ID (a retry
	// after a lost acknowledgement, or a delayed duplicate).
	command(t, c, CommandPayload{Intent: "confirm", ConfirmationID: confirm.ConfirmationID})
	second, _ := lastOf[ActionResultPayload](t, c, TypeActionResult)

	assert.True(t, second.Duplicate)
	assert.True(t, second.Saved)
	assert.Equal(t, first.AnswerID, second.AnswerID)
	assert.Equal(t, 1, h.exam.count("SubmitAnswer"))
}

func TestAnswer_ConfirmationIDMustMatch(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()

	command(t, c, CommandPayload{Intent: "select_option", OptionKey: "B"})
	_, resp := command(t, c, CommandPayload{Intent: "confirm", ConfirmationID: "some-older-confirmation"})

	assert.Equal(t, CodeConfirmationUnknown, resp.Code)
	assert.Zero(t, h.exam.count("SubmitAnswer"))
}

func TestAnswer_PersistenceFailureIsNeverReportedAsSaved(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()
	qid := h.exam.currentQuestionID()

	command(t, c, CommandPayload{Intent: "select_option", OptionKey: "B"})
	h.exam.fail("SubmitAnswer", errors.New("pq: connection refused"))
	_, resp := command(t, c, CommandPayload{Intent: "confirm"})

	result, _ := lastOf[ActionResultPayload](t, c, TypeActionResult)
	assert.Equal(t, StatusFailed, result.Status)
	assert.False(t, result.Saved)
	assert.True(t, result.Retryable)
	assert.Equal(t, CodeUnavailable, result.Code)
	assert.Nil(t, result.AnswerID)
	assert.Equal(t, kindError, resp.Kind)
	assert.NotContains(t, resp.Text, "connection refused", "internal errors are not shown to the student")
	_, saved := h.exam.saved(qid)
	assert.False(t, saved)
	// One attempt only: the write was not retried behind the student's back.
	assert.Equal(t, 1, h.exam.count("SubmitAnswer"))

	// When storage recovers, an explicit second "yes" saves it.
	h.exam.fail("SubmitAnswer", nil)
	command(t, c, CommandPayload{Intent: "confirm"})
	result, _ = lastOf[ActionResultPayload](t, c, TypeActionResult)
	assert.True(t, result.Saved)
	_, saved = h.exam.saved(qid)
	assert.True(t, saved)
}

func TestAnswer_SaveCompletesEvenIfStudentInterrupts(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()
	qid := h.exam.currentQuestionID()

	command(t, c, CommandPayload{Intent: "select_option", OptionKey: "C"})

	gate := make(chan struct{})
	h.exam.mu.Lock()
	h.exam.submitGate = gate
	h.exam.mu.Unlock()

	confirmID := c.send(TypeCommand, CommandPayload{Intent: "confirm"})
	// While the save is in flight, the student asks for something else.
	time.Sleep(50 * time.Millisecond)
	repeatID := c.send(TypeCommand, CommandPayload{Intent: "repeat_question"})
	time.Sleep(50 * time.Millisecond)
	close(gate)

	var result ActionResultPayload
	for {
		env := c.until(TypeActionResult)
		if env.ReplyTo == confirmID {
			result = decode[ActionResultPayload](t, env)
			break
		}
	}
	assert.True(t, result.Saved, "a save that had begun is carried through and reported")
	_, saved := h.exam.saved(qid)
	assert.True(t, saved)

	for c.until(TypeResponse).ReplyTo != repeatID {
	}
}

func TestDictation_UsesStudentWordsVerbatim(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()
	command(t, c, CommandPayload{Intent: "next_question"})
	command(t, c, CommandPayload{Intent: "next_question"})
	qid := h.exam.currentQuestionID()

	// The model labels the utterance; whatever else it might return is ignored.
	h.cls.set(func(context.Context, intent.Input) (intent.Intent, error) {
		return intent.Intent{Name: intent.DictateAnswer, Confidence: 0.95}, nil
	})
	h.say(c, "Plants turn light into chemical energy")
	confirm := decode[ConfirmRequestPayload](t, c.until(TypeConfirmRequest))
	assert.Equal(t, "dictate_answer", confirm.Action)
	assert.Equal(t, "Plants turn light into chemical energy", confirm.Text)
	c.until(TypeResponse)
	_, saved := h.exam.saved(qid)
	assert.False(t, saved)

	command(t, c, CommandPayload{Intent: "confirm"})
	ans, saved := h.exam.saved(qid)
	require.True(t, saved)
	assert.Equal(t, "Plants turn light into chemical energy", ans.TextAnswer)
	assert.Nil(t, ans.SelectedOptionID)

	_, resp := command(t, c, CommandPayload{Intent: "read_back_answer"})
	assert.Contains(t, resp.Text, "Plants turn light into chemical energy")
}

func TestDictation_NotAcceptedForMultipleChoice(t *testing.T) {
	h := newHarness(t, withoutTTS())
	h.cls.set(func(context.Context, intent.Input) (intent.Intent, error) {
		return intent.Intent{Name: intent.DictateAnswer, Confidence: 0.95}, nil
	})
	c, _ := h.connect()

	h.say(c, "I think it is probably Paris")
	resp := decode[ResponsePayload](t, c.until(TypeResponse))
	assert.Equal(t, CodeUnsupportedIntent, resp.Code)
	assert.Zero(t, h.exam.count("SubmitAnswer"))
}

func TestNavigation_GoesThroughSessionService(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()

	t.Run("previous at the first question is rejected by the service", func(t *testing.T) {
		_, resp := command(t, c, CommandPayload{Intent: "previous_question"})
		result, _ := lastOf[ActionResultPayload](t, c, TypeActionResult)
		assert.Equal(t, StatusRejected, result.Status)
		assert.Equal(t, CodeNoPreviousQuestion, result.Code)
		assert.Equal(t, msgFirstQuestion, resp.Text)
	})
	t.Run("next reads the new question and reports state", func(t *testing.T) {
		h.say(c, "next question")
		result := decode[ActionResultPayload](t, c.until(TypeActionResult))
		state := decode[SessionState](t, c.until(TypeSessionState))
		resp := decode[ResponsePayload](t, c.until(TypeResponse))
		assert.Equal(t, StatusOK, result.Status)
		assert.Equal(t, "next", result.Direction)
		assert.Equal(t, 2, state.Question.CurrentIndex)
		assert.Contains(t, resp.Text, "Question 2 of 3")
	})
	t.Run("next at the last question is rejected by the service", func(t *testing.T) {
		command(t, c, CommandPayload{Intent: "next_question"})
		_, resp := command(t, c, CommandPayload{Intent: "next_question"})
		result, _ := lastOf[ActionResultPayload](t, c, TypeActionResult)
		assert.Equal(t, StatusRejected, result.Status)
		assert.Equal(t, CodeNoNextQuestion, result.Code)
		assert.Equal(t, msgLastQuestion, resp.Text)
	})
	t.Run("redis or lock outage changes nothing and is retryable", func(t *testing.T) {
		h.exam.fail("PreviousQuestion", session.ErrSessionLocked)
		defer h.exam.fail("PreviousQuestion", nil)
		before := h.exam.currentQuestionID()
		_, resp := command(t, c, CommandPayload{Intent: "previous_question"})
		result, _ := lastOf[ActionResultPayload](t, c, TypeActionResult)
		assert.Equal(t, StatusFailed, result.Status)
		assert.Equal(t, CodeBusy, result.Code)
		assert.True(t, result.Retryable)
		assert.Equal(t, msgBusy, resp.Text)
		assert.Equal(t, before, h.exam.currentQuestionID())
	})
}

func TestReadBackAndTimeRemaining(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()

	_, resp := command(t, c, CommandPayload{Intent: "read_back_answer"})
	assert.Equal(t, CodeNoAnswer, resp.Code)

	command(t, c, CommandPayload{Intent: "select_option", OptionKey: "B"})
	command(t, c, CommandPayload{Intent: "confirm"})
	_, resp = command(t, c, CommandPayload{Intent: "read_back_answer"})
	assert.Equal(t, "Your saved answer is option B: Paris.", resp.Text)

	_, resp = command(t, c, CommandPayload{Intent: "time_remaining"})
	assert.Contains(t, resp.Text, "59 minutes")
}

func TestCommand_InvalidOptionIsRejected(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()

	_, resp := command(t, c, CommandPayload{Intent: "select_option", OptionKey: "Q"})
	assert.Equal(t, CodeInvalidOption, resp.Code)
	_, ok := lastOf[ConfirmRequestPayload](t, c, TypeConfirmRequest)
	assert.False(t, ok)
}

func TestSpokenDuration(t *testing.T) {
	assert.Equal(t, "less than a minute", spokenDuration(42))
	assert.Equal(t, "1 minute", spokenDuration(61))
	assert.Equal(t, "1 hour and 5 minutes", spokenDuration(3900))
	assert.Equal(t, "2 hours", spokenDuration(7200))
}
