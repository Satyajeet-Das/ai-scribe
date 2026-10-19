package voice

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech/speechtest"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
	"github.com/Satyajeet-Das/ai-scribe/internal/voice/intent"
)

// ---------------------------------------------------------------------------
// TTS, barge-in and stale audio
// ---------------------------------------------------------------------------

// audioFor collects the audio frames of one response read so far.
func audioFor(c *client, responseID string) []audioFrame {
	var out []audioFrame
	for _, f := range c.seen {
		if f.audio != nil && f.audio.ID.String() == responseID {
			out = append(out, *f.audio)
		}
	}
	return out
}

func TestTTS_StreamsChunksTaggedWithResponseID(t *testing.T) {
	h := newHarness(t)
	h.tts.Chunks = 4
	c, _ := h.connect()

	id := c.send(TypeCommand, CommandPayload{Intent: "repeat_question"})
	resp := decode[ResponsePayload](t, c.until(TypeResponse))
	start := decode[TTSStartPayload](t, c.until(TypeTTSStart))
	endEnv := c.until(TypeTTSEnd)
	end := decode[TTSEndPayload](t, endEnv)

	assert.True(t, resp.Audio)
	assert.Equal(t, resp.ResponseID, start.ResponseID)
	assert.Equal(t, speech.EncodingPCM16LE, start.Format.Encoding)
	assert.Equal(t, 24000, start.Format.SampleRate)
	assert.Equal(t, id, endEnv.ReplyTo)
	assert.Equal(t, "completed", end.Reason)
	assert.EqualValues(t, 4, end.Chunks)

	frames := audioFor(c, resp.ResponseID)
	require.Len(t, frames, 4)
	for i, f := range frames {
		assert.EqualValues(t, i, f.Seq, "chunks arrive in order")
		assert.Len(t, f.Audio, 320)
	}
	assert.Equal(t, []string{resp.Text}, h.tts.Texts())
	eventually(t, func() bool { return h.tts.Closed() == 1 }, "synthesis released")
	assert.EqualValues(t, 1, h.svc.Metrics.Samples(MetricProviderLatency, "tts_first_chunk"))
}

func TestTTS_BargeInCancelsSpeechAndDiscardsStaleChunks(t *testing.T) {
	h := newHarness(t)
	gate := make(chan struct{})
	h.tts.Gate = gate
	h.tts.Chunks = 50
	h.stt.Set(func(s *speechtest.STT) { s.Finals = []string{"next question"} })
	c, _ := h.connect()

	c.send(TypeCommand, CommandPayload{Intent: "repeat_question"})
	first := decode[ResponsePayload](t, c.until(TypeResponse))
	c.until(TypeTTSStart)
	// Two chunks of the first response are released, then the student speaks.
	gate <- struct{}{}
	gate <- struct{}{}
	c.untilAudio(1)

	stream, _ := startStream(c)

	end := decode[TTSEndPayload](t, c.until(TypeTTSEnd))
	assert.Equal(t, first.ResponseID, end.ResponseID)
	assert.Equal(t, "barge_in", end.Reason)
	eventually(t, func() bool { return h.tts.Closed() == 1 }, "cancelled synthesis released its provider stream")

	// The provider may still be willing to produce audio; none of it may
	// reach the client under the cancelled response ID.
	close(gate)
	marker := len(c.seen)
	c.chunk(stream, 0, 640)
	n := uint32(1)
	c.send(TypeAudioEnd, AudioEndPayload{StreamID: stream.String(), Chunks: &n})

	second := decode[ResponsePayload](t, c.until(TypeResponse))
	end = decode[TTSEndPayload](t, c.until(TypeTTSEnd))
	assert.Equal(t, second.ResponseID, end.ResponseID)
	assert.Equal(t, "completed", end.Reason)
	assert.NotEqual(t, first.ResponseID, second.ResponseID)
	assert.Contains(t, second.Text, "Question 2 of 3")

	for _, f := range c.seen[marker:] {
		if f.audio != nil {
			assert.Equal(t, second.ResponseID, f.audio.ID.String(), "no late chunk of the cancelled response is delivered")
		}
	}
	assert.Len(t, audioFor(c, first.ResponseID), 2)
	assert.EqualValues(t, 1, h.svc.Metrics.Value(MetricCancellations, "barge_in"))
}

