package intent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultGroqBaseURL is Groq's OpenAI-compatible API.
const DefaultGroqBaseURL = "https://api.groq.com/openai/v1"

// GroqConfig configures the classifier backed by Groq.
type GroqConfig struct {
	// APIKey is sent as a bearer token. It is never logged or put in an error.
	APIKey string
	// Model is a Groq chat model ID, e.g. llama-3.3-70b-versatile.
	Model string
	// BaseURL overrides the API endpoint; empty means DefaultGroqBaseURL.
	BaseURL string
	// Prompt is the system prompt; PromptVersion labels it in logs.
	Prompt        string
	PromptVersion string
	Timeout       time.Duration
	MaxConcurrent int
	// HTTPClient is optional; tests and callers may supply their own.
	HTTPClient *http.Client
}

// Groq classifies transcripts with a model hosted on Groq.
//
// It asks for JSON object mode, which every Groq model supports, and states
// the schema in the prompt. That is only a request: like every classifier its
// output is untrusted, and is parsed and validated against the intent schema
// and the current question's options before the backend does anything with it.
type Groq struct {
	cfg    GroqConfig
	http   *http.Client
	slots  chan struct{}
	system string
}

func NewGroq(cfg GroqConfig) *Groq {
	g := &Groq{cfg: cfg, http: cfg.HTTPClient}
	if g.http == nil {
		g.http = &http.Client{}
	}
	g.cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if g.cfg.BaseURL == "" {
		g.cfg.BaseURL = DefaultGroqBaseURL
	}
	if cfg.MaxConcurrent > 0 {
		g.slots = make(chan struct{}, cfg.MaxConcurrent)
	}
	// JSON object mode does not constrain the fields, so say what they are.
	schema, _ := json.Marshal(Schema())
	g.system = cfg.Prompt + "\n\nReply with a single JSON object and nothing else. " +
		"It must match this JSON schema exactly: " + string(schema)
	return g
}

func (g *Groq) Name() string { return "groq:" + g.cfg.PromptVersion }

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqChatRequest struct {
	Model       string        `json:"model"`
	Messages    []groqMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	// MaxCompletionTokens leaves room for a model that thinks before it
	// answers; the answer itself is a few dozen tokens.
	MaxCompletionTokens int               `json:"max_completion_tokens"`
	Stream              bool              `json:"stream"`
	ResponseFormat      map[string]string `json:"response_format"`
}

type groqChatResponse struct {
	Choices []struct {
		Message      groqMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
}

func (g *Groq) Classify(ctx context.Context, in Input) (Intent, error) {
	if g.slots != nil {
		select {
		case g.slots <- struct{}{}:
			defer func() { <-g.slots }()
		default:
			return Intent{}, ErrBusy
		}
	}

	options := in.Options
	if options == nil {
		options = []Option{}
	}
	body, err := json.Marshal(request{
		Transcript:           in.Transcript,
		QuestionType:         in.QuestionType,
		Options:              options,
		AwaitingConfirmation: in.AwaitingConfirmation,
	})
	if err != nil {
		return Intent{}, fmt.Errorf("encoding classification request: %w", err)
	}
	payload, err := json.Marshal(groqChatRequest{
		Model: g.cfg.Model,
		Messages: []groqMessage{
			{Role: "system", Content: g.system},
			// The transcript travels as a field of a JSON object, so it is data
			// to classify and never part of the instructions.
			{Role: "user", Content: string(body)},
		},
		Temperature:         0,
		MaxCompletionTokens: 512,
		Stream:              false,
		ResponseFormat:      map[string]string{"type": "json_object"},
	})
	if err != nil {
		return Intent{}, fmt.Errorf("encoding classification request: %w", err)
	}

	if g.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.cfg.Timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return Intent{}, errors.New("intent provider misconfigured")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.cfg.APIKey)

	resp, err := g.http.Do(req)
	if err != nil {
		// Not wrapped: a transport error can embed the URL, and callers only
		// need to know the call failed.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Intent{}, ctxErr
		}
		return Intent{}, errors.New("intent provider unreachable")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// The body is discarded unread: an error body may echo the request.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == http.StatusTooManyRequests {
			return Intent{}, ErrRateLimited
		}
		return Intent{}, fmt.Errorf("intent provider returned status %d", resp.StatusCode)
	}

	var out groqChatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return Intent{}, fmt.Errorf("%w: malformed provider response", ErrInvalidIntent)
	}
	if len(out.Choices) == 0 {
		return Intent{}, fmt.Errorf("%w: provider returned no answer", ErrInvalidIntent)
	}
	// A reply that was cut off is not an interpretation.
	if reason := out.Choices[0].FinishReason; reason != "stop" {
		return Intent{}, fmt.Errorf("intent provider stopped with reason %q", reason)
	}
	return Parse([]byte(out.Choices[0].Message.Content), in)
}
