package voice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/answer"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/ws"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
	"github.com/Satyajeet-Das/ai-scribe/internal/voice/intent"
)

// Reply kinds, carried in "response" messages.
const (
	kindQuestion     = "question"
	kindConfirmation = "confirmation_request"
	kindSaved        = "saved"
	kindInfo         = "info"
	kindClarify      = "clarification"
	kindError        = "error"
)

const (
	msgNoSpeech          = "I didn't hear anything. Please try again."
	msgTooLong           = "That was too long for me to process. Please say it in a shorter form."
	msgIntentUnavailable = "Sorry, I couldn't process that just now. Nothing was changed. Please try again."
	msgInvalidOption     = "That is not one of the options for this question."
	msgNotUnderstood     = "Sorry, I didn't understand. You can say repeat, next, previous, read my answer, or how much time is left. To give a written answer by voice, press Dictate answer, or say my answer is, followed by your answer."
	msgNotUnderstoodMCQ  = "Sorry, I didn't understand. You can say an option, such as option A, or say repeat, next, previous, or read my answer."
	msgConfirmPrompt     = "Please say yes to save your answer, or no to cancel."
	msgNothingToConfirm  = "There is nothing waiting to be confirmed."
	msgConfirmExpired    = "That confirmation has expired, so your answer was not saved. Please give your answer again."
	msgCancelled         = "Cancelled. Nothing was saved."
	msgDiscarded         = "Your unconfirmed answer was not saved. "
	msgNotMCQ            = "This question does not have options. Say my answer is, followed by your answer."
	msgUseOption         = "This is a multiple choice question. Please say the option you want, such as option A."
	msgNoAnswer          = "You have not answered this question yet."
	msgLastQuestion      = "You are already on the last question."
	msgFirstQuestion     = "You are already on the first question."
	msgSaveUnconfirmed   = "I could not confirm that your answer was saved. Say read my answer to check, or say yes to try again."
	msgSaveFailed        = "Your answer could not be saved."
	msgBusy              = "The exam system is busy. Nothing was changed. Please try again."
	msgNoCorrection      = "Only a dictated answer can be corrected. Nothing was saved."
)

// reply is what the assistant says in response to a turn.
type reply struct {
	kind string
	code string
	text string
}

// pendingConfirmation is an answer the student has proposed but not yet
// confirmed. It lives only in connection memory: until it is confirmed and
// the answer service has persisted it, nothing about it is saved anywhere.
type pendingConfirmation struct {
	id          string
	questionID  uuid.UUID
	questionNum int
	optionID    *uuid.UUID
	optionKey   string
	text        string
	expiresAt   time.Time
	// transcriptionID and recordingIDs are set when the dictated audio was kept.
	// They are linked to the answer's revision only when it is confirmed.
	transcriptionID *uuid.UUID
	recordingIDs    []uuid.UUID
}

// failure is how an error from the exam services is presented to the client.
type failure struct {
	code        string
	message     string
	retryable   bool
	terminal    bool
	unexpected  bool
	closeReason string
	closeCode   ws.StatusCode
}

// classifyError maps errors from the session and answer services onto the
// protocol. Messages are fixed strings: internal error text never reaches the
// client.
func classifyError(err error) failure {
	ended := func(code, message, reason string) failure {
		return failure{code: code, message: message, terminal: true, closeReason: reason, closeCode: CloseSessionEnded}
	}
	switch {
	case errors.Is(err, session.ErrSessionExpired):
		return ended(CodeSessionExpired, "the exam time has expired", "expired")
	case errors.Is(err, session.ErrSessionAlreadySubmitted):
		return ended(CodeSessionSubmitted, "the exam has already been submitted", "submitted")
	case errors.Is(err, session.ErrSessionNotInProgress),
		errors.Is(err, session.ErrInvalidTransition),
		errors.Is(err, answer.ErrSessionNotActive):
		return ended(CodeSessionInactive, "the exam session is not in progress", "inactive")
	case errors.Is(err, session.ErrSessionNotFound):
		return ended(CodeSessionInactive, "the exam session no longer exists", "not_found")
	case errors.Is(err, session.ErrUnauthorizedStudent), errors.Is(err, answer.ErrUnauthorizedStudent):
		return failure{code: CodeForbidden, message: "you do not have access to this exam session", terminal: true, closeReason: "forbidden", closeCode: CloseForbidden}
	case errors.Is(err, session.ErrExamNotPublished):
		return failure{code: CodeExamUnavailable, message: "the exam is currently unavailable"}
	case errors.Is(err, session.ErrNoNextQuestion):
		return failure{code: CodeNoNextQuestion, message: "already at the last question"}
	case errors.Is(err, session.ErrNoPreviousQuestion):
		return failure{code: CodeNoPreviousQuestion, message: "already at the first question"}
	case errors.Is(err, session.ErrSessionLocked):
		return failure{code: CodeBusy, message: "the session is busy, please retry", retryable: true}
	case errors.Is(err, answer.ErrOptionNotForQuestion),
		errors.Is(err, answer.ErrQuestionNotForExam),
		errors.Is(err, answer.ErrInvalidAnswer),
		errors.Is(err, question.ErrQuestionNotFound),
		errors.Is(err, question.ErrOptionNotFound),
		errors.Is(err, session.ErrNoQuestionsInExam):
		return failure{code: CodeInvalidOption, message: "that answer is not valid for the current question"}
	default:
		// PostgreSQL or Redis trouble, or a timeout. The outcome of a write
		// may be unknown, so it is reported as retryable and never as saved.
		return failure{code: CodeUnavailable, message: "the exam service is temporarily unavailable", retryable: true, unexpected: true}
	}
}