func TestTTS_QueuedChunksOfCancelledResponseAreDropped(t *testing.T) {
	// The Drop hook is what discards frames that were queued before the
	// cancellation but not yet written.
	h := newHarness(t)
	c, _ := h.connect()
	conv := h.onlyConversation(t)

	ctx, cancel := context.WithCancel(context.Background())
	stale := uuid.New()
	live := uuid.New()
	drop := func() bool { return ctx.Err() != nil }

	cancel()
	require.NoError(t, conv.conn.SendContext(context.Background(), wsFrame(stale, 0, drop)))
	require.NoError(t, conv.conn.SendContext(context.Background(), wsFrame(live, 0, nil)))

	for {
		f, err := c.read()
		require.NoError(t, err)
		if f.audio != nil && f.audio.ID == live {
			break
		}
	}
	assert.Empty(t, audioFor(c, stale.String()))
}

func TestTTS_ClientCancel(t *testing.T) {
	h := newHarness(t)
	gate := make(chan struct{})
	h.tts.Gate = gate
	h.tts.Chunks = 50
	c, _ := h.connect()

	c.send(TypeCommand, CommandPayload{Intent: "repeat_question"})
	resp := decode[ResponsePayload](t, c.until(TypeResponse))
	c.until(TypeTTSStart)
	gate <- struct{}{}

	c.send(TypeTTSCancel, TTSCancelPayload{ResponseID: resp.ResponseID})
	end := decode[TTSEndPayload](t, c.until(TypeTTSEnd))
	assert.Equal(t, "cancelled", end.Reason)
	eventually(t, func() bool { return h.tts.Closed() == 1 }, "synthesis released")

	// Cancelling again, after it has finished, is harmless.
	id := c.send(TypeTTSCancel, TTSCancelPayload{ResponseID: resp.ResponseID})
	for c.until(TypeAck).ReplyTo != id {
	}
}

func TestTTS_ProviderFailureStillDeliversText(t *testing.T) {
	cases := []struct {
		name   string
		script func(tts *speechtest.TTS)
		code   string
		chunks uint32
	}{
		{"cannot start", func(tts *speechtest.TTS) { tts.SynthErr = errors.New("502 bad gateway") }, CodeTTSFailed, 0},
		{"at capacity", func(tts *speechtest.TTS) { tts.SynthErr = speech.ErrBusy }, CodeProviderBusy, 0},
		{"fails mid-stream", func(tts *speechtest.TTS) { tts.Chunks, tts.FailAfter = 5, 2 }, CodeTTSFailed, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			tc.script(h.tts)
			c, _ := h.connect()

			c.send(TypeCommand, CommandPayload{Intent: "repeat_question"})
			resp := decode[ResponsePayload](t, c.until(TypeResponse))
			end := decode[TTSEndPayload](t, c.until(TypeTTSEnd))

			assert.Contains(t, resp.Text, "capital of France", "the student still gets the response as text")
			assert.Equal(t, "error", end.Reason)
			assert.Equal(t, tc.code, end.Code)
			assert.Equal(t, tc.chunks, end.Chunks)

			// The connection keeps working.
			id := c.send(TypePing, nil)
			for c.until(TypePong).ReplyTo != id {
			}
		})
	}
}

func TestTTS_TimeoutIsBounded(t *testing.T) {
	h := newHarness(t, withConfig(func(cfg *config.VoiceConfig) { cfg.TTS.Timeout = 100 * time.Millisecond }))
	h.tts.Gate = make(chan struct{}) // never releases a chunk
	c, _ := h.connect()

	c.send(TypeCommand, CommandPayload{Intent: "repeat_question"})
	end := decode[TTSEndPayload](t, c.until(TypeTTSEnd))
	assert.Equal(t, "error", end.Reason)
	assert.Equal(t, CodeTTSFailed, end.Code)
	eventually(t, func() bool { return h.tts.Closed() == 1 }, "synthesis released")
}

