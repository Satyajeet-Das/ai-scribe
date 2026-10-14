// Package speechtest provides deterministic, in-memory STT and TTS providers
// for tests. They make no network calls.
package speechtest

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech"
)

// STT is a scripted speech.STTProvider.
type STT struct {
	mu sync.Mutex

	// OpenErr, when set, fails OpenStream.
	OpenErr error
	// Finals are emitted, in order, as final segments after CloseSend.
	Finals []string
	// PartialPerChunk emits one interim event for every chunk received.
	PartialPerChunk bool
	// RecvErr, when set, is returned by Recv after CloseSend instead of results.
	RecvErr error
	// Hang makes the stream never complete after CloseSend.
	Hang bool
	// EndEarly completes the stream as soon as it is opened.
	EndEarly bool
	// SendGate, when non-nil, makes every Send wait for a value, which
	// simulates a recognizer that cannot keep up.
	SendGate chan struct{}

	opened  int
	closed  int
	streams []*STTStream
}

func (s *STT) Name() string { return "fake-stt" }

// Set changes the script under the provider's lock.
func (s *STT) Set(fn func(s *STT)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s)
}

// Opened and Closed report how many streams were opened and closed.
func (s *STT) Opened() int { s.mu.Lock(); defer s.mu.Unlock(); return s.opened }
func (s *STT) Closed() int { s.mu.Lock(); defer s.mu.Unlock(); return s.closed }

// Last returns the most recently opened stream.
func (s *STT) Last() *STTStream {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.streams) == 0 {
		return nil
	}
	return s.streams[len(s.streams)-1]
}

func (s *STT) OpenStream(_ context.Context, cfg speech.STTConfig) (speech.STTStream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.OpenErr != nil {
		return nil, s.OpenErr
	}
	st := &STTStream{
		p:      s,
		Config: cfg,
		events: make(chan speech.TranscriptEvent, 1024),
		done:   make(chan struct{}),
		closed: make(chan struct{}),
		gate:   s.SendGate,
	}
	if s.EndEarly {
		close(st.done)
		st.finished = true
	}
	s.opened++
	s.streams = append(s.streams, st)
	return st, nil
}

// STTStream is one fake recognition stream.
type STTStream struct {
	p      *STT
	Config speech.STTConfig
	gate   chan struct{}

	mu       sync.Mutex
	chunks   int
	bytes    int
	finished bool

	events    chan speech.TranscriptEvent
	done      chan struct{}
	recvErr   error
	closed    chan struct{}
	closeOnce sync.Once
}

// Chunks and Bytes report the audio the stream received.
func (s *STTStream) Chunks() int { s.mu.Lock(); defer s.mu.Unlock(); return s.chunks }
func (s *STTStream) Bytes() int  { s.mu.Lock(); defer s.mu.Unlock(); return s.bytes }

func (s *STTStream) Send(ctx context.Context, audio []byte) error {
	if s.gate != nil {
		select {
		case <-s.gate:
		case <-ctx.Done():
			return ctx.Err()
		case <-s.closed:
			return errors.New("stream closed")
		}
	}
	s.mu.Lock()
	s.chunks++
	s.bytes += len(audio)
	s.mu.Unlock()

	s.p.mu.Lock()
	partial := s.p.PartialPerChunk
	s.p.mu.Unlock()
	if partial {
		select {
		case s.events <- speech.TranscriptEvent{Text: "partial"}:
		default:
		}
	}
	return nil
}

func (s *STTStream) CloseSend(context.Context) error {
	s.p.mu.Lock()
	finals, recvErr, hang := append([]string(nil), s.p.Finals...), s.p.RecvErr, s.p.Hang
	s.p.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished || hang {
		return nil
	}
	s.finished = true
	s.recvErr = recvErr
	if recvErr == nil {
		for _, f := range finals {
			s.events <- speech.TranscriptEvent{Final: true, Text: f, Confidence: 0.99}
		}
	}
	close(s.done)
	return nil
}

func (s *STTStream) Recv(ctx context.Context) (speech.TranscriptEvent, error) {
	// Deliver queued events before reporting completion.
	select {
	case ev := <-s.events:
		return ev, nil
	default:
	}
	select {
	case ev := <-s.events:
		return ev, nil
	case <-s.done:
		select {
		case ev := <-s.events:
			return ev, nil
		default:
		}
		s.mu.Lock()
		err := s.recvErr
		s.mu.Unlock()
		if err != nil {
			return speech.TranscriptEvent{}, err
		}
		return speech.TranscriptEvent{}, io.EOF
	case <-ctx.Done():
		return speech.TranscriptEvent{}, ctx.Err()
	case <-s.closed:
		return speech.TranscriptEvent{}, errors.New("stream closed")
	}
}

func (s *STTStream) Close() error {
	s.closeOnce.Do(func() {
		close(s.closed)
		s.p.mu.Lock()
		s.p.closed++
		s.p.mu.Unlock()
	})
	return nil
}

// TTS is a scripted speech.TTSProvider.
type TTS struct {
	mu sync.Mutex

	// SynthErr, when set, fails Synthesize.
	SynthErr error
	// Chunks is how many chunks each synthesis yields (default 3).
	Chunks int
	// ChunkBytes is the size of each chunk (default 320).
	ChunkBytes int
	// FailAfter, when > 0, fails the stream after that many chunks.
	FailAfter int
	// Gate, when non-nil, makes every Next wait for a value, so a test can
	// release audio one chunk at a time.
	Gate chan struct{}

	texts  []string
	opened int
	closed int
}

func (t *TTS) Name() string { return "fake-tts" }

func (t *TTS) Format() speech.AudioFormat {
	return speech.AudioFormat{Encoding: speech.EncodingPCM16LE, SampleRate: 24000, Channels: 1}
}

// Texts returns everything that was synthesized.
func (t *TTS) Texts() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.texts...)
}

// Opened and Closed report how many syntheses were started and closed.
func (t *TTS) Opened() int { t.mu.Lock(); defer t.mu.Unlock(); return t.opened }
func (t *TTS) Closed() int { t.mu.Lock(); defer t.mu.Unlock(); return t.closed }

func (t *TTS) Synthesize(_ context.Context, req speech.TTSRequest) (speech.TTSStream, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.SynthErr != nil {
		return nil, t.SynthErr
	}
	chunks, size := t.Chunks, t.ChunkBytes
	if chunks <= 0 {
		chunks = 3
	}
	if size <= 0 {
		size = 320
	}
	t.texts = append(t.texts, req.Text)
	t.opened++
	return &ttsStream{p: t, remaining: chunks, size: size, failAfter: t.FailAfter, gate: t.Gate}, nil
}

type ttsStream struct {
	p         *TTS
	remaining int
	sent      int
	size      int
	failAfter int
	gate      chan struct{}
	closeOnce sync.Once
}

func (s *ttsStream) Next(ctx context.Context) ([]byte, error) {
	if s.gate != nil {
		select {
		case <-s.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.failAfter > 0 && s.sent >= s.failAfter {
		return nil, errors.New("synthesis failed")
	}
	if s.remaining == 0 {
		return nil, io.EOF
	}
	s.remaining--
	s.sent++
	chunk := make([]byte, s.size)
	for i := range chunk {
		chunk[i] = byte(s.sent)
	}
	return chunk, nil
}

func (s *ttsStream) Close() error {
	s.closeOnce.Do(func() {
		s.p.mu.Lock()
		s.p.closed++
		s.p.mu.Unlock()
	})
	return nil
}
