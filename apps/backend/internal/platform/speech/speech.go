// Package speech defines provider-independent contracts for streaming speech
// recognition (STT) and speech synthesis (TTS). Concrete providers live in
// sub-packages and are selected by configuration.
package speech

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// EncodingPCM16LE is signed 16-bit little-endian linear PCM, the only audio
// encoding the pipeline carries.
const EncodingPCM16LE = "pcm_s16le"

// AudioFormat describes raw audio on the wire.
type AudioFormat struct {
	Encoding   string `json:"encoding"`
	SampleRate int    `json:"sampleRate"`
	Channels   int    `json:"channels"`
}

var (
	// ErrUnavailable means no provider is configured for the capability.
	ErrUnavailable = errors.New("speech: provider unavailable")
	// ErrBusy means the provider's concurrency limit is exhausted.
	ErrBusy = errors.New("speech: provider at capacity")
	// ErrRateLimited means the upstream provider rejected the call for quota.
	ErrRateLimited = errors.New("speech: provider rate limited")
)

// ProviderError wraps an upstream failure with the provider and operation.
// Its message never contains audio or transcript content.
type ProviderError struct {
	Provider string
	Op       string
	Err      error
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("%s %s: %v", e.Provider, e.Op, e.Err)
}

func (e *ProviderError) Unwrap() error { return e.Err }

// TranscriptEvent is one recognition result. A non-final event is an interim
// hypothesis for the current segment and may be revised; a final event is a
// settled segment.
type TranscriptEvent struct {
	Final      bool
	Text       string
	Confidence float64
}

// STTConfig describes the audio an STT stream will receive.
type STTConfig struct {
	Format   AudioFormat
	Language string
}

// STTStream is one live recognition stream. Send and CloseSend may be called
// from one goroutine while another calls Recv. Close may be called at any time,
// from any goroutine, and unblocks both.
type STTStream interface {
	// Send forwards one audio chunk.
	Send(ctx context.Context, audio []byte) error
	// CloseSend signals the end of audio; results keep arriving until Recv
	// returns io.EOF.
	CloseSend(ctx context.Context) error
	// Recv returns the next event, or io.EOF once the stream has completed.
	Recv(ctx context.Context) (TranscriptEvent, error)
	// Close aborts the stream and releases its resources. Idempotent.
	Close() error
}

// STTProvider opens recognition streams.
type STTProvider interface {
	Name() string
	OpenStream(ctx context.Context, cfg STTConfig) (STTStream, error)
}

// TTSRequest is text to be spoken.
type TTSRequest struct {
	Text string
}

// TTSStream yields synthesized audio in chunks.
type TTSStream interface {
	// Next returns the next chunk, or io.EOF when synthesis is complete.
	Next(ctx context.Context) ([]byte, error)
	// Close aborts synthesis and releases its resources. Idempotent.
	Close() error
}

// TTSProvider synthesizes speech.
type TTSProvider interface {
	Name() string
	// Format is the format of every chunk the provider emits.
	Format() AudioFormat
	Synthesize(ctx context.Context, req TTSRequest) (TTSStream, error)
}

// ---------------------------------------------------------------------------
// Disabled providers
// ---------------------------------------------------------------------------

type disabledSTT struct{}

// DisabledSTT returns a provider that refuses every stream with ErrUnavailable.
func DisabledSTT() STTProvider { return disabledSTT{} }

func (disabledSTT) Name() string { return "disabled" }
func (disabledSTT) OpenStream(context.Context, STTConfig) (STTStream, error) {
	return nil, ErrUnavailable
}

type disabledTTS struct{}

// DisabledTTS returns a provider that refuses every request with ErrUnavailable.
func DisabledTTS() TTSProvider { return disabledTTS{} }

func (disabledTTS) Name() string        { return "disabled" }
func (disabledTTS) Format() AudioFormat { return AudioFormat{} }
func (disabledTTS) Synthesize(context.Context, TTSRequest) (TTSStream, error) {
	return nil, ErrUnavailable
}

// ---------------------------------------------------------------------------
// Concurrency limits
// ---------------------------------------------------------------------------

type slots chan struct{}

func (s slots) tryAcquire() bool {
	select {
	case s <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s slots) release() { <-s }

type limitedSTT struct {
	STTProvider
	slots slots
}

// LimitSTT caps the number of concurrently open streams. When the cap is
// reached OpenStream fails fast with ErrBusy instead of queueing, so a
// saturated provider never stalls a student's turn.
func LimitSTT(p STTProvider, max int) STTProvider {
	if max <= 0 {
		return p
	}
	return &limitedSTT{STTProvider: p, slots: make(slots, max)}
}

func (l *limitedSTT) OpenStream(ctx context.Context, cfg STTConfig) (STTStream, error) {
	if !l.slots.tryAcquire() {
		return nil, ErrBusy
	}
	s, err := l.STTProvider.OpenStream(ctx, cfg)
	if err != nil {
		l.slots.release()
		return nil, err
	}
	return &limitedSTTStream{STTStream: s, release: l.slots.release}, nil
}

type limitedSTTStream struct {
	STTStream
	once    sync.Once
	release func()
}

func (s *limitedSTTStream) Close() error {
	err := s.STTStream.Close()
	s.once.Do(s.release)
	return err
}

type limitedTTS struct {
	TTSProvider
	slots slots
}

// LimitTTS caps the number of concurrent syntheses; see LimitSTT.
func LimitTTS(p TTSProvider, max int) TTSProvider {
	if max <= 0 {
		return p
	}
	return &limitedTTS{TTSProvider: p, slots: make(slots, max)}
}

func (l *limitedTTS) Synthesize(ctx context.Context, req TTSRequest) (TTSStream, error) {
	if !l.slots.tryAcquire() {
		return nil, ErrBusy
	}
	s, err := l.TTSProvider.Synthesize(ctx, req)
	if err != nil {
		l.slots.release()
		return nil, err
	}
	return &limitedTTSStream{TTSStream: s, release: l.slots.release}, nil
}

type limitedTTSStream struct {
	TTSStream
	once    sync.Once
	release func()
}

func (s *limitedTTSStream) Close() error {
	err := s.TTSStream.Close()
	s.once.Do(s.release)
	return err
}