func TestStaleTurn_InterruptedBeforeInterpretationDoesNothing(t *testing.T) {
	h := newHarness(t, withoutTTS())
	release := make(chan struct{})
	h.cls.set(func(ctx context.Context, _ intent.Input) (intent.Intent, error) {
		select {
		case <-release:
		case <-ctx.Done():
			return intent.Intent{}, ctx.Err()
		}
		return intent.Intent{Name: intent.NextQuestion, Confidence: 1}, nil
	})
	c, _ := h.connect()

	// "next question" is being classified when the student changes their mind.
	_, firstID := h.say(c, "next question")
	c.until(TypeSTTFinal)
	repeatID := c.send(TypeCommand, CommandPayload{Intent: "repeat_question"})

	env := c.until(TypeError)
	assert.Equal(t, firstID, env.ReplyTo)
	assert.Equal(t, CodeTurnCancelled, decode[ErrorPayload](t, env).Code)
	for c.until(TypeResponse).ReplyTo != repeatID {
	}
	close(release)

	assert.Zero(t, h.exam.count("NextQuestion"), "a superseded request must never act on the exam")
}

// ---------------------------------------------------------------------------
// Session end, expiry and re-validation
// ---------------------------------------------------------------------------

func TestSession_ExpiredOrSubmittedEndsTheConnection(t *testing.T) {
	cases := []struct {
		status session.Status
		code   string
		reason string
	}{
		{session.StatusExpired, CodeSessionExpired, "expired"},
		{session.StatusSubmitted, CodeSessionSubmitted, "submitted"},
	}
	for _, tc := range cases {
		t.Run("command on "+string(tc.status)+" session", func(t *testing.T) {
			h := newHarness(t, withoutTTS())
			c, _ := h.connect()
			h.exam.setStatus(tc.status)

			c.send(TypeCommand, CommandPayload{Intent: "next_question"})
			assert.Equal(t, tc.code, c.untilError().Code)
			assert.Equal(t, tc.reason, decode[SessionClosedPayload](t, c.until(TypeSessionClosed)).Reason)
			assert.Equal(t, websocket.StatusCode(CloseSessionEnded), c.closeCode())
			assert.Zero(t, h.exam.count("NextQuestion"))
			eventually(t, func() bool { return h.svc.ActiveConnections() == 0 }, "slot released")
		})
		t.Run("audio on "+string(tc.status)+" session", func(t *testing.T) {
			h := newHarness(t, withoutTTS())
			c, _ := h.connect()
			h.exam.setStatus(tc.status)

			startStream(c)
			assert.Equal(t, tc.code, c.untilError().Code)
			assert.Equal(t, websocket.StatusCode(CloseSessionEnded), c.closeCode())
			assert.Zero(t, h.stt.Opened(), "no audio is accepted for a finished session")
		})
		t.Run("confirm on "+string(tc.status)+" session saves nothing", func(t *testing.T) {
			h := newHarness(t, withoutTTS())
			c, _ := h.connect()
			qid := h.exam.currentQuestionID()
			command(t, c, CommandPayload{Intent: "select_option", OptionKey: "B"})

			// The session ends between the selection and the confirmation;
			// only the answer service can tell, and it refuses.
			h.exam.setStatus(tc.status)
			conv := h.onlyConversation(t)
			q := h.exam.questions[0]
			q.CurrentIndex = 1
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			conv.act(&turn{requestID: "late", ctx: ctx, cancel: cancel}, intent.Intent{Name: intent.Confirm, Confidence: 1}, "", &q, "")

			result := decode[ActionResultPayload](t, c.until(TypeActionResult))
			assert.False(t, result.Saved)
			assert.Equal(t, tc.code, result.Code)
			assert.Equal(t, websocket.StatusCode(CloseSessionEnded), c.closeCode())
			_, saved := h.exam.saved(qid)
			assert.False(t, saved)
		})
	}
}

