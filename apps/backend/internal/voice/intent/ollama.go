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

// OllamaConfig configures the classifier backed by a local Ollama server.
type OllamaConfig struct {
	// BaseURL is the Ollama server, e.g. http://localhost:11434.
	BaseURL string
	Model   string
	// Prompt is the system prompt; PromptVersion labels it in logs.
	Prompt        string
	PromptVersion string
	Timeout       time.Duration
	MaxConcurrent int
	// HTTPClient is optional; tests and callers may supply their own.
	HTTPClient *http.Client
}

// Ollama classifies transcripts with a local model constrained to the intent
// JSON schema. Like every classifier it only interprets: its output is parsed
// and validated by the backend before anything is done with it.
type Ollama struct {
	cfg   OllamaConfig
	http  *http.Client
	slots chan struct{}
}

func NewOllama(cfg OllamaConfig) *Ollama {
	o := &Ollama{cfg: cfg, http: cfg.HTTPClient}
	if o.http == nil {
		o.http = &http.Client{}
	}
	o.cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.MaxConcurrent > 0 {
		o.slots = make(chan struct{}, cfg.MaxConcurrent)
	}
	return o
}

func (o *Ollama) Name() string { return "ollama:" + o.cfg.PromptVersion }

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   map[string]any  `json:"format"`
	Options  map[string]any  `json:"options"`
}

type ollamaChatResponse struct {
	Message    ollamaMessage `json:"message"`
	Done       bool          `json:"done"`
	DoneReason string        `json:"done_reason"`
}

func (o *Ollama) Classify(ctx context.Context, in Input) (Intent, error) {
	if o.slots != nil {
		select {
		case o.slots <- struct{}{}:
			defer func() { <-o.slots }()
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
	payload, err := json.Marshal(ollamaChatRequest{
		Model: o.cfg.Model,
		Messages: []ollamaMessage{
			{Role: "system", Content: o.cfg.Prompt},
			{Role: "user", Content: string(body)},
		},
		Stream:  false,
		Format:  Schema(),
		Options: map[string]any{"temperature": 0},
	})
	if err != nil {
		return Intent{}, fmt.Errorf("encoding classification request: %w", err)
	}

	if o.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.cfg.Timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.cfg.BaseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return Intent{}, errors.New("intent provider misconfigured")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.http.Do(req)
	if err != nil {
		// The error is not wrapped: it can embed the URL but never the
		// transcript, and callers only need to know the call failed.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Intent{}, ctxErr
		}
		return Intent{}, errors.New("intent provider unreachable")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == http.StatusTooManyRequests {
			return Intent{}, ErrRateLimited
		}
		return Intent{}, fmt.Errorf("intent provider returned status %d", resp.StatusCode)
	}

	var out ollamaChatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return Intent{}, fmt.Errorf("%w: malformed provider response", ErrInvalidIntent)
	}
	// A truncated reply is not an interpretation.
	if !out.Done || (out.DoneReason != "" && out.DoneReason != "stop") {
		return Intent{}, fmt.Errorf("intent provider stopped with reason %q", out.DoneReason)
	}
	return Parse([]byte(out.Message.Content), in)
}
