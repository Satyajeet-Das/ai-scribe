package voice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech/speechtest"
	"github.com/Satyajeet-Das/ai-scribe/internal/voice/intent"
)

func startStream(c *client) (uuid.UUID, string) {
	stream := uuid.New()
	id := c.send(TypeAudioStart, AudioStartPayload{StreamID: stream.String(), Encoding: "pcm_s16le", SampleRate: 16000, Channels: 1})
	return stream, id
}

func TestAudio_PartialAndFinalTranscripts(t *testing.T) {
	h := newHarness(t, withoutTTS())
	h.stt.Set(func(s *speechtest.STT) {
		s.PartialPerChunk = true
		s.Finals = []string{"repeat", "the question"}
	})
	c, _ := h.connect()

	stream, startID := c.speak(3)

	partial := decode[TranscriptPayload](t, c.until(TypeSTTPartial))
	assert.Equal(t, stream.String(), partial.StreamID)

	finalEnv := c.until(TypeSTTFinal)
	assert.Equal(t, startID, finalEnv.ReplyTo, "results are correlated to the audio.start request")
	assert.Equal(t, "repeat the question", decode[TranscriptPayload](t, finalEnv).Text)

	it := decode[IntentPayload](t, c.until(TypeIntent))
	assert.Equal(t, "repeat_question", it.Intent)

	resp := decode[ResponsePayload](t, c.until(TypeResponse))
	assert.Equal(t, kindQuestion, resp.Kind)
	assert.Contains(t, resp.Text, "capital of France")
	assert.False(t, resp.Audio)

	st := h.stt.Last()
	assert.Equal(t, 3, st.Chunks())
	assert.Equal(t, 3*640, st.Bytes())
	assert.Equal(t, speech.AudioFormat{Encoding: "pcm_s16le", SampleRate: 16000, Channels: 1}, st.Config.Format)
	eventually(t, func() bool { return h.stt.Closed() == 1 }, "recognition stream released")
	assert.EqualValues(t, 1, h.svc.Metrics.Value(MetricStreamsEnded, "completed"))
	assert.EqualValues(t, 1, h.svc.Metrics.Samples(MetricProviderLatency, "stt_final"))
}

func TestAudio_StartValidation(t *testing.T) {
	h := newHarness(t)
	c, _ := h.connect()

	cases := map[string]AudioStartPayload{
		"unsupported encoding": {StreamID: uuid.NewString(), Encoding: "opus", SampleRate: 16000, Channels: 1},
		"unsupported rate":     {StreamID: uuid.NewString(), Encoding: "pcm_s16le", SampleRate: 44100, Channels: 1},
		"stereo":               {StreamID: uuid.NewString(), Encoding: "pcm_s16le", SampleRate: 16000, Channels: 2},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			c.send(TypeAudioStart, p)
			assert.Equal(t, CodeInvalidAudioFormat, c.untilError().Code)
		})
	}
	t.Run("stream id must be a uuid", func(t *testing.T) {
		c.send(TypeAudioStart, AudioStartPayload{StreamID: "abc", Encoding: "pcm_s16le", SampleRate: 16000, Channels: 1})
		assert.Equal(t, CodeInvalidMessage, c.untilError().Code)
	})
	assert.Zero(t, h.stt.Opened())
}

