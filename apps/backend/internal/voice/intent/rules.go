package intent

import (
	"context"
	"strings"
	"unicode"
)

// Rules is a deterministic classifier that recognizes a fixed vocabulary of
// exact commands. It never guesses: anything outside the vocabulary is
// Unknown. It needs no network, so it also works when the LLM is unavailable.
type Rules struct{}

func (Rules) Name() string { return "rules" }

var phrases = map[string]Name{
	"repeat": RepeatQuestion, "repeat that": RepeatQuestion, "repeat the question": RepeatQuestion,
	"repeat question": RepeatQuestion, "say that again": RepeatQuestion, "say it again": RepeatQuestion,
	"read the question": RepeatQuestion, "read the question again": RepeatQuestion, "read question": RepeatQuestion,
	"read it again": RepeatQuestion, "again": RepeatQuestion, "read the options": RepeatQuestion,
	"repeat the options": RepeatQuestion,

	"next": NextQuestion, "next question": NextQuestion, "go to the next question": NextQuestion,
	"go to next question": NextQuestion, "go next": NextQuestion, "move on": NextQuestion, "skip": NextQuestion,

	"previous": PreviousQuestion, "previous question": PreviousQuestion, "go back": PreviousQuestion,
	"back": PreviousQuestion, "go to the previous question": PreviousQuestion,

	"read my answer": ReadBackAnswer, "read back my answer": ReadBackAnswer, "what is my answer": ReadBackAnswer,
	"what did i answer": ReadBackAnswer, "read back": ReadBackAnswer, "what was my answer": ReadBackAnswer,
	"what did i choose": ReadBackAnswer,

	"how much time is left": TimeRemaining, "how much time do i have": TimeRemaining, "time remaining": TimeRemaining,
	"time left": TimeRemaining, "how much time do i have left": TimeRemaining, "how much time remains": TimeRemaining,

	"yes": Confirm, "yes please": Confirm, "confirm": Confirm, "correct": Confirm, "that is correct": Confirm,
	"yes confirm": Confirm, "save it": Confirm, "yes save it": Confirm,

	"no": Deny, "cancel": Deny, "no cancel": Deny, "wrong": Deny, "that is wrong": Deny, "no thanks": Deny,
	"do not save": Deny,
}

// optionPrefixes may precede an option key; they are tried longest first.
var optionPrefixes = []string{
	"my answer is option ", "the answer is option ", "i choose option ", "i select option ", "select option ",
	"choose option ", "my answer is ", "the answer is ", "i choose ", "i select ", "option ", "answer ", "choice ",
	"select ", "choose ",
}

// dictationPrefixes mark the rest of the utterance as a free-text answer.
var dictationPrefixes = []string{"my answer is ", "the answer is ", "write ", "write down "}

func (Rules) Classify(_ context.Context, in Input) (Intent, error) {
	text := Normalize(in.Transcript)
	if text == "" {
		return Intent{Name: Unknown}, nil
	}

	if name, ok := phrases[text]; ok {
		return Intent{Name: name, Confidence: 1}, nil
	}

	if len(in.Options) > 0 {
		candidate := text
		for _, p := range optionPrefixes {
			if strings.HasPrefix(text, p) {
				candidate = strings.TrimPrefix(text, p)
				break
			}
		}
		// An exact option key ("b", "option b").
		for _, opt := range in.Options {
			if Normalize(opt.Key) == candidate {
				return Intent{Name: SelectOption, OptionKey: opt.Key, Confidence: 1}, nil
			}
		}
		// The exact text of exactly one option.
		match, count := "", 0
		for _, opt := range in.Options {
			if Normalize(opt.Text) == candidate {
				match = opt.Key
				count++
			}
		}
		if count == 1 {
			return Intent{Name: SelectOption, OptionKey: match, Confidence: 1}, nil
		}
		return Intent{Name: Unknown}, nil
	}

	for _, p := range dictationPrefixes {
		if strings.HasPrefix(text, p) && len(text) > len(p) {
			return Intent{Name: DictateAnswer, Confidence: 1}, nil
		}
	}
	return Intent{Name: Unknown}, nil
}

// Normalize lower-cases text, drops punctuation and collapses whitespace.
func Normalize(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		case r == '\'' || r == '’':
			// Drop apostrophes so "what's" and "whats" compare equal.
		default:
			space = true
		}
	}
	return b.String()
}
