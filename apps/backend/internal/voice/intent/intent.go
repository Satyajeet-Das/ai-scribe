// Package intent turns a student's transcript into one of a closed set of
// typed intents. A classifier only ever *interprets*: it returns a value that
// the backend validates and then decides what, if anything, to do with. It has
// no access to exam state, answer keys or grading data, and cannot execute
// anything.
package intent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

// Name identifies a permitted intent.
type Name string

const (
	RepeatQuestion   Name = "repeat_question"
	NextQuestion     Name = "next_question"
	PreviousQuestion Name = "previous_question"
	SelectOption     Name = "select_option"
	DictateAnswer    Name = "dictate_answer"
	ReadBackAnswer   Name = "read_back_answer"
	TimeRemaining    Name = "time_remaining"
	Confirm          Name = "confirm"
	Deny             Name = "deny"
	// Unknown means the utterance did not clearly match a permitted intent and
	// the student must be asked to clarify.
	Unknown Name = "unknown"
)

// Names is the complete, ordered set of permitted intents.
var Names = []Name{
	RepeatQuestion, NextQuestion, PreviousQuestion, SelectOption, DictateAnswer,
	ReadBackAnswer, TimeRemaining, Confirm, Deny, Unknown,
}

// Valid reports whether n is one of the permitted intents.
func (n Name) Valid() bool {
	for _, k := range Names {
		if n == k {
			return true
		}
	}
	return false
}

// Intent is a classifier's interpretation of one utterance.
type Intent struct {
	Name Name `json:"intent"`
	// OptionKey is set only for SelectOption and always names one of the
	// options offered in the Input.
	OptionKey  string  `json:"option_key"`
	Confidence float64 `json:"confidence"`
}

// Option is one answer choice as the student sees it.
// It deliberately has no notion of correctness.
type Option struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// Input is everything a classifier may see.
type Input struct {
	Transcript string
	// QuestionType is the current question's type (MCQ, ESSAY, VOICE).
	QuestionType string
	// Options are the current question's choices; empty for free-text questions.
	Options []Option
	// AwaitingConfirmation is true while the backend holds an unconfirmed
	// answer and is waiting for a yes or no.
	AwaitingConfirmation bool
}

// Classifier interprets a transcript.
type Classifier interface {
	Name() string
	Classify(ctx context.Context, in Input) (Intent, error)
}

// ErrInvalidIntent means classifier output failed schema validation. It is
// handled like an ambiguous utterance: the student is asked to clarify.
var ErrInvalidIntent = errors.New("intent: output failed validation")

// Validate checks an intent against the schema and the current question, and
// returns it with the option key normalized to the question's own spelling.
// Classifier output is untrusted input, whichever classifier produced it.
func Validate(it Intent, in Input) (Intent, error) {
	if !it.Name.Valid() {
		return Intent{}, fmt.Errorf("%w: unknown intent", ErrInvalidIntent)
	}
	if math.IsNaN(it.Confidence) || it.Confidence < 0 || it.Confidence > 1 {
		return Intent{}, fmt.Errorf("%w: confidence out of range", ErrInvalidIntent)
	}

	key := strings.TrimSpace(it.OptionKey)
	if it.Name != SelectOption {
		if key != "" {
			return Intent{}, fmt.Errorf("%w: option_key is only allowed with select_option", ErrInvalidIntent)
		}
		it.OptionKey = ""
		return it, nil
	}

	if key == "" {
		return Intent{}, fmt.Errorf("%w: select_option requires option_key", ErrInvalidIntent)
	}
	for _, opt := range in.Options {
		if strings.EqualFold(opt.Key, key) {
			it.OptionKey = opt.Key
			return it, nil
		}
	}
	return Intent{}, fmt.Errorf("%w: option_key is not an option of the current question", ErrInvalidIntent)
}

// Parse strictly decodes classifier JSON output and validates it. Unknown
// fields, trailing data and wrong types are all rejected.
func Parse(data []byte, in Input) (Intent, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var raw struct {
		Intent     *string  `json:"intent"`
		OptionKey  *string  `json:"option_key"`
		Confidence *float64 `json:"confidence"`
	}
	if err := dec.Decode(&raw); err != nil {
		return Intent{}, fmt.Errorf("%w: malformed JSON", ErrInvalidIntent)
	}
	if dec.More() {
		return Intent{}, fmt.Errorf("%w: trailing data", ErrInvalidIntent)
	}
	if raw.Intent == nil || raw.Confidence == nil {
		return Intent{}, fmt.Errorf("%w: missing required field", ErrInvalidIntent)
	}

	it := Intent{Name: Name(*raw.Intent), Confidence: *raw.Confidence}
	if raw.OptionKey != nil {
		it.OptionKey = *raw.OptionKey
	}
	return Validate(it, in)
}

// Schema is the JSON schema that classifier output must satisfy. It is sent to
// the model as the structured-output format; Parse re-validates regardless.
func Schema() map[string]any {
	names := make([]string, len(Names))
	for i, n := range Names {
		names[i] = string(n)
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"intent":     map[string]any{"type": "string", "enum": names},
			"option_key": map[string]any{"type": "string", "description": "The key of the chosen option for select_option; otherwise an empty string."},
			"confidence": map[string]any{"type": "number", "description": "Confidence between 0 and 1."},
		},
		"required":             []string{"intent", "option_key", "confidence"},
		"additionalProperties": false,
	}
}

// Chain consults a fast deterministic classifier first and falls back to a
// second one only when the first cannot place the utterance.
type Chain struct {
	First  Classifier
	Second Classifier
}

func (c Chain) Name() string { return c.First.Name() + "+" + c.Second.Name() }

func (c Chain) Classify(ctx context.Context, in Input) (Intent, error) {
	it, err := c.First.Classify(ctx, in)
	if err == nil && it.Name != Unknown {
		return it, nil
	}
	return c.Second.Classify(ctx, in)
}