func TestSession_WatchdogClosesWhenSessionEndsElsewhere(t *testing.T) {
	h := newHarness(t, withoutTTS())
	h.svc.recheckInterval = 50 * time.Millisecond
	c, _ := h.connect()

	// Submitted through the REST API, or expired by the background job.
	h.exam.setStatus(session.StatusSubmitted)

	assert.Equal(t, "submitted", decode[SessionClosedPayload](t, c.until(TypeSessionClosed)).Reason)
	assert.Equal(t, websocket.StatusCode(CloseSessionEnded), c.closeCode())
}

// The periodic re-check stays at its default (one minute) in these tests, so
// a prompt close can only come from SessionEnded.
func TestSessionEnded_ClosesTheConnectionAtOnce(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()

	h.exam.setStatus(session.StatusSubmitted)
	h.svc.SessionEnded(h.sessionID)

	assert.Equal(t, "submitted", decode[SessionClosedPayload](t, c.until(TypeSessionClosed)).Reason)
	assert.Equal(t, websocket.StatusCode(CloseSessionEnded), c.closeCode())
	eventually(t, func() bool { return h.svc.ActiveConnections() == 0 }, "slot released")
}

func TestSessionEnded_DuringDictationKeepsNothing(t *testing.T) {
	h := newHarness(t, withoutTTS(), withRecorder())
	c, _ := h.connect()
	toEssay(t, h, c)

	stream := uuid.New()
	c.send(TypeAudioStart, AudioStartPayload{StreamID: stream.String(), Encoding: "pcm_s16le", SampleRate: 16000, Channels: 1, Mode: AudioModeAnswer})
	c.chunk(stream, 0, 640)
	eventually(t, func() bool { return h.stt.Opened() == 1 }, "the stream is open")

	h.exam.setStatus(session.StatusSubmitted)
	h.svc.SessionEnded(h.sessionID)

	assert.Equal(t, "submitted", decode[SessionClosedPayload](t, c.until(TypeSessionClosed)).Reason)
	assert.Equal(t, websocket.StatusCode(CloseSessionEnded), c.closeCode())
	eventually(t, func() bool { return h.svc.ActiveConnections() == 0 }, "slot released")
	assert.Empty(t, h.rec.inputs(), "no audio of an ended exam is kept")
	assert.Zero(t, h.exam.count("SubmitAnswer"))
}

func TestSessionEnded_WhileTheRecordingIsBeingStoredKeepsNothing(t *testing.T) {
	h := newHarness(t, withoutTTS(), withRecorder())
	h.rec.gate = make(chan struct{}) // the store never answers
	c, _ := h.connect()
	toEssay(t, h, c)

	h.sayAs(c, AudioModeAnswer, "Plants turn light into energy")
	c.until(TypeSTTFinal)

	h.exam.setStatus(session.StatusExpired)
	h.svc.SessionEnded(h.sessionID)

	assert.Equal(t, "expired", decode[SessionClosedPayload](t, c.until(TypeSessionClosed)).Reason)
	assert.Equal(t, websocket.StatusCode(CloseSessionEnded), c.closeCode())
	eventually(t, func() bool { return h.svc.ActiveConnections() == 0 }, "slot released")
	assert.Empty(t, h.rec.inputs(), "the save in progress is abandoned")
	assert.Zero(t, h.exam.count("SubmitAnswer"))
}

func TestSessionEnded_IgnoresSessionsWithoutAConnection(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()

	h.svc.SessionEnded(uuid.New()) // another session: this one is untouched
	id := c.send(TypePing, nil)
	assert.Equal(t, id, c.until(TypePong).ReplyTo)

	c.conn.Close(websocket.StatusNormalClosure, "")
	eventually(t, func() bool { return h.svc.ActiveConnections() == 0 }, "slot released")
	h.svc.SessionEnded(h.sessionID) // after the connection has gone: a no-op
}