// actionContext returns a context for a state-changing call. It is detached
// from the turn's cancellation on purpose: once the backend has decided to
// perform a mutation, a barge-in or a dropped socket must not abort it halfway.
// It is still bounded by ActionTimeout.
func (c *conversation) actionContext(t *turn) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(t.ctx), c.svc.Config.ActionTimeout)
}

// act is where the backend decides. It takes a validated intent, checks it
// against the current dialog and exam state, and performs at most one action
// through the session or answer service. dialogMu makes this atomic per
// connection.
func (c *conversation) act(t *turn, it intent.Intent, transcript string, q *session.StudentQuestionResponse, confirmationID string) reply {
	c.dialogMu.Lock()
	defer c.dialogMu.Unlock()

	// A turn that was interrupted before reaching this point is stale.
	if t.ctx.Err() != nil {
		return reply{}
	}

	now := c.svc.now()
	p := c.pending
	expired := false
	if p != nil && (now.After(p.expiresAt) || p.questionID != q.ID) {
		// The confirmation timed out, or the exam moved to another question
		// (possibly through the REST API). Either way it no longer applies.
		c.pending, p, expired = nil, nil, true
	}

	if it.Name == intent.Unknown || it.Confidence < c.svc.Config.Intent.MinConfidence {
		if p != nil {
			return reply{kind: kindClarify, code: CodeNotUnderstood, text: msgConfirmPrompt}
		}
		if len(q.Options) > 0 {
			return reply{kind: kindClarify, code: CodeNotUnderstood, text: msgNotUnderstoodMCQ}
		}
		return reply{kind: kindClarify, code: CodeNotUnderstood, text: msgNotUnderstood}
	}

	switch it.Name {
	case intent.Confirm:
		if confirmationID != "" {
			if prior, ok := c.completed[confirmationID]; ok {
				// A repeated confirm for an answer that is already saved:
				// report the original result, do not write again.
				prior.Duplicate = true
				c.send(TypeActionResult, t.requestID, prior)
				t.responded.Store(true)
				return reply{kind: kindSaved, text: "Your answer is already saved."}
			}
		}
		if p == nil {
			if expired {
				return reply{kind: kindInfo, code: CodeConfirmationExpired, text: msgConfirmExpired}
			}
			return reply{kind: kindInfo, code: CodeNothingToConfirm, text: msgNothingToConfirm}
		}
		if confirmationID != "" && confirmationID != p.id {
			return reply{kind: kindInfo, code: CodeConfirmationUnknown, text: msgConfirmPrompt}
		}
		if t.correction != nil && p.optionID != nil {
			// A chosen option has no text to correct. Keep it staged.
			return reply{kind: kindClarify, code: CodeUnsupportedIntent, text: msgNoCorrection}
		}
		return c.commit(t, p)

	case intent.Deny:
		if p == nil {
			return reply{kind: kindInfo, code: CodeNothingToConfirm, text: msgNothingToConfirm}
		}
		if confirmationID != "" && confirmationID != p.id {
			return reply{kind: kindInfo, code: CodeConfirmationUnknown, text: msgConfirmPrompt}
		}
		c.discard(t, p)
		return reply{kind: kindInfo, text: msgCancelled}

	case intent.SelectOption:
		return c.proposeOption(t, it, q)

	case intent.DictateAnswer:
		return c.proposeText(t, transcript, q)
	}

	// Any other request abandons an unconfirmed answer, and says so.
	prefix := ""
	if p != nil {
		c.discard(t, p)
		prefix = msgDiscarded
	}

	var r reply
	switch it.Name {
	case intent.RepeatQuestion:
		r = reply{kind: kindQuestion, text: readQuestion(q)}
	case intent.NextQuestion:
		r = c.navigate(t, true)
	case intent.PreviousQuestion:
		r = c.navigate(t, false)
	case intent.ReadBackAnswer:
		r = c.readBack(t, q)
	case intent.TimeRemaining:
		r = c.timeRemaining(t)
	default:
		r = reply{kind: kindClarify, code: CodeUnsupportedIntent, text: msgNotUnderstood}
	}
	if r.text != "" {
		r.text = prefix + r.text
	}
	return r
}

