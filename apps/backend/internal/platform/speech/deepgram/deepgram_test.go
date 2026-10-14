package deepgram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech"
)

// These tests run the adapters against local servers that speak Deepgram's
// documented wire format. They verify the adapters' protocol handling; they
// do not verify the real service.

var pcm16k = speech.STTConfig{
	Format:   speech.AudioFormat{Encoding: speech.EncodingPCM16LE, SampleRate: 16000, Channels: 1},
	Language: "en",
}

// fakeListen is a stand-in for wss://api.deepgram.com/v1/listen.
func fakeListen(t *testing.T, script func(ctx context.Context, c *websocket.Conn, r *http.Request)) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token secret-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		script(r.Context(), c, r)
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func results(text string, final bool) []byte {
	out, _ := json.Marshal(map[string]any{
		"type": "Results", "is_final": final,
		"channel": map[string]any{"alternatives": []map[string]any{{"transcript": text, "confidence": 0.98}}},
	})
	return out
}

func TestSTT_StreamsAudioAndReturnsPartialAndFinalResults(t *testing.T) {
	var audioBytes atomic.Int64
	var query atomic.Value
	url := fakeListen(t, func(ctx context.Context, c *websocket.Conn, r *http.Request) {
		query.Store(r.URL.RawQuery)
		for {
			typ, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			if typ == websocket.MessageBinary {
				audioBytes.Add(int64(len(data)))
				_ = c.Write(ctx, websocket.MessageText, results("option", false))
				continue
			}
			var ctl struct{ Type string }
			_ = json.Unmarshal(data, &ctl)
			if ctl.Type == "CloseStream" {
				_ = c.Write(ctx, websocket.MessageText, results("", true)) // empty results are skipped
				_ = c.Write(ctx, websocket.MessageText, results("option B", true))
				_ = c.Write(ctx, websocket.MessageText, []byte(`{"type":"Metadata","request_id":"x"}`))
				_ = c.Close(websocket.StatusNormalClosure, "")
				return
			}
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stt := NewSTT(STTConfig{APIKey: "secret-key", BaseURL: url + "/", Model: "nova-3"})
	assert.Equal(t, "deepgram", stt.Name())

	stream, err := stt.OpenStream(ctx, pcm16k)
	require.NoError(t, err)
	defer stream.Close()

	require.NoError(t, stream.Send(ctx, make([]byte, 640)))
	ev, err := stream.Recv(ctx)
	require.NoError(t, err)
	assert.Equal(t, speech.TranscriptEvent{Final: false, Text: "option", Confidence: 0.98}, ev)

	require.NoError(t, stream.Send(ctx, make([]byte, 640)))
	require.NoError(t, stream.CloseSend(ctx))

	var finals []string
	for {
		ev, err := stream.Recv(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if ev.Final {
			finals = append(finals, ev.Text)
		}
	}
	assert.Equal(t, []string{"option B"}, finals)
	assert.EqualValues(t, 1280, audioBytes.Load())

	q := query.Load().(string)
	for _, want := range []string{"model=nova-3", "encoding=linear16", "sample_rate=16000", "channels=1", "interim_results=true", "language=en"} {
		assert.Contains(t, q, want)
	}
}

func TestSTT_Failures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Run("rejected credentials", func(t *testing.T) {
		url := fakeListen(t, func(context.Context, *websocket.Conn, *http.Request) {})
		_, err := NewSTT(STTConfig{APIKey: "wrong", BaseURL: url}).OpenStream(ctx, pcm16k)
		var pe *speech.ProviderError
		require.ErrorAs(t, err, &pe)
		assert.Equal(t, "open", pe.Op)
		assert.NotContains(t, err.Error(), "wrong", "the API key never appears in errors")
	})
	t.Run("rate limited", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer srv.Close()
		_, err := NewSTT(STTConfig{APIKey: "k", BaseURL: "ws" + strings.TrimPrefix(srv.URL, "http")}).OpenStream(ctx, pcm16k)
		assert.ErrorIs(t, err, speech.ErrRateLimited)
	})
	t.Run("unsupported encoding", func(t *testing.T) {
		_, err := NewSTT(STTConfig{APIKey: "k", BaseURL: "ws://127.0.0.1:1"}).OpenStream(ctx, speech.STTConfig{Format: speech.AudioFormat{Encoding: "opus"}})
		assert.Error(t, err)
	})
	t.Run("provider error message", func(t *testing.T) {
		url := fakeListen(t, func(ctx context.Context, c *websocket.Conn, _ *http.Request) {
			_ = c.Write(ctx, websocket.MessageText, []byte(`{"type":"Error","description":"bad audio"}`))
			<-ctx.Done()
		})
		stream, err := NewSTT(STTConfig{APIKey: "secret-key", BaseURL: url}).OpenStream(ctx, pcm16k)
		require.NoError(t, err)
		defer stream.Close()
		_, err = stream.Recv(ctx)
		assert.Error(t, err)
		assert.NotErrorIs(t, err, io.EOF)
	})
	t.Run("interrupted stream is an error, not a clean end", func(t *testing.T) {
		url := fakeListen(t, func(ctx context.Context, c *websocket.Conn, _ *http.Request) {
			_, _, _ = c.Read(ctx)
			_ = c.CloseNow()
		})
		stream, err := NewSTT(STTConfig{APIKey: "secret-key", BaseURL: url}).OpenStream(ctx, pcm16k)
		require.NoError(t, err)
		defer stream.Close()
		require.NoError(t, stream.Send(ctx, make([]byte, 64)))
		_, err = stream.Recv(ctx)
		require.Error(t, err)
		assert.NotErrorIs(t, err, io.EOF, "a dropped connection must not look like a complete transcript")
	})
	t.Run("cancellation unblocks Recv", func(t *testing.T) {
		url := fakeListen(t, func(ctx context.Context, c *websocket.Conn, _ *http.Request) {
			_, _, _ = c.Read(ctx)
		})
		stream, err := NewSTT(STTConfig{APIKey: "secret-key", BaseURL: url}).OpenStream(ctx, pcm16k)
		require.NoError(t, err)
		defer stream.Close()

		recvCtx, cancelRecv := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancelRecv()
		start := time.Now()
		_, err = stream.Recv(recvCtx)
		require.Error(t, err)
		assert.Less(t, time.Since(start), 2*time.Second)
	})
}

// fakeSpeak is a stand-in for https://api.deepgram.com/v1/speak.
func fakeSpeak(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

func drain(ctx context.Context, s speech.TTSStream) ([][]byte, error) {
	var chunks [][]byte
	for {
		chunk, err := s.Next(ctx)
		if errors.Is(err, io.EOF) {
			return chunks, nil
		}
		if err != nil {
			return chunks, err
		}
		chunks = append(chunks, chunk)
	}
}

func TestTTS_StreamsAudioInSampleAlignedChunks(t *testing.T) {
	var texts []string
	var query string
	url := fakeSpeak(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Token secret-key", r.Header.Get("Authorization"))
		assert.Equal(t, "/v1/speak", r.URL.Path)
		query = r.URL.RawQuery
		var body struct{ Text string }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		texts = append(texts, body.Text)
		// 2500 bytes of audio, flushed in uneven pieces.
		for _, n := range []int{700, 1, 1299, 500} {
			_, _ = w.Write(make([]byte, n))
			w.(http.Flusher).Flush()
		}
	})

	tts := NewTTS(TTSConfig{APIKey: "secret-key", BaseURL: url, Model: "aura-2-thalia-en", SampleRate: 24000, ChunkBytes: 1001})
	assert.Equal(t, speech.AudioFormat{Encoding: speech.EncodingPCM16LE, SampleRate: 24000, Channels: 1}, tts.Format())

	ctx := context.Background()
	stream, err := tts.Synthesize(ctx, speech.TTSRequest{Text: "Question 1 of 3."})
	require.NoError(t, err)
	chunks, err := drain(ctx, stream)
	require.NoError(t, err)
	require.NoError(t, stream.Close())

	total := 0
	for _, c := range chunks {
		assert.LessOrEqual(t, len(c), 1000)
		assert.Zero(t, len(c)%2, "a 16-bit sample is never split across chunks")
		total += len(c)
	}
	assert.Equal(t, 2500, total)
	assert.Equal(t, []string{"Question 1 of 3."}, texts)
	for _, want := range []string{"model=aura-2-thalia-en", "encoding=linear16", "sample_rate=24000", "container=none"} {
		assert.Contains(t, query, want)
	}
}

func TestTTS_LongTextIsSplitIntoSequentialRequests(t *testing.T) {
	var requests atomic.Int32
	url := fakeSpeak(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Text string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		assert.LessOrEqual(t, len(body.Text), maxSpeakChars)
		requests.Add(1)
		_, _ = w.Write(make([]byte, 100))
	})
	long := strings.Repeat("This is one sentence of the question. ", 120)

	ctx := context.Background()
	stream, err := NewTTS(TTSConfig{APIKey: "k", BaseURL: url, SampleRate: 24000}).Synthesize(ctx, speech.TTSRequest{Text: long})
	require.NoError(t, err)
	chunks, err := drain(ctx, stream)
	require.NoError(t, err)

	assert.EqualValues(t, 3, requests.Load())
	assert.Len(t, chunks, 3)
}

func TestTTS_Failures(t *testing.T) {
	ctx := context.Background()

	t.Run("rate limited", func(t *testing.T) {
		url := fakeSpeak(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) })
		_, err := NewTTS(TTSConfig{APIKey: "k", BaseURL: url}).Synthesize(ctx, speech.TTSRequest{Text: "hi"})
		assert.ErrorIs(t, err, speech.ErrRateLimited)
	})
	t.Run("server error", func(t *testing.T) {
		url := fakeSpeak(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, "upstream exploded: secret detail")
		})
		_, err := NewTTS(TTSConfig{APIKey: "k", BaseURL: url}).Synthesize(ctx, speech.TTSRequest{Text: "hi"})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "secret detail")
	})
	t.Run("empty text", func(t *testing.T) {
		_, err := NewTTS(TTSConfig{APIKey: "k", BaseURL: "http://127.0.0.1:1"}).Synthesize(ctx, speech.TTSRequest{Text: "   "})
		assert.Error(t, err)
	})
	t.Run("cancellation stops a stalled stream", func(t *testing.T) {
		release := make(chan struct{})
		url := fakeSpeak(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(make([]byte, 64))
			w.(http.Flusher).Flush()
			select {
			case <-release:
			case <-r.Context().Done():
			}
		})
		defer close(release)

		cctx, cancel := context.WithCancel(ctx)
		stream, err := NewTTS(TTSConfig{APIKey: "k", BaseURL: url, ChunkBytes: 64}).Synthesize(cctx, speech.TTSRequest{Text: "hi"})
		require.NoError(t, err)
		defer stream.Close()

		chunk, err := stream.Next(cctx)
		require.NoError(t, err)
		assert.Len(t, chunk, 64)

		time.AfterFunc(50*time.Millisecond, cancel)
		start := time.Now()
		_, err = stream.Next(cctx)
		require.Error(t, err)
		assert.Less(t, time.Since(start), 2*time.Second)
	})
}

func TestSplitText(t *testing.T) {
	assert.Empty(t, splitText("   ", 10))
	assert.Equal(t, []string{"short"}, splitText("short", 10))

	parts := splitText("One two three. Four five six. Seven eight nine.", 20)
	assert.Equal(t, []string{"One two three.", "Four five six.", "Seven eight nine."}, parts)

	// No sentence or word boundary, with multi-byte characters.
	parts = splitText(strings.Repeat("é", 30), 21)
	assert.Equal(t, strings.Repeat("é", 30), strings.Join(parts, ""))
	for _, p := range parts {
		assert.LessOrEqual(t, len(p), 21)
		assert.True(t, strings.ToValidUTF8(p, "?") == p, "a rune is never split")
	}
}