func TestSession_WatchdogSurvivesTransientOutage(t *testing.T) {
	h := newHarness(t, withoutTTS())
	h.svc.recheckInterval = 30 * time.Millisecond
	c, _ := h.connect()

	h.exam.fail("GetSession", errors.New("database connection refused"))
	time.Sleep(150 * time.Millisecond)
	h.exam.fail("GetSession", nil)

	id := c.send(TypePing, nil)
	assert.Equal(t, id, c.until(TypePong).ReplyTo, "a storage blip must not end the exam connection")
}

func TestSession_DeadlineTriggersRevalidation(t *testing.T) {
	h := newHarness(t, withoutTTS())
	h.exam.mu.Lock()
	h.exam.durationMins = 1
	h.exam.sess.StartedAt = time.Now().Add(-time.Minute + 150*time.Millisecond)
	h.exam.mu.Unlock()
	c, _ := h.connect()

	// The session service flips to EXPIRED when time runs out (as GetSession
	// does passively in production).
	time.AfterFunc(150*time.Millisecond, func() { h.exam.setStatus(session.StatusExpired) })

	assert.Equal(t, "expired", decode[SessionClosedPayload](t, c.until(TypeSessionClosed)).Reason)
	assert.Equal(t, websocket.StatusCode(CloseSessionEnded), c.closeCode())
}

// ---------------------------------------------------------------------------
// Token expiry
// ---------------------------------------------------------------------------

func TestAuth_ExpiryWarnsThenCloses(t *testing.T) {
	h := newHarness(t, withoutTTS(), withConfig(func(cfg *config.VoiceConfig) { cfg.AuthExpiryGrace = 50 * time.Millisecond }))
	h.svc.authWarnLead = 100 * time.Millisecond
	h.auth.issue(h.token, h.student, time.Now().Add(200*time.Millisecond))
	c, _ := h.connect()

	c.until(TypeAuthExpiring)
	assert.Equal(t, "auth_expired", decode[SessionClosedPayload](t, c.until(TypeSessionClosed)).Reason)
	assert.Equal(t, websocket.StatusCode(CloseUnauthorized), c.closeCode())
}

func TestAuth_RefreshKeepsConnectionAlive(t *testing.T) {
	h := newHarness(t, withoutTTS(), withConfig(func(cfg *config.VoiceConfig) { cfg.AuthExpiryGrace = 50 * time.Millisecond }))
	h.svc.authWarnLead = 100 * time.Millisecond
	h.auth.issue(h.token, h.student, time.Now().Add(200*time.Millisecond))
	h.auth.issue("fresh-token", h.student, time.Now().Add(time.Hour))
	c, _ := h.connect()

	c.until(TypeAuthExpiring)
	id := c.send(TypeAuthRefresh, AuthRefreshPayload{Token: "fresh-token"})
	for c.until(TypeAck).ReplyTo != id {
	}

	time.Sleep(400 * time.Millisecond)
	pingID := c.send(TypePing, nil)
	assert.Equal(t, pingID, c.until(TypePong).ReplyTo)
}

func TestAuth_RefreshRejectsInvalidAndForeignTokens(t *testing.T) {
	h := newHarness(t, withoutTTS())
	h.auth.issue("other-token", uuid.New(), time.Time{})
	c, _ := h.connect()

	c.send(TypeAuthRefresh, AuthRefreshPayload{Token: "forged"})
	assert.Equal(t, CodeUnauthorized, c.untilError().Code)

	// A valid token for a different user must not be adopted by this session.
	c.send(TypeAuthRefresh, AuthRefreshPayload{Token: "other-token"})
	assert.Equal(t, CodeForbidden, c.untilError().Code)
	assert.Equal(t, websocket.StatusCode(CloseForbidden), c.closeCode())
}

// ---------------------------------------------------------------------------
// Disconnect, reconnect, shutdown
// ---------------------------------------------------------------------------