func (c *conversation) discard(t *turn, p *pendingConfirmation) {
	c.pending = nil
	c.svc.Metrics.Inc(MetricActions, ActionAnswer, StatusCancelled)
	c.send(TypeActionResult, t.requestID, ActionResultPayload{
		Action: ActionAnswer, Status: StatusCancelled, Saved: false,
		ConfirmationID: p.id, QuestionID: &p.questionID, OptionKey: p.optionKey,
	})
}

func (c *conversation) propose(t *turn, p *pendingConfirmation, action, optionText string) {
	p.id = uuid.NewString()
	p.expiresAt = c.svc.now().Add(c.svc.Config.ConfirmationTTL)
	c.pending = p
	c.send(TypeConfirmRequest, t.requestID, ConfirmRequestPayload{
		ConfirmationID: p.id,
		Action:         action,
		QuestionID:     p.questionID,
		OptionKey:      p.optionKey,
		OptionText:     optionText,
		Text:           p.text,
		ExpiresAt:      p.expiresAt,
	})
}

// proposeOption never saves. Choosing an answer in an exam is consequential,
// so it always goes through an explicit spoken or typed confirmation.
func (c *conversation) proposeOption(t *turn, it intent.Intent, q *session.StudentQuestionResponse) reply {
	if len(q.Options) == 0 {
		return reply{kind: kindClarify, code: CodeUnsupportedIntent, text: msgNotMCQ}
	}
	for _, o := range q.Options {
		if o.OptionKey != it.OptionKey {
			continue
		}
		optionID := o.ID
		c.propose(t, &pendingConfirmation{
			questionID: q.ID, questionNum: q.CurrentIndex, optionID: &optionID, optionKey: o.OptionKey,
		}, string(intent.SelectOption), o.OptionText)
		return reply{
			kind: kindConfirmation,
			text: fmt.Sprintf("You chose option %s: %s. Say yes to save this answer, or no to cancel.", o.OptionKey, o.OptionText),
		}
	}
	return reply{kind: kindClarify, code: CodeInvalidOption, text: msgInvalidOption}
}

// proposeText stages the student's own words, verbatim from the recognizer,
// as a free-text answer. No model output is ever used as answer content.
func (c *conversation) proposeText(t *turn, transcript string, q *session.StudentQuestionResponse) reply {
	if len(q.Options) > 0 {
		return reply{kind: kindClarify, code: CodeUnsupportedIntent, text: msgUseOption}
	}
	text := strings.TrimSpace(transcript)
	if text == "" {
		return reply{kind: kindClarify, code: CodeUnsupportedIntent, text: msgNotMCQ}
	}
	p := &pendingConfirmation{questionID: q.ID, questionNum: q.CurrentIndex, text: text}
	if t.saved != nil {
		trID, recID := t.saved.TranscriptionID, t.saved.RecordingID
		p.transcriptionID, p.recordingIDs = &trID, []uuid.UUID{recID}
	}
	c.propose(t, p, string(intent.DictateAnswer), "")
	return reply{
		kind: kindConfirmation,
		text: fmt.Sprintf("I heard: %s. Say yes to save this as your answer, or no to cancel.", text),
	}
}

