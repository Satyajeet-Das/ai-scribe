// Package groq is the extract.Model backed by models hosted on Groq's
// OpenAI-compatible API. Groq cannot read a PDF, so it is sent page text (and,
// for pictures, page images) that the PDF layer has already pulled out.
//
// Everything the model returns is untrusted text: this package only moves it,
// and extract validates it. Credentials, page text and replies are never put in
// logs or errors; a failure is described by a status code and, at most, one
// short machine-readable code from the provider.
package groq

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
)

// DefaultBaseURL is Groq's OpenAI-compatible API.
const DefaultBaseURL = "https://api.groq.com/openai/v1"

// maxCompletionTokens leaves room for a reasoning model to think before it
// writes a long list of questions. A reply that is still cut off is an error,
// never a shorter list.
const maxCompletionTokens = 16384

const maxResponseBytes = 4 << 20

// Config configures the model.
type Config struct {
	// APIKey is sent as a bearer token. It is never logged or put in an error.
	APIKey string
	// BaseURL overrides the API endpoint; empty means DefaultBaseURL.
	BaseURL string
	// TextModel reads pages that have a text layer.
	TextModel string
	// VisionModel reads picture pages. Empty means pictures are not read.
	VisionModel   string
	Prompt        string
	PromptVersion string
	Timeout       time.Duration
	// HTTPClient is optional; tests may supply their own.
	HTTPClient *http.Client
}

// Model implements extract.Model.
type Model struct {
	cfg    Config
	http   *http.Client
	system string
}

func New(cfg Config) *Model {
	m := &Model{cfg: cfg, http: cfg.HTTPClient}
	if m.http == nil {
		m.http = &http.Client{}
	}
	m.cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if m.cfg.BaseURL == "" {
		m.cfg.BaseURL = DefaultBaseURL
	}
	m.system = cfg.Prompt
	return m
}

func (m *Model) Name() string {
	if m.cfg.VisionModel == "" {
		return "groq:" + m.cfg.TextModel
	}
	return "groq:" + m.cfg.TextModel + "+" + m.cfg.VisionModel
}

func (m *Model) PromptVersion() string { return m.cfg.PromptVersion }

func (m *Model) CanReadImages() bool { return m.cfg.VisionModel != "" }

