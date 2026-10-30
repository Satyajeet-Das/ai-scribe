// Package extracttest holds a scripted extract.Model for tests, so no test needs
// a real provider or the network.
package extracttest

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
)

// Reply is what the fake returns for one call: a body, or an error.
type Reply struct {
	Body string
	Err  error
}

// Model returns its replies in order, one per call, and repeats the last. It
// records every request so a test can assert what the model was sent.
type Model struct {
	Replies []Reply
	// Vision is what CanReadImages reports.
	Vision bool
	// Block, when set, makes a call wait for the channel or the context.
	Block <-chan struct{}

	mu       sync.Mutex
	Requests []extract.Request
}

func (m *Model) Name() string          { return "fake" }
func (m *Model) PromptVersion() string { return "test" }
func (m *Model) CanReadImages() bool   { return m.Vision }

func (m *Model) Complete(ctx context.Context, req extract.Request) (string, error) {
	m.mu.Lock()
	m.Requests = append(m.Requests, req)
	i := len(m.Requests) - 1
	if i >= len(m.Replies) {
		i = len(m.Replies) - 1
	}
	var r Reply
	if i >= 0 {
		r = m.Replies[i]
	}
	block := m.Block
	m.mu.Unlock()

	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return r.Body, r.Err
}

// Calls reports how many requests were made.
func (m *Model) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Requests)
}

// ReplyJSON wraps question objects (see MCQ and Essay) in the reply envelope a
// model is asked to produce.
func ReplyJSON(questions ...string) string {
	return `{"documentInstructions": null, "questions": [` + strings.Join(questions, ",") + `], "unassigned": []}`
}

// MCQ is a multiple-choice question object. Options are labelled A, B, C...
func MCQ(label, text string, page int, options ...string) string {
	opts := make([]string, len(options))
	for i, o := range options {
		opts[i] = fmt.Sprintf(`{"label": %q, "text": %q}`, string(rune('A'+i)), o)
	}
	return fmt.Sprintf(`{"numberLabel": %q, "type": "MCQ", "instructions": null, "text": %q, "marks": null,
		"options": [%s], "tables": [], "diagrams": [], "pages": [%d], "continuesPrevious": false, "uncertain": false}`,
		label, text, strings.Join(opts, ","), page)
}

// Essay is a written-answer question object.
func Essay(label, text string, page int) string {
	return fmt.Sprintf(`{"numberLabel": %q, "type": "ESSAY", "instructions": null, "text": %q, "marks": null,
		"options": [], "tables": [], "diagrams": [], "pages": [%d], "continuesPrevious": false, "uncertain": false}`,
		label, text, page)
}