// commit persists a confirmed answer through the answer service, which takes
// the session lock, runs the FSM guard and writes to PostgreSQL. The answer is
// reported as saved only after that call returns successfully.
func (c *conversation) commit(t *turn, p *pendingConfirmation) reply {
	ctx, cancel := c.actionContext(t)
	defer cancel()

	// What is saved is what the student confirmed. If they corrected the text,
	// that is their wording; the recognizer's original stays in its transcript.
	text, source, corrected := p.text, answer.SourceSTTConfirmed, false
	if t.correction != nil && p.optionID == nil {
		if edited := strings.TrimSpace(*t.correction); edited != p.text {
			text, source, corrected = edited, answer.SourceStudentCorrection, true
		}
	}
	req := answer.SubmitAnswerRequest{SelectedOptionID: p.optionID, TextAnswer: text}
	// Say where the value came from, and tie a repeated confirm to one revision.
	req.Revision = &answer.RevisionInput{Source: source, ConfirmationID: p.id,
		TranscriptionID: p.transcriptionID, RecordingIDs: p.recordingIDs}
	if p.optionID != nil {
		req.Revision = &answer.RevisionInput{Source: answer.SourceOption, ConfirmationID: p.id}
	}
	start := c.svc.now()
	ans, err := c.svc.Answers.SubmitAnswer(ctx, c.sessionID, p.questionID, req, c.userID)
	c.svc.Metrics.Observe(MetricProviderLatency, c.svc.now().Sub(start), "answer_save")

	result := ActionResultPayload{
		Action: ActionAnswer, ConfirmationID: p.id, QuestionID: &p.questionID, OptionKey: p.optionKey,
	}
	// From here on the client is told the outcome even if the turn was
	// interrupted, because the outcome is about durable state.
	t.responded.Store(true)

	if err != nil {
		f := classifyError(err)
		result.Status, result.Code, result.Retryable = StatusFailed, f.code, f.retryable
		c.svc.Metrics.Inc(MetricActions, ActionAnswer, StatusFailed)
		c.svc.Metrics.Inc(MetricFailures, f.code)
		c.log.Error().Err(err).Str("event", "voice.answer_failed").Str("code", f.code).
			Str("question_id", p.questionID.String()).Str("confirmation_id", p.id).
			Str("request_id", t.requestID).Msg("answer was not saved")
		c.send(TypeActionResult, t.requestID, result)

		if f.terminal {
			c.pending = nil
			c.endSession(f.closeReason, f.closeCode)
			return reply{}
		}
		if !f.retryable {
			c.pending = nil
			return reply{kind: kindError, code: f.code, text: msgSaveFailed}
		}
		// The write is an idempotent upsert, so the student may explicitly
		// confirm again; it is never retried automatically.
		return reply{kind: kindError, code: f.code, text: msgSaveUnconfirmed}
	}

	c.pending = nil
	result.Status, result.Saved, result.Corrected = StatusOK, true, corrected
	result.AnswerID, result.SavedAt = &ans.ID, &ans.UpdatedAt
	c.remember(p.id, result)
	c.svc.Metrics.Inc(MetricActions, ActionAnswer, StatusOK)
	c.log.Info().Str("event", "voice.answer_saved").
		Str("question_id", p.questionID.String()).Str("answer_id", ans.ID.String()).
		Str("confirmation_id", p.id).Str("request_id", t.requestID).Msg("answer saved by voice")
	c.send(TypeActionResult, t.requestID, result)

	if p.optionKey != "" {
		return reply{kind: kindSaved, text: fmt.Sprintf("Saved. Option %s is recorded for question %d.", p.optionKey, p.questionNum)}
	}
	if corrected {
		return reply{kind: kindSaved, text: fmt.Sprintf("Saved. Your corrected answer is recorded for question %d.", p.questionNum)}
	}
	return reply{kind: kindSaved, text: fmt.Sprintf("Saved. Your answer is recorded for question %d.", p.questionNum)}
}

// remember keeps a bounded record of confirmed answers so that a duplicate
// confirm can be answered without writing twice.
func (c *conversation) remember(id string, result ActionResultPayload) {
	if len(c.completed) >= 32 {
		for k := range c.completed {
			delete(c.completed, k)
			break
		}
	}
	c.completed[id] = result
}