type message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type imageURL struct {
	URL string `json:"url"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type chatRequest struct {
	Model               string         `json:"model"`
	Messages            []message      `json:"messages"`
	Temperature         float64        `json:"temperature"`
	MaxCompletionTokens int            `json:"max_completion_tokens"`
	Stream              bool           `json:"stream"`
	ResponseFormat      map[string]any `json:"response_format"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// pagesPayload is what the model is shown. Page text travels as a field of a
// JSON object, so it is data to read and never part of the instructions.
type pagesPayload struct {
	Pages []pagePayload `json:"pages"`
}

type pagePayload struct {
	Page int    `json:"page"`
	Text string `json:"text"`
}

func (m *Model) Complete(ctx context.Context, req extract.Request) (string, error) {
	if len(req.Pages) == 0 {
		return "", extract.NewError(extract.CodeInternal, false, "empty batch")
	}
	payload := pagesPayload{}
	for _, p := range req.Pages {
		payload.Pages = append(payload.Pages, pagePayload{Page: p.Number, Text: p.Text})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", extract.NewError(extract.CodeInternal, false, "encoding request")
	}

	model := m.cfg.TextModel
	var user any = string(body)
	if req.Vision {
		if m.cfg.VisionModel == "" {
			return "", extract.NewError(extract.CodeRejected, false, "no vision model is configured")
		}
		model = m.cfg.VisionModel
		parts := []contentPart{{Type: "text", Text: string(body)}}
		for _, p := range req.Pages {
			for _, im := range p.Images {
				mime := "image/png"
				if im.Format == "jpg" {
					mime = "image/jpeg"
				}
				parts = append(parts, contentPart{
					Type:     "image_url",
					ImageURL: &imageURL{URL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(im.Data)},
				})
			}
		}
		user = parts
	}

	rf := map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "exam_extraction",
			"strict": true,
			"schema": extract.Schema(),
		},
	}
	reqBody, err := json.Marshal(chatRequest{
		Model: model,
		Messages: []message{
			{Role: "system", Content: m.system},
			{Role: "user", Content: user},
		},
		Temperature:         0,
		MaxCompletionTokens: maxCompletionTokens,
		ResponseFormat:      rf,
	})
	if err != nil {
		return "", extract.NewError(extract.CodeInternal, false, "encoding request")
	}

	parent := ctx
	if m.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.cfg.Timeout)
		defer cancel()
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.BaseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", extract.NewError(extract.CodeInternal, false, "provider misconfigured")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.cfg.APIKey)

	resp, err := m.http.Do(httpReq)
	if err != nil {
		// Not wrapped: a transport error can embed the URL.
		if perr := parent.Err(); perr != nil {
			return "", perr
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", extract.NewError(extract.CodeTimeout, true, "the provider did not answer in time")
		}
		return "", extract.NewError(extract.CodeUnavailable, true, "the provider could not be reached")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		code := providerErrorCode(io.LimitReader(resp.Body, 4096))
		return "", statusError(resp.StatusCode, code)
	}

	var out chatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&out); err != nil {
		if perr := parent.Err(); perr != nil {
			return "", perr
		}
		return "", extract.NewError(extract.CodeInvalidOutput, false, "malformed provider response")
	}
	if len(out.Choices) == 0 {
		return "", extract.NewError(extract.CodeInvalidOutput, false, "the provider returned no answer")
	}
	switch reason := out.Choices[0].FinishReason; reason {
	case "stop":
	case "length":
		return "", extract.NewError(extract.CodeTruncated, false, "the reply was cut off; the pages may hold too many questions for one request")
	default:
		return "", extract.NewError(extract.CodeRejected, false, "the provider stopped with reason %q", safeToken(reason))
	}
	return out.Choices[0].Message.Content, nil
}

var tokenPattern = regexp.MustCompile(`^[a-z0-9_.-]{1,64}$`)

func safeToken(s string) string {
	if tokenPattern.MatchString(s) {
		return s
	}
	return ""
}

// providerErrorCode reads the machine-readable code from an OpenAI-style error
// body ({"error":{"code":"model_not_found","type":"..."}}). It returns "" when
// there is none or it is not a plain token: the rest of an error body may echo
// the request.
func providerErrorCode(body io.Reader) string {
	var out struct {
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	if json.NewDecoder(body).Decode(&out) != nil {
		return ""
	}
	for _, c := range []string{out.Error.Code, out.Error.Type} {
		if tokenPattern.MatchString(c) {
			return c
		}
	}
	return ""
}

func statusError(status int, providerCode string) error {
	suffix := ""
	if providerCode != "" {
		suffix = " (" + providerCode + ")"
	}
	switch {
	case status == http.StatusTooManyRequests:
		return extract.NewError(extract.CodeRateLimited, true, "the provider is rate limiting requests")
	case status == http.StatusRequestTimeout, status == http.StatusGatewayTimeout:
		return extract.NewError(extract.CodeTimeout, true, "the provider timed out (status %d)", status)
	case status >= 500:
		return extract.NewError(extract.CodeUnavailable, true, "the provider is unavailable (status %d)", status)
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return extract.NewError(extract.CodeAuth, false, "the provider refused the credentials (status %d)", status)
	case providerCode == "model_not_found":
		return extract.NewError(extract.CodeModelNotFound, false, "the configured model is not available from this provider; it may have been retired")
	}
	return extract.NewError(extract.CodeRejected, false, "the provider rejected the request (status %d)%s", status, suffix)
}

var _ extract.Model = (*Model)(nil)
