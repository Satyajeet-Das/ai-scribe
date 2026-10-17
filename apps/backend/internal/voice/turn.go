package voice

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/ws"
	"github.com/Satyajeet-Das/ai-scribe/internal/recording"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
	"github.com/Satyajeet-Das/ai-scribe/internal/voice/intent"
)

var errSTTTimeout = errors.New("timed out waiting for the final transcript")

// runAudioTurn is the pipeline for one utterance:
// audio → transcript → intent → validated action → spoken response.
func (c *conversation) runAudioTurn(t *turn, format speech.AudioFormat) {
	defer c.finishTurn(t)

	transcript, ok := c.transcribe(t, format)
	if !ok {
		return
	}
	c.send(TypeSTTFinal, t.requestID, TranscriptPayload{StreamID: t.streamLabel(), Text: transcript})

	if transcript == "" {
		c.respond(t, reply{kind: kindClarify, code: CodeNoSpeech, text: msgNoSpeech})
		return
	}
	// The limit is in characters, not bytes: a byte count would cut speech in
	// Hindi or Arabic to a third of what the same limit allows in English.
	if utf8.RuneCountInString(transcript) > c.svc.Config.Intent.MaxTranscriptChars {
		c.respond(t, reply{kind: kindClarify, code: CodeNotUnderstood, text: msgTooLong})
		return
	}

	q, ok := c.currentQuestion(t)
	if !ok {
		return
	}

	// Answer mode: the student chose, with a control, to dictate. The words are
	// content, not a request, so no classifier ever sees them and a phrase such
	// as "read the question" is just text. act() still stages the transcript for
	// explicit confirmation; nothing is saved here.
	if t.mode == AudioModeAnswer {
		c.keepRecording(t, format, transcript, q)
		it := intent.Intent{Name: intent.DictateAnswer, Confidence: 1}
		c.svc.Metrics.Inc(MetricIntents, string(it.Name))
		c.log.Info().Str("event", "voice.answer_utterance").Str("request_id", t.requestID).
			Str("stream_id", t.streamLabel()).Int("transcript_chars", len(transcript)).Msg("answer-mode utterance")
		c.send(TypeIntent, t.requestID, IntentPayload{
			StreamID: t.streamLabel(), Intent: string(it.Name), Confidence: 1, Classifier: "mode:answer",
		})
		c.respond(t, c.act(t, it, transcript, q, ""))
		return
	}

	in := c.classifierInput(transcript, q)

	ctx, cancel := context.WithTimeout(t.ctx, c.svc.Config.Intent.Timeout)
	start := c.svc.now()
	it, err := c.svc.Classifier.Classify(ctx, in)
	cancel()
	c.svc.Metrics.Observe(MetricProviderLatency, c.svc.now().Sub(start), "intent")

	if t.ctx.Err() != nil {
		return
	}
	if err == nil {
		// Whatever produced it, classifier output is untrusted until validated.
		it, err = intent.Validate(it, in)
	}
	switch {
	case errors.Is(err, intent.ErrInvalidIntent):
		// Output that fails the schema is treated as "not understood": the
		// student is asked again and nothing is assumed.
		c.svc.Metrics.Inc(MetricProviderErrors, "intent", "invalid_output")
		c.log.Warn().Str("event", "voice.intent_invalid").Str("request_id", t.requestID).Msg("classifier output failed validation")
		it = intent.Intent{Name: intent.Unknown}
	case err != nil:
		c.svc.Metrics.Inc(MetricProviderErrors, "intent", "error")
		c.svc.Metrics.Inc(MetricFailures, CodeIntentUnavailable)
		c.log.Error().Err(err).Str("event", "voice.intent_failed").Str("request_id", t.requestID).Msg("intent classification failed")
		// Recovery path: say so and change nothing. The intent is never guessed.
		c.respond(t, reply{kind: kindError, code: CodeIntentUnavailable, text: msgIntentUnavailable})
		return
	}

	// In command mode a model may not decide that speech is answer content: only
	// an explicit dictation phrase can, and the answer is what follows it. This is
	// not a provider failure, just a request the student can make clearer.
	if it.Name == intent.DictateAnswer {
		if body, ok := intent.DictationBody(transcript); ok {
			transcript = body
		} else {
			c.svc.Metrics.Inc(MetricIntents, "dictation_not_explicit")
			c.log.Info().Str("event", "voice.dictation_not_explicit").Str("request_id", t.requestID).
				Msg("speech was not an explicit dictation; the student is asked again")
			it = intent.Intent{Name: intent.Unknown}
		}
	}

	c.svc.Metrics.Inc(MetricIntents, string(it.Name))
	c.log.Info().Str("event", "voice.intent").Str("intent", string(it.Name)).
		Float64("confidence", it.Confidence).Str("classifier", c.svc.Classifier.Name()).
		Str("request_id", t.requestID).Str("stream_id", t.streamLabel()).
		Int("transcript_chars", len(transcript)).Msg("intent classified")
	c.send(TypeIntent, t.requestID, IntentPayload{
		StreamID: t.streamLabel(), Intent: string(it.Name), OptionKey: it.OptionKey,
		Confidence: it.Confidence, Classifier: c.svc.Classifier.Name(),
	})

	c.respond(t, c.act(t, it, transcript, q, ""))
}