// navigate moves through the exam using the session service, whose FSM is the
// only authority on whether the move is allowed.
func (c *conversation) navigate(t *turn, forward bool) reply {
	ctx, cancel := c.actionContext(t)
	defer cancel()

	direction, call := "previous", c.svc.Sessions.PreviousQuestion
	if forward {
		direction, call = "next", c.svc.Sessions.NextQuestion
	}

	result := ActionResultPayload{Action: ActionNavigate, Direction: direction}
	if _, err := call(ctx, c.sessionID, c.userID); err != nil {
		f := classifyError(err)
		t.responded.Store(true)
		result.Status, result.Code, result.Retryable = StatusFailed, f.code, f.retryable
		switch f.code {
		case CodeNoNextQuestion:
			result.Status = StatusRejected
			c.send(TypeActionResult, t.requestID, result)
			return reply{kind: kindInfo, code: f.code, text: msgLastQuestion}
		case CodeNoPreviousQuestion:
			result.Status = StatusRejected
			c.send(TypeActionResult, t.requestID, result)
			return reply{kind: kindInfo, code: f.code, text: msgFirstQuestion}
		}
		c.svc.Metrics.Inc(MetricActions, ActionNavigate, StatusFailed)
		c.svc.Metrics.Inc(MetricFailures, f.code)
		if f.unexpected {
			c.log.Error().Err(err).Str("event", "voice.navigate_failed").Str("request_id", t.requestID).Msg("navigation failed")
		}
		c.send(TypeActionResult, t.requestID, result)
		if f.terminal {
			c.endSession(f.closeReason, f.closeCode)
			return reply{}
		}
		return reply{kind: kindError, code: f.code, text: msgBusy}
	}

	t.responded.Store(true)
	result.Status = StatusOK
	c.svc.Metrics.Inc(MetricActions, ActionNavigate, StatusOK)
	c.log.Info().Str("event", "voice.navigated").Str("direction", direction).Str("request_id", t.requestID).Msg("navigated by voice")
	c.send(TypeActionResult, t.requestID, result)

	state, err := c.sessionState(ctx)
	if err != nil || state.Question == nil {
		return reply{kind: kindInfo, text: "Moved to the " + direction + " question. Say repeat to hear it."}
	}
	c.send(TypeSessionState, t.requestID, state)
	return reply{kind: kindQuestion, text: readQuestion(state.Question)}
}

func (c *conversation) readBack(t *turn, q *session.StudentQuestionResponse) reply {
	ctx, cancel := context.WithTimeout(t.ctx, c.svc.Config.ActionTimeout)
	defer cancel()

	ans, err := c.svc.Answers.GetAnswer(ctx, c.sessionID, q.ID, c.userID)
	if errors.Is(err, answer.ErrAnswerNotFound) || (err == nil && ans == nil) {
		return reply{kind: kindInfo, code: CodeNoAnswer, text: msgNoAnswer}
	}
	if err != nil {
		if t.ctx.Err() != nil {
			return reply{}
		}
		f := c.domainError(t.requestID, err, t.streamLabel())
		t.responded.Store(true)
		if f.terminal {
			return reply{}
		}
		return reply{kind: kindError, code: f.code, text: msgBusy}
	}

	if ans.SelectedOptionID != nil {
		for _, o := range q.Options {
			if o.ID == *ans.SelectedOptionID {
				return reply{kind: kindInfo, text: fmt.Sprintf("Your saved answer is option %s: %s.", o.OptionKey, o.OptionText)}
			}
		}
	}
	if strings.TrimSpace(ans.TextAnswer) != "" {
		return reply{kind: kindInfo, text: "Your saved answer is: " + ans.TextAnswer}
	}
	return reply{kind: kindInfo, code: CodeNoAnswer, text: msgNoAnswer}
}

func (c *conversation) timeRemaining(t *turn) reply {
	ctx, cancel := context.WithTimeout(t.ctx, c.svc.Config.ActionTimeout)
	defer cancel()

	sess, durationMins, err := c.svc.Sessions.GetSession(ctx, c.sessionID, c.userID)
	if err != nil || sess == nil {
		if t.ctx.Err() != nil {
			return reply{}
		}
		if err == nil {
			err = session.ErrSessionNotFound
		}
		f := c.domainError(t.requestID, err, t.streamLabel())
		t.responded.Store(true)
		if f.terminal {
			return reply{}
		}
		return reply{kind: kindError, code: f.code, text: msgBusy}
	}
	remaining := session.ToSessionResponse(sess, durationMins).RemainingSeconds
	if remaining == nil {
		return reply{kind: kindInfo, text: "This exam has no time limit."}
	}
	return reply{kind: kindInfo, text: "You have " + spokenDuration(*remaining) + " left."}
}

func spokenDuration(seconds int64) string {
	if seconds < 60 {
		return "less than a minute"
	}
	hours, minutes := seconds/3600, (seconds%3600)/60
	plural := func(n int64, unit string) string {
		if n == 1 {
			return fmt.Sprintf("%d %s", n, unit)
		}
		return fmt.Sprintf("%d %ss", n, unit)
	}
	switch {
	case hours > 0 && minutes > 0:
		return plural(hours, "hour") + " and " + plural(minutes, "minute")
	case hours > 0:
		return plural(hours, "hour")
	default:
		return plural(minutes, "minute")
	}
}
