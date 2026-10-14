package intent

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

//go:embed prompts/*.md
var promptFS embed.FS

// ErrRateLimited means the model provider rejected the call for quota.
var ErrRateLimited = errors.New("intent: provider rate limited")

// ErrBusy means the classifier's own concurrency limit is exhausted.
var ErrBusy = errors.New("intent: classifier at capacity")

// LoadPrompt returns the classification prompt. A file path, when given,
// overrides the prompt embedded under the given version.
func LoadPrompt(version, file string) (string, error) {
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("reading intent prompt file: %w", err)
		}
		if strings.TrimSpace(string(data)) == "" {
			return "", errors.New("intent prompt file is empty")
		}
		return string(data), nil
	}
	data, err := promptFS.ReadFile("prompts/" + version + ".md")
	if err != nil {
		return "", fmt.Errorf("unknown intent prompt version %q", version)
	}
	return string(data), nil
}

// AnthropicConfig configures the Claude-backed classifier.
type AnthropicConfig struct {
	APIKey string
	Model  string
	// Prompt is the system prompt; PromptVersion labels it in logs.
	Prompt        string
	PromptVersion string
	Timeout       time.Duration
	MaxConcurrent int
	// BaseURL overrides the API endpoint; used by tests.
	BaseURL string
}

// Anthropic classifies transcripts with a Claude model constrained to the
// intent JSON schema. The model is given the transcript and the option keys
// and texts the student already sees; nothing else about the exam.
type Anthropic struct {
	client anthropic.Client
	cfg    AnthropicConfig
	slots  chan struct{}
}

func NewAnthropic(cfg AnthropicConfig) *Anthropic {
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		// Classification is a pure read, so one retry on a transient failure
		// is safe; the caller's context still bounds the total time.
		option.WithMaxRetries(1),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	a := &Anthropic{client: anthropic.NewClient(opts...), cfg: cfg}
	if cfg.MaxConcurrent > 0 {
		a.slots = make(chan struct{}, cfg.MaxConcurrent)
	}
	return a
}

func (a *Anthropic) Name() string { return "anthropic:" + a.cfg.PromptVersion }

// request is the JSON document the model classifies.
type request struct {
	Transcript           string   `json:"transcript"`
	QuestionType         string   `json:"question_type"`
	Options              []Option `json:"options"`
	AwaitingConfirmation bool     `json:"awaiting_confirmation"`
}

func (a *Anthropic) Classify(ctx context.Context, in Input) (Intent, error) {
	if a.slots != nil {
		select {
		case a.slots <- struct{}{}:
			defer func() { <-a.slots }()
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

	if a.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.cfg.Timeout)
		defer cancel()
	}

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model: anthropic.Model(a.cfg.Model),
		// Thinking tokens count against this limit; the answer itself is tiny.
		MaxTokens: 2048,
		System:    []anthropic.TextBlockParam{{Text: a.cfg.Prompt}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(string(body))),
		},
		OutputConfig: anthropic.OutputConfigParam{
			// A short, well-specified labelling task on a latency-sensitive path.
			Effort: anthropic.OutputConfigEffortLow,
			Format: anthropic.JSONOutputFormatParam{Schema: Schema()},
		},
	})
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			switch apiErr.StatusCode {
			case 429:
				return Intent{}, ErrRateLimited
			default:
				return Intent{}, fmt.Errorf("intent provider returned status %d", apiErr.StatusCode)
			}
		}
		// Transport failure or timeout. The error is wrapped without the
		// request body so no transcript reaches the logs.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Intent{}, ctxErr
		}
		return Intent{}, errors.New("intent provider unreachable")
	}

	// A refusal or a truncated reply is not an interpretation; surface it so
	// the caller takes the safe recovery path instead of acting on a guess.
	if resp.StopReason != anthropic.StopReasonEndTurn {
		return Intent{}, fmt.Errorf("intent provider stopped with reason %q", resp.StopReason)
	}

	var text strings.Builder
	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(tb.Text)
		}
	}
	return Parse([]byte(text.String()), in)
}