// keepRecording retains the audio of a dictated answer together with its
// transcript. It never decides anything about the answer: whatever happens
// here, the transcript is staged for confirmation exactly as it would be
// without recording, so a storage problem can cost the student the recording
// but never the answer. The outcome is reported separately.
func (c *conversation) keepRecording(t *turn, format speech.AudioFormat, transcript string, q *session.StudentQuestionResponse) {
	// Audio of an utterance that was interrupted, or that cannot become an
	// answer (a multiple-choice question), is not kept.
	pcm := t.takePCM() // taken first, so it is released even when nothing is kept
	if !t.record || len(pcm) == 0 || len(q.Options) > 0 || t.ctx.Err() != nil {
		return
	}
	m := c.svc.Metrics
	status := RecordingStatusPayload{StreamID: t.streamLabel()}

	start := c.svc.now()
	saved, err := c.svc.Recorder.Save(t.ctx, recording.SaveInput{
		SessionID: c.sessionID, QuestionID: q.ID, StudentID: c.userID, StreamID: t.streamID,
		SampleRate: format.SampleRate, ChunkCount: int(t.nextSeq), PCM: pcm,
		Transcript: transcript, Provider: c.svc.STT.Name(), Language: c.svc.Config.STT.Language,
	})
	m.Observe(MetricProviderLatency, c.svc.now().Sub(start), "recording_save")

	if err != nil {
		if t.ctx.Err() != nil {
			return
		}
		code := "recording_failed"
		m.Inc(MetricFailures, code)
		c.log.Error().Err(err).Str("event", "voice.recording_failed").Str("request_id", t.requestID).
			Str("stream_id", t.streamLabel()).Msg("answer audio was not kept")
		status.Status, status.Code = RecordingFailed, code
		c.send(TypeRecordingStatus, t.requestID, status)
		return
	}

	t.saved = saved
	c.log.Info().Str("event", "voice.recording_saved").Str("request_id", t.requestID).
		Str("stream_id", t.streamLabel()).Str("recording_id", saved.RecordingID.String()).
		Int("duration_ms", saved.DurationMillis).Msg("answer audio kept")
	status.Status, status.RecordingID, status.DurationMs = RecordingStored, saved.RecordingID.String(), saved.DurationMillis
	c.send(TypeRecordingStatus, t.requestID, status)
}

// runCommandTurn handles a typed intent sent without speech. It skips
// recognition and classification but not validation or confirmation.
func (c *conversation) runCommandTurn(t *turn, it intent.Intent, confirmationID string) {
	defer c.finishTurn(t)

	q, ok := c.currentQuestion(t)
	if !ok {
		return
	}
	validated, err := intent.Validate(it, c.classifierInput("", q))
	if err != nil {
		c.respond(t, reply{kind: kindClarify, code: CodeInvalidOption, text: msgInvalidOption})
		return
	}
	c.svc.Metrics.Inc(MetricIntents, string(validated.Name))
	c.log.Info().Str("event", "voice.command").Str("intent", string(validated.Name)).
		Str("request_id", t.requestID).Msg("command received")

	c.respond(t, c.act(t, validated, "", q, confirmationID))
}

func (c *conversation) currentQuestion(t *turn) (*session.StudentQuestionResponse, bool) {
	ctx, cancel := context.WithTimeout(t.ctx, c.svc.Config.ActionTimeout)
	defer cancel()
	q, err := c.svc.Sessions.GetCurrentQuestion(ctx, c.sessionID, c.userID)
	if err != nil {
		if t.ctx.Err() != nil {
			return nil, false
		}
		t.responded.Store(true)
		t.setOutcome("failed")
		c.domainError(t.requestID, err, t.streamLabel())
		return nil, false
	}
	return q, true
}

// classifierInput builds what a classifier may see: the transcript and the
// options exactly as the student sees them. session.StudentQuestionResponse
// carries no correctness or grading data, so none can leak from here.
func (c *conversation) classifierInput(transcript string, q *session.StudentQuestionResponse) intent.Input {
	in := intent.Input{Transcript: transcript, QuestionType: string(q.Type)}
	for _, o := range q.Options {
		in.Options = append(in.Options, intent.Option{Key: o.OptionKey, Text: o.OptionText})
	}
	c.dialogMu.Lock()
	in.AwaitingConfirmation = c.pending != nil
	c.dialogMu.Unlock()
	return in
}