func TestAudio_ChunkValidation(t *testing.T) {
	cases := []struct {
		name string
		send func(c *client, stream uuid.UUID)
		code string
	}{
		{"out of sequence", func(c *client, s uuid.UUID) { c.chunk(s, 0, 640); c.chunk(s, 2, 640) }, CodeChunkOutOfOrder},
		{"replayed chunk", func(c *client, s uuid.UUID) { c.chunk(s, 0, 640); c.chunk(s, 0, 640) }, CodeChunkOutOfOrder},
		{"oversized chunk", func(c *client, s uuid.UUID) { c.chunk(s, 0, 4096+2) }, CodeChunkTooLarge},
		{"odd length is not whole samples", func(c *client, s uuid.UUID) { c.chunk(s, 0, 641) }, CodeInvalidFrame},
		{"empty chunk", func(c *client, s uuid.UUID) { c.chunk(s, 0, 0) }, CodeInvalidFrame},
		{"stream byte limit", func(c *client, s uuid.UUID) {
			for i := uint32(0); i < 4; i++ {
				c.chunk(s, i, 4096)
			}
		}, CodeStreamTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, withoutTTS(), withConfig(func(cfg *config.VoiceConfig) {
				cfg.Audio.MaxChunkBytes = 4096
				cfg.Audio.MaxStreamBytes = 3 * 4096
			}))
			h.stt.Set(func(s *speechtest.STT) { s.Finals = []string{"repeat"} })
			c, _ := h.connect()

			stream, startID := startStream(c)
			tc.send(c, stream)

			env := c.until(TypeError)
			p := decode[ErrorPayload](t, env)
			assert.Equal(t, tc.code, p.Code)
			assert.Equal(t, stream.String(), p.StreamID)
			assert.Equal(t, startID, env.ReplyTo)

			// The stream is dead: more of its chunks are discarded silently,
			// its end is rejected, and nothing was interpreted.
			from := len(c.seen)
			c.chunk(stream, 5, 640)
			n := uint32(1)
			c.send(TypeAudioEnd, AudioEndPayload{StreamID: stream.String(), Chunks: &n})
			assert.Equal(t, CodeUnknownStream, c.untilError().Code)
			assert.NotContains(t, c.types(from), TypeSTTFinal)
			eventually(t, func() bool { return h.stt.Closed() == h.stt.Opened() }, "recognition stream released")

			// A fresh stream works.
			c.speak(1)
			assert.Equal(t, "repeat", decode[TranscriptPayload](t, c.until(TypeSTTFinal)).Text)
		})
	}
}

func TestAudio_EndValidation(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()

	t.Run("chunk count mismatch means audio was lost", func(t *testing.T) {
		stream, _ := startStream(c)
		c.chunk(stream, 0, 640)
		n := uint32(2)
		c.send(TypeAudioEnd, AudioEndPayload{StreamID: stream.String(), Chunks: &n})
		p := c.untilError()
		assert.Equal(t, CodeChunkCountMismatch, p.Code)
		assert.True(t, p.Retryable)
	})
	t.Run("empty stream", func(t *testing.T) {
		stream, _ := startStream(c)
		c.send(TypeAudioEnd, AudioEndPayload{StreamID: stream.String()})
		assert.Equal(t, CodeEmptyStream, c.untilError().Code)
	})
	t.Run("end for a stream that was never started", func(t *testing.T) {
		c.send(TypeAudioEnd, AudioEndPayload{StreamID: uuid.NewString()})
		assert.Equal(t, CodeUnknownStream, c.untilError().Code)
	})
	t.Run("chunk for a stream that was never started", func(t *testing.T) {
		unknown := uuid.New()
		c.chunk(unknown, 0, 640)
		p := c.untilError()
		assert.Equal(t, CodeUnknownStream, p.Code)
		assert.Equal(t, unknown.String(), p.StreamID)
	})
}

func TestAudio_OnlyOneOpenStream(t *testing.T) {
	h := newHarness(t, withoutTTS())
	h.stt.Set(func(s *speechtest.STT) { s.Finals = []string{"repeat"} })
	c, _ := h.connect()

	first, _ := startStream(c)
	c.chunk(first, 0, 640)
	second, _ := startStream(c)

	p := c.untilError()
	assert.Equal(t, CodeStreamActive, p.Code)
	assert.Equal(t, second.String(), p.StreamID)

	// The first stream is unaffected.
	n := uint32(1)
	c.send(TypeAudioEnd, AudioEndPayload{StreamID: first.String(), Chunks: &n})
	assert.Equal(t, first.String(), decode[TranscriptPayload](t, c.until(TypeSTTFinal)).StreamID)
}

