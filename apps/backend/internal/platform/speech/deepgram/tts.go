package deepgram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech"
)

// maxSpeakChars keeps each request under Deepgram's per-request text limit.
const maxSpeakChars = 1800

// TTSConfig configures the speech synthesis client.
type TTSConfig struct {
	APIKey string
	// BaseURL is the HTTP origin, e.g. https://api.deepgram.com.
	BaseURL    string
	Model      string
	SampleRate int
	ChunkBytes int
	// HTTPClient is optional; it must not set a total timeout, because the
	// response body is streamed and bounded by the caller's context instead.
	HTTPClient *http.Client
}

// TTS is a speech.TTSProvider backed by Deepgram's /v1/speak HTTP API, which
// streams raw PCM in the response body.
type TTS struct {
	cfg    TTSConfig
	client *http.Client
}

func NewTTS(cfg TTSConfig) *TTS {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.ChunkBytes <= 0 {
		cfg.ChunkBytes = 8 * 1024
	}
	// PCM16 samples are two bytes; never split one across chunks.
	cfg.ChunkBytes -= cfg.ChunkBytes % 2
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	return &TTS{cfg: cfg, client: client}
}

func (t *TTS) Name() string { return providerName }

func (t *TTS) Format() speech.AudioFormat {
	return speech.AudioFormat{Encoding: speech.EncodingPCM16LE, SampleRate: t.cfg.SampleRate, Channels: 1}
}

func (t *TTS) Synthesize(ctx context.Context, req speech.TTSRequest) (speech.TTSStream, error) {
	segments := splitText(req.Text, maxSpeakChars)
	if len(segments) == 0 {
		return nil, &speech.ProviderError{Provider: providerName, Op: "synthesize", Err: errors.New("empty text")}
	}
	s := &ttsStream{tts: t, segments: segments, buf: make([]byte, t.cfg.ChunkBytes)}
	// Open the first segment eagerly so that authentication, quota and
	// connectivity failures surface before any audio is promised.
	if err := s.open(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

type ttsStream struct {
	tts      *TTS
	segments []string
	body     io.ReadCloser
	buf      []byte
	err      error
}

func (s *ttsStream) open(ctx context.Context) error {
	text := s.segments[0]
	s.segments = s.segments[1:]

	q := url.Values{}
	q.Set("model", s.tts.cfg.Model)
	q.Set("encoding", "linear16")
	q.Set("sample_rate", strconv.Itoa(s.tts.cfg.SampleRate))
	q.Set("container", "none")

	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return &speech.ProviderError{Provider: providerName, Op: "synthesize", Err: err}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tts.cfg.BaseURL+"/v1/speak?"+q.Encode(), bytes.NewReader(payload))
	if err != nil {
		return &speech.ProviderError{Provider: providerName, Op: "synthesize", Err: err}
	}
	httpReq.Header.Set("Authorization", "Token "+s.tts.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.tts.client.Do(httpReq)
	if err != nil {
		return &speech.ProviderError{Provider: providerName, Op: "synthesize", Err: err}
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			return &speech.ProviderError{Provider: providerName, Op: "synthesize", Err: speech.ErrRateLimited}
		}
		return &speech.ProviderError{Provider: providerName, Op: "synthesize", Err: fmt.Errorf("unexpected status %d", resp.StatusCode)}
	}
	s.body = resp.Body
	return nil
}

// Next reads the next chunk. The request context passed to Synthesize governs
// the HTTP body, so cancelling it unblocks a pending read.
func (s *ttsStream) Next(ctx context.Context) ([]byte, error) {
	for {
		if s.err != nil {
			return nil, s.err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if s.body == nil {
			if len(s.segments) == 0 {
				return nil, io.EOF
			}
			if err := s.open(ctx); err != nil {
				return nil, err
			}
		}

		n, err := io.ReadFull(s.body, s.buf)
		if err != nil {
			_ = s.body.Close()
			s.body = nil
			// A short or empty read is the normal end of a segment; anything
			// else is reported once the bytes already received are delivered.
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				s.err = &speech.ProviderError{Provider: providerName, Op: "read", Err: err}
			}
		}
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, s.buf[:n])
			return chunk, nil
		}
	}
}

func (s *ttsStream) Close() error {
	s.segments = nil
	if s.body != nil {
		err := s.body.Close()
		s.body = nil
		return err
	}
	return nil
}

// splitText breaks text into segments of at most max characters, preferring
// sentence boundaries and falling back to word boundaries.
func splitText(text string, max int) []string {
	text = strings.TrimSpace(text)
	var out []string
	for len(text) > max {
		// end is exclusive: the segment is text[:end].
		end := strings.LastIndexAny(text[:max], ".?!\n") + 1
		if end < max/2 {
			end = strings.LastIndexByte(text[:max], ' ') + 1
		}
		if end <= 0 {
			end = max
			// Do not split a multi-byte rune.
			for end > 1 && text[end]&0xC0 == 0x80 {
				end--
			}
		}
		out = append(out, strings.TrimSpace(text[:end]))
		text = strings.TrimSpace(text[end:])
	}
	if text != "" {
		out = append(out, text)
	}
	return out
}