// transcribe streams the turn's audio to the recognizer and returns the final
// transcript. It reports false if the turn failed or was cancelled.
func (c *conversation) transcribe(t *turn, format speech.AudioFormat) (string, bool) {
	cfg := c.svc.Config.STT
	m := c.svc.Metrics

	openCtx, cancelOpen := context.WithTimeout(t.ctx, cfg.ConnectTimeout)
	start := c.svc.now()
	stream, err := c.svc.STT.OpenStream(openCtx, speech.STTConfig{Format: format, Language: cfg.Language})
	cancelOpen()
	m.Observe(MetricProviderLatency, c.svc.now().Sub(start), "stt_open")
	if err != nil {
		if t.ctx.Err() != nil {
			return "", false
		}
		code, retryable := CodeSTTFailed, true
		switch {
		case errors.Is(err, speech.ErrUnavailable):
			code, retryable = CodeSTTUnavailable, false
		case errors.Is(err, speech.ErrBusy), errors.Is(err, speech.ErrRateLimited):
			code = CodeProviderBusy
		}
		m.Inc(MetricProviderErrors, "stt", code)
		c.log.Error().Err(err).Str("event", "voice.stt_open_failed").Str("request_id", t.requestID).Msg("could not open recognition stream")
		c.failTurn(t, code, "speech recognition is unavailable, please try again", retryable)
		return "", false
	}

	// recvCtx is cancelled when the pump fails or the final-transcript
	// deadline passes; pumpCtx stops the pump when this function returns.
	recvCtx, cancelRecv := context.WithCancelCause(t.ctx)
	pumpCtx, cancelPump := context.WithCancel(t.ctx)
	pumpDone := make(chan struct{})
	// audioEnded is closed once the client's audio.end has been observed.
	audioEnded := make(chan struct{})
	var finalTimer *time.Timer
	go func() {
		defer close(pumpDone)
		for {
			select {
			case <-pumpCtx.Done():
				return
			case chunk, more := <-t.chunks:
				if !more {
					// audio.end: flush, then bound the wait for the result.
					close(audioEnded)
					if err := stream.CloseSend(pumpCtx); err != nil {
						cancelRecv(err)
						return
					}
					finalTimer = time.AfterFunc(cfg.FinalTimeout, func() { cancelRecv(errSTTTimeout) })
					return
				}
				if err := stream.Send(pumpCtx, chunk); err != nil {
					cancelRecv(err)
					return
				}
			}
		}
	}()
	defer func() {
		cancelPump()
		<-pumpDone
		if finalTimer != nil {
			finalTimer.Stop()
		}
		cancelRecv(nil)
		_ = stream.Close()
	}()

	var finals []string
	for {
		ev, err := stream.Recv(recvCtx)
		// Only the bare sentinel means "complete". An error that merely wraps
		// io.EOF is a broken stream and must not pass for a full transcript.
		if err == io.EOF {
			break
		}
		if err != nil {
			if t.ctx.Err() != nil {
				return "", false
			}
			code, message := CodeSTTFailed, "speech recognition failed, please try again"
			if errors.Is(context.Cause(recvCtx), errSTTTimeout) {
				code, message = CodeSTTTimeout, "speech recognition timed out, please try again"
			}
			m.Inc(MetricProviderErrors, "stt", code)
			c.log.Error().Err(err).Str("event", "voice.stt_failed").Str("code", code).Str("request_id", t.requestID).Msg("recognition stream failed")
			c.failTurn(t, code, message, true)
			return "", false
		}

		text := strings.Join(finals, " ")
		if ev.Final {
			finals = append(finals, strings.TrimSpace(ev.Text))
			text = strings.Join(finals, " ")
		} else if ev.Text != "" {
			text = strings.TrimSpace(text + " " + ev.Text)
		}
		// Interim transcripts are advisory; dropping one under load is harmless.
		c.sendLossy(TypeSTTPartial, t.requestID, TranscriptPayload{StreamID: t.streamLabel(), Text: text})
	}

	// The provider finished. If the student had not finished speaking, the
	// transcript may be incomplete and must not be acted on.
	select {
	case <-audioEnded:
	default:
		if t.ctx.Err() != nil {
			return "", false
		}
		m.Inc(MetricProviderErrors, "stt", "closed_early")
		c.failTurn(t, CodeSTTFailed, "speech recognition ended unexpectedly, please try again", true)
		return "", false
	}
	if cause := context.Cause(recvCtx); cause != nil {
		if t.ctx.Err() != nil {
			return "", false
		}
		m.Inc(MetricProviderErrors, "stt", CodeSTTFailed)
		c.failTurn(t, CodeSTTFailed, "speech recognition failed, please try again", true)
		return "", false
	}

	if !t.endedAt.IsZero() {
		m.Observe(MetricProviderLatency, c.svc.now().Sub(t.endedAt), "stt_final")
	}
	return strings.TrimSpace(strings.Join(finals, " ")), true
}