func TestAudio_BackpressureIsBoundedAndFailsTheStream(t *testing.T) {
	h := newHarness(t, withoutTTS(), withConfig(func(cfg *config.VoiceConfig) {
		cfg.Audio.BufferChunks = 2
		cfg.Audio.EnqueueTimeout = 100 * time.Millisecond
	}))
	// A recognizer that accepts nothing: every Send blocks.
	h.stt.Set(func(s *speechtest.STT) { s.SendGate = make(chan struct{}) })
	c, _ := h.connect()

	stream, _ := startStream(c)
	// One chunk is held by the pump, two fill the buffer, the next cannot fit.
	for i := uint32(0); i < 8; i++ {
		c.chunk(stream, i, 640)
	}

	p := c.untilError()
	assert.Equal(t, CodeBackpressure, p.Code)
	assert.Equal(t, stream.String(), p.StreamID)
	assert.EqualValues(t, 1, h.svc.Metrics.Value(MetricFailures, CodeBackpressure))
	// Memory held for the stream never exceeded pump + buffer + the one in hand.
	assert.LessOrEqual(t, h.svc.Metrics.Value(MetricStreamBytes), int64(4*640))
	eventually(t, func() bool { return h.stt.Closed() == 1 }, "recognition stream released")

	// The connection survives and stays responsive.
	id := c.send(TypePing, nil)
	assert.Equal(t, id, c.until(TypePong).ReplyTo)
}

func TestAudio_MaxDurationEndsTheStream(t *testing.T) {
	h := newHarness(t, withoutTTS(), withConfig(func(cfg *config.VoiceConfig) {
		cfg.Audio.MaxStreamDuration = 100 * time.Millisecond
	}))
	c, _ := h.connect()

	stream, _ := startStream(c)
	c.chunk(stream, 0, 640)

	p := c.untilError()
	assert.Equal(t, CodeStreamTooLong, p.Code)
	eventually(t, func() bool { return h.stt.Closed() == 1 }, "recognition stream released")
}

func TestAudio_ClientCancelStopsRecognition(t *testing.T) {
	h := newHarness(t, withoutTTS())
	h.stt.Set(func(s *speechtest.STT) { s.Finals = []string{"next"} })
	c, _ := h.connect()

	stream, _ := startStream(c)
	c.chunk(stream, 0, 640)
	from := len(c.seen)
	cancelID := c.send(TypeAudioCancel, AudioCancelPayload{StreamID: stream.String()})

	env := c.until(TypeAck)
	for env.ReplyTo != cancelID {
		env = c.until(TypeAck)
	}
	eventually(t, func() bool { return h.stt.Closed() == 1 }, "recognition stream released")

	// Nothing is interpreted and the exam does not move.
	pingID := c.send(TypePing, nil)
	for c.until(TypePong).ReplyTo != pingID {
	}
	assert.NotContains(t, c.types(from), TypeSTTFinal)
	assert.NotContains(t, c.types(from), TypeError, "a client-requested cancel is acknowledged, not reported as an error")
	assert.Zero(t, h.exam.count("NextQuestion"))
	assert.EqualValues(t, 1, h.svc.Metrics.Value(MetricCancellations, "client"))
	assert.EqualValues(t, 1, h.svc.Metrics.Value(MetricStreamsEnded, "cancelled"))
}

