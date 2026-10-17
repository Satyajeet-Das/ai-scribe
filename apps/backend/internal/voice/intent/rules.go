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

// dictationLeads are the spoken phrases that mark the rest of the utterance as a
// free-text answer, as lists of words. Longest first.
var dictationLeads = [][]string{
	{"my", "answer", "is"},
	{"the", "answer", "is"},
	{"write", "down"},
	{"write"},
}

// dictationFillers may come before the phrase ("okay, my answer is ..."). At
// most two are skipped, so the phrase still has to be how the sentence begins.
var dictationFillers = map[string]bool{
	"okay": true, "ok": true, "so": true, "well": true, "um": true, "uh": true, "alright": true,
}

// DictationBody finds an explicit spoken dictation phrase at the start of the
// transcript ("my answer is ...", "write down ...", optionally after a filler
// such as "okay") and returns what follows it, in the student's own words.
//
// It is the one way speech in command mode may become answer text. Nothing a
// model says can stand in for it: a model may label an utterance as dictation,
// but only these words, said by the student, make it one.
func DictationBody(transcript string) (string, bool) {
	fields := strings.Fields(transcript)
	i := 0
	for i < len(fields) && i < 2 && dictationFillers[Normalize(fields[i])] {
		i++
	}
	for _, lead := range dictationLeads {
		if i+len(lead) >= len(fields) {
			continue // the phrase must be followed by something
		}
		matched := true
		for k, word := range lead {
			if Normalize(fields[i+k]) != word {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		body := strings.TrimSpace(strings.Join(fields[i+len(lead):], " "))
		if Normalize(body) == "" {
			return "", false
		}
		return body, true
	}
	return "", false
}

// HasDictationPrefix reports whether the transcript begins with an explicit
// spoken dictation phrase followed by content. See DictationBody.
func HasDictationPrefix(transcript string) bool {
	_, ok := DictationBody(transcript)
	return ok
}

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

	if _, ok := DictationBody(in.Transcript); ok {
		return Intent{Name: DictateAnswer, Confidence: 1}, nil
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