// ---------------------------------------------------------------------------
// Responding
// ---------------------------------------------------------------------------

// respond delivers a reply: always as text, and as audio when TTS is
// configured. A turn that was cancelled in the meantime says nothing.
func (c *conversation) respond(t *turn, r reply) {
	if r.text == "" || t.ctx.Err() != nil {
		return
	}
	t.responded.Store(true)
	if r.code != "" {
		t.setOutcome(r.code)
	}

	responseID := uuid.New()
	audio := c.svc.ttsEnabled()
	c.send(TypeResponse, t.requestID, ResponsePayload{
		ResponseID: responseID.String(),
		StreamID:   t.streamLabel(),
		Kind:       r.kind,
		Code:       r.code,
		Text:       r.text,
		Audio:      audio,
	})
	if audio {
		c.speak(t, responseID, r.text)
	}
}

// speak synthesizes text and streams the audio to the client. Every frame
// carries the response ID, and frames of a cancelled response are discarded
// both here and, if already queued, at the moment they would be written.
func (c *conversation) speak(t *turn, responseID uuid.UUID, text string) {
	cfg := c.svc.Config.TTS
	m := c.svc.Metrics

	cancelCtx, cancel := context.WithCancelCause(t.ctx)
	defer cancel(nil)
	ctx, cancelTimeout := context.WithTimeout(cancelCtx, cfg.Timeout)
	defer cancelTimeout()

	epoch := c.speechEpoch.Load()
	c.mu.Lock()
	t.speechCancel = cancel
	t.responseID = responseID.String()
	c.lastResponseID = t.responseID
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		t.speechCancel = nil
		c.mu.Unlock()
	}()

	var seq uint32
	end := func(reason, code string) {
		c.send(TypeTTSEnd, t.requestID, TTSEndPayload{ResponseID: responseID.String(), Reason: reason, Code: code, Chunks: seq})
	}
	// stopped translates a done context into the reason reported to the client.
	stopped := func() (string, string) {
		switch cause := context.Cause(ctx); {
		case errors.Is(cause, errBargeIn):
			return "barge_in", ""
		case errors.Is(cause, context.DeadlineExceeded):
			m.Inc(MetricProviderErrors, "tts", "timeout")
			return "error", CodeTTSFailed
		default:
			return "cancelled", ""
		}
	}

	start := c.svc.now()
	stream, err := c.svc.TTS.Synthesize(ctx, speech.TTSRequest{Text: text})
	if err != nil {
		if ctx.Err() != nil {
			end(stopped())
			return
		}
		code := CodeTTSFailed
		switch {
		case errors.Is(err, speech.ErrUnavailable):
			code = CodeTTSUnavailable
		case errors.Is(err, speech.ErrBusy), errors.Is(err, speech.ErrRateLimited):
			code = CodeProviderBusy
		}
		m.Inc(MetricProviderErrors, "tts", code)
		c.log.Error().Err(err).Str("event", "voice.tts_failed").Str("request_id", t.requestID).Msg("speech synthesis failed")
		// The text response was already delivered, so the student is not left
		// without an answer; only the audio is missing.
		end("error", code)
		return
	}
	defer stream.Close()

	c.send(TypeTTSStart, t.requestID, TTSStartPayload{ResponseID: responseID.String(), Format: c.svc.TTS.Format()})

	// A frame is stale once the student has interrupted since this response
	// began. Normal completion does not make queued frames stale.
	stale := func() bool { return c.speechEpoch.Load() != epoch }
	for {
		chunk, err := stream.Next(ctx)
		if err == io.EOF {
			break
		}
		if err == nil && seq == 0 {
			m.Observe(MetricProviderLatency, c.svc.now().Sub(start), "tts_first_chunk")
		}
		if err == nil && len(chunk) > 0 {
			// SendContext blocks while the client is slow, which holds back
			// the provider read instead of buffering audio without limit.
			err = c.conn.SendContext(ctx, ws.Message{Binary: true, Data: encodeFrame(responseID, seq, chunk), Drop: stale})
			if err == nil {
				seq++
			}
		}
		if err != nil {
			switch {
			case ctx.Err() != nil:
				reason, code := stopped()
				if reason != "error" {
					m.Inc(MetricCancellations, "tts_"+reason)
				}
				end(reason, code)
			case errors.Is(err, ws.ErrClosed):
			default:
				m.Inc(MetricProviderErrors, "tts", CodeTTSFailed)
				c.log.Error().Err(err).Str("event", "voice.tts_failed").Str("request_id", t.requestID).Msg("speech synthesis stream failed")
				end("error", CodeTTSFailed)
			}
			return
		}
	}
	end("completed", "")
}