func TestSTT_Failures(t *testing.T) {
	cases := []struct {
		name      string
		script    func(s *speechtest.STT)
		code      string
		retryable bool
	}{
		{"provider cannot be reached", func(s *speechtest.STT) { s.OpenErr = errors.New("dial tcp: timeout") }, CodeSTTFailed, true},
		{"provider not configured", func(s *speechtest.STT) { s.OpenErr = speech.ErrUnavailable }, CodeSTTUnavailable, false},
		{"provider at capacity", func(s *speechtest.STT) { s.OpenErr = speech.ErrBusy }, CodeProviderBusy, true},
		{"provider rate limited", func(s *speechtest.STT) { s.OpenErr = speech.ErrRateLimited }, CodeProviderBusy, true},
		{"stream error after audio", func(s *speechtest.STT) { s.RecvErr = errors.New("connection reset") }, CodeSTTFailed, true},
		{"no final transcript in time", func(s *speechtest.STT) { s.Hang = true }, CodeSTTTimeout, true},
		{"provider ends before the student finishes", func(s *speechtest.STT) { s.EndEarly = true }, CodeSTTFailed, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, withoutTTS())
			h.stt.Set(func(s *speechtest.STT) { s.Finals = []string{"next"} })
			h.stt.Set(tc.script)
			c, _ := h.connect()

			stream := uuid.New()
			c.send(TypeAudioStart, AudioStartPayload{StreamID: stream.String(), Encoding: "pcm_s16le", SampleRate: 16000, Channels: 1})
			// Depending on timing the server may already have failed the stream.
			_ = c.sendBinary(encodeFrame(stream, 0, make([]byte, 640)))
			if !h.stt.EndEarly && h.stt.OpenErr == nil {
				n := uint32(1)
				c.send(TypeAudioEnd, AudioEndPayload{StreamID: stream.String(), Chunks: &n})
			}

			var p ErrorPayload
			for {
				p = c.untilError()
				if p.Code != CodeUnknownStream {
					break
				}
			}
			assert.Equal(t, tc.code, p.Code)
			assert.Equal(t, tc.retryable, p.Retryable)
			assert.Equal(t, stream.String(), p.StreamID)

			// A failed recognition never moves the exam, and leaks nothing.
			assert.Zero(t, h.exam.count("NextQuestion"))
			eventually(t, func() bool { return h.stt.Closed() == h.stt.Opened() }, "recognition stream released")
			assert.Zero(t, h.cls.calls())
		})
	}
}

func TestSTT_ConcurrencyLimit(t *testing.T) {
	inner := &speechtest.STT{}
	limited := speech.LimitSTT(inner, 1)

	first, err := limited.OpenStream(context.Background(), speech.STTConfig{})
	require.NoError(t, err)
	_, err = limited.OpenStream(context.Background(), speech.STTConfig{})
	assert.ErrorIs(t, err, speech.ErrBusy)

	require.NoError(t, first.Close())
	require.NoError(t, first.Close(), "double close must not free the slot twice")
	second, err := limited.OpenStream(context.Background(), speech.STTConfig{})
	require.NoError(t, err)
	_, err = limited.OpenStream(context.Background(), speech.STTConfig{})
	assert.ErrorIs(t, err, speech.ErrBusy)
	require.NoError(t, second.Close())
}

func TestSTT_DisabledProviderRefusesStreams(t *testing.T) {
	h := newHarness(t, withoutTTS())
	h.svc.STT = speech.DisabledSTT()
	c, ready := h.connect()
	assert.False(t, ready.Capabilities.STT)

	c.send(TypeAudioStart, AudioStartPayload{StreamID: uuid.NewString(), Encoding: "pcm_s16le", SampleRate: 16000, Channels: 1})
	assert.Equal(t, CodeSTTUnavailable, c.untilError().Code)
}

func TestIntent_EmptyTranscriptAsksAgain(t *testing.T) {
	h := newHarness(t, withoutTTS())
	h.stt.Set(func(s *speechtest.STT) { s.Finals = nil })
	c, _ := h.connect()

	c.speak(2)
	resp := decode[ResponsePayload](t, c.until(TypeResponse))
	assert.Equal(t, CodeNoSpeech, resp.Code)
	assert.Zero(t, h.cls.calls(), "silence is never sent for interpretation")
}

// calls reports how many times the classifier was invoked.
func (c *fakeClassifier) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.inputs)
}

func (c *fakeClassifier) lastInput() intent.Input {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inputs[len(c.inputs)-1]
}
