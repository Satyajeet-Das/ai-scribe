// Package deepgram adapts Deepgram's streaming speech-to-text and
// text-to-speech APIs to the provider-independent speech contracts.
package deepgram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/coder/websocket"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech"
)

const providerName = "deepgram"

// STTConfig configures the streaming recognition client.
type STTConfig struct {
	APIKey string
	// BaseURL is the WebSocket origin, e.g. wss://api.deepgram.com.
	BaseURL string
	Model   string
}

// STT is a speech.STTProvider backed by Deepgram's /v1/listen WebSocket API.
type STT struct {
	cfg STTConfig
}

func NewSTT(cfg STTConfig) *STT {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &STT{cfg: cfg}
}

func (s *STT) Name() string { return providerName }

func (s *STT) OpenStream(ctx context.Context, cfg speech.STTConfig) (speech.STTStream, error) {
	if cfg.Format.Encoding != speech.EncodingPCM16LE {
		return nil, &speech.ProviderError{Provider: providerName, Op: "open", Err: fmt.Errorf("unsupported encoding %q", cfg.Format.Encoding)}
	}

	q := url.Values{}
	q.Set("model", s.cfg.Model)
	q.Set("encoding", "linear16")
	q.Set("sample_rate", strconv.Itoa(cfg.Format.SampleRate))
	q.Set("channels", strconv.Itoa(cfg.Format.Channels))
	q.Set("interim_results", "true")
	q.Set("punctuate", "true")
	if cfg.Language != "" {
		q.Set("language", cfg.Language)
	}

	header := http.Header{}
	header.Set("Authorization", "Token "+s.cfg.APIKey)

	conn, resp, err := websocket.Dial(ctx, s.cfg.BaseURL+"/v1/listen?"+q.Encode(), &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			err = speech.ErrRateLimited
		}
		return nil, &speech.ProviderError{Provider: providerName, Op: "open", Err: err}
	}
	conn.SetReadLimit(1 << 20)
	return &sttStream{conn: conn}, nil
}

type sttStream struct {
	conn *websocket.Conn
}

func (s *sttStream) Send(ctx context.Context, audio []byte) error {
	if err := s.conn.Write(ctx, websocket.MessageBinary, audio); err != nil {
		return &speech.ProviderError{Provider: providerName, Op: "send", Err: err}
	}
	return nil
}

func (s *sttStream) CloseSend(ctx context.Context) error {
	// CloseStream makes Deepgram flush the audio it holds, emit the remaining
	// results and then close the socket.
	if err := s.conn.Write(ctx, websocket.MessageText, []byte(`{"type":"CloseStream"}`)); err != nil {
		return &speech.ProviderError{Provider: providerName, Op: "close_send", Err: err}
	}
	return nil
}

// listenMessage is the subset of Deepgram's streaming response that is used.
type listenMessage struct {
	Type    string `json:"type"`
	IsFinal bool   `json:"is_final"`
	Channel struct {
		Alternatives []struct {
			Transcript string  `json:"transcript"`
			Confidence float64 `json:"confidence"`
		} `json:"alternatives"`
	} `json:"channel"`
	Description string `json:"description"`
}

func (s *sttStream) Recv(ctx context.Context) (speech.TranscriptEvent, error) {
	for {
		typ, data, err := s.conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				return speech.TranscriptEvent{}, io.EOF
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return speech.TranscriptEvent{}, &speech.ProviderError{Provider: providerName, Op: "recv", Err: ctxErr}
			}
			// A dropped connection surfaces as a wrapped io.EOF. It is
			// flattened so that it can never satisfy errors.Is(err, io.EOF),
			// which callers take to mean "transcript complete".
			return speech.TranscriptEvent{}, &speech.ProviderError{Provider: providerName, Op: "recv", Err: errors.New(err.Error())}
		}
		if typ != websocket.MessageText {
			continue
		}

		var msg listenMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return speech.TranscriptEvent{}, &speech.ProviderError{Provider: providerName, Op: "recv", Err: errors.New("malformed provider message")}
		}

		switch msg.Type {
		case "Results":
			if len(msg.Channel.Alternatives) == 0 {
				continue
			}
			alt := msg.Channel.Alternatives[0]
			if strings.TrimSpace(alt.Transcript) == "" {
				continue
			}
			return speech.TranscriptEvent{Final: msg.IsFinal, Text: alt.Transcript, Confidence: alt.Confidence}, nil
		case "Error":
			return speech.TranscriptEvent{}, &speech.ProviderError{Provider: providerName, Op: "recv", Err: errors.New("provider reported an error")}
		default:
			// Metadata, SpeechStarted, UtteranceEnd: not needed.
		}
	}
}

func (s *sttStream) Close() error {
	return s.conn.CloseNow()
}