func TestReconnect_ReplacesOldConnectionAndRevalidates(t *testing.T) {
	h := newHarness(t, withoutTTS())
	old, ready := h.connect()
	assert.False(t, ready.Resumed)

	// State left on the old connection must not leak into the new one.
	command(t, old, CommandPayload{Intent: "select_option", OptionKey: "B"})
	command(t, old, CommandPayload{Intent: "next_question"})

	fresh, ready := h.connect()
	assert.True(t, ready.Resumed)
	assert.Equal(t, 2, ready.Session.Question.CurrentIndex, "position comes from the persisted session")
	assert.Equal(t, websocket.StatusCode(CloseReplaced), old.closeCode())

	_, resp := command(t, fresh, CommandPayload{Intent: "confirm"})
	assert.Equal(t, CodeNothingToConfirm, resp.Code, "a pending confirmation does not survive a reconnect")
	assert.Zero(t, h.exam.count("SubmitAnswer"))

	eventually(t, func() bool { return h.svc.ActiveConnections() == 1 }, "old slot released")
	assert.EqualValues(t, 1, h.svc.Metrics.Value(MetricReconnects))
	assert.EqualValues(t, 1, h.svc.Metrics.Value(MetricConnectionsClosed, "replaced"))
}

func TestReconnect_AfterSessionEndedIsRefused(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()
	_ = c.conn.Close(websocket.StatusNormalClosure, "")
	eventually(t, func() bool { return h.svc.ActiveConnections() == 0 }, "slot released")

	h.exam.setStatus(session.StatusSubmitted)
	_, resp, err := h.dialRaw(dialOptions{token: h.token})
	require.Error(t, err)
	assert.Equal(t, 409, resp.StatusCode)
}

func TestDisconnect_MidStreamReleasesEverything(t *testing.T) {
	h := newHarness(t)
	h.tts.Gate = make(chan struct{})
	h.stt.Set(func(s *speechtest.STT) { s.SendGate = make(chan struct{}) })
	c, _ := h.connect()

	// Speech is being synthesized and audio is being captured when the
	// socket drops without a close handshake.
	c.send(TypeCommand, CommandPayload{Intent: "repeat_question"})
	c.until(TypeTTSStart)
	stream, _ := startStream(c)
	c.chunk(stream, 0, 640)
	eventually(t, func() bool { return h.stt.Opened() == 1 }, "recognition started")
	_ = c.conn.CloseNow()

	eventually(t, func() bool { return h.svc.ActiveConnections() == 0 }, "slot released")
	eventually(t, func() bool { return h.stt.Closed() == 1 && h.tts.Closed() == 1 }, "provider streams released")
	assert.EqualValues(t, 0, h.svc.Metrics.Value(MetricConnectionsActive))
	// The harness cleanup additionally asserts that no goroutine is left.
}

func TestShutdown_ClosesConnectionsGracefully(t *testing.T) {
	h := newHarness(t)
	h.tts.Gate = make(chan struct{})
	c, _ := h.connect()
	c.send(TypeCommand, CommandPayload{Intent: "repeat_question"})
	c.until(TypeTTSStart)

	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
		defer cancel()
		done <- h.svc.Shutdown(ctx)
	}()

	assert.Equal(t, websocket.StatusGoingAway, c.closeCode())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(waitTimeout):
		t.Fatal("shutdown did not return")
	}
	assert.Zero(t, h.svc.ActiveConnections())
	assert.Equal(t, 1, h.tts.Closed())

	_, resp, err := h.dialRaw(dialOptions{token: h.token})
	require.Error(t, err)
	assert.Equal(t, 503, resp.StatusCode, "no new connections once shutdown has begun")
}

func TestSlowConsumer_IsDisconnectedNotBuffered(t *testing.T) {
	h := newHarness(t, withoutTTS(), withConfig(func(cfg *config.VoiceConfig) { cfg.SendQueueSize = 4 }))
	c, _ := h.connect()
	conv := h.onlyConversation(t)

	// The client stops reading while the server keeps producing.
	pad := map[string]string{"pad": strings.Repeat("x", 2048)}
	for i := 0; i < 20000; i++ {
		conv.send(TypePong, "flood", pad)
	}
	eventually(t, func() bool { return h.svc.Metrics.Value(MetricFailures, "slow_consumer") >= 1 }, "slow consumer detected")
	assert.Equal(t, websocket.StatusCode(CloseSlowConsumer), c.closeCode())
}
