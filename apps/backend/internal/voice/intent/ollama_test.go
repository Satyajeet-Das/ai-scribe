package intent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ollamaServer(t *testing.T, status int, content string, doneReason string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/chat", r.URL.Path)
		var req ollamaChatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.False(t, req.Stream)
		assert.NotEmpty(t, req.Format)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(ollamaChatResponse{
			Message: ollamaMessage{Role: "assistant", Content: content}, Done: true, DoneReason: doneReason,
		})
	}))
}

func TestOllamaClassify(t *testing.T) {
	in := Input{Transcript: "pick the second one", QuestionType: "MCQ", Options: []Option{{Key: "A", Text: "x"}, {Key: "B", Text: "y"}}}

	t.Run("valid", func(t *testing.T) {
		srv := ollamaServer(t, 200, `{"intent":"select_option","option_key":"b","confidence":0.9}`, "stop")
		defer srv.Close()
		got, err := NewOllama(OllamaConfig{BaseURL: srv.URL + "/", Model: "m"}).Classify(context.Background(), in)
		require.NoError(t, err)
		assert.Equal(t, SelectOption, got.Name)
		assert.Equal(t, "B", got.OptionKey)
	})
	t.Run("option not offered is rejected", func(t *testing.T) {
		srv := ollamaServer(t, 200, `{"intent":"select_option","option_key":"Z","confidence":0.9}`, "stop")
		defer srv.Close()
		_, err := NewOllama(OllamaConfig{BaseURL: srv.URL, Model: "m"}).Classify(context.Background(), in)
		assert.ErrorIs(t, err, ErrInvalidIntent)
	})
	t.Run("truncated reply", func(t *testing.T) {
		srv := ollamaServer(t, 200, `{"intent":"unk`, "length")
		defer srv.Close()
		_, err := NewOllama(OllamaConfig{BaseURL: srv.URL, Model: "m"}).Classify(context.Background(), in)
		assert.Error(t, err)
	})
	t.Run("server error", func(t *testing.T) {
		srv := ollamaServer(t, 500, "", "")
		defer srv.Close()
		_, err := NewOllama(OllamaConfig{BaseURL: srv.URL, Model: "m"}).Classify(context.Background(), in)
		assert.Error(t, err)
	})
	t.Run("unreachable", func(t *testing.T) {
		_, err := NewOllama(OllamaConfig{BaseURL: "http://127.0.0.1:1", Model: "m"}).Classify(context.Background(), in)
		assert.EqualError(t, err, "intent provider unreachable")
	})
}
