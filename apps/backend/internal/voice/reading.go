package voice

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Satyajeet-Das/ai-scribe/internal/session"
)

// msgUnreadable is appended to a question that contains something speech
// cannot faithfully convey. It never tries to describe the content: a wrong
// description of a diagram or formula would be worse than none.
const msgUnreadable = "Note: this question contains content, such as a formula, image or diagram, that may not be read out correctly. Ask your invigilator if anything is unclear."

var (
	// Markup and notation that a speech synthesizer reads as noise or skips:
	// $...$ maths (but not "$5 and $3" prices), \( \[ and \command, HTML
	// image, svg, maths and table tags, markdown images, and image URLs.
	unreadableMarkup = regexp.MustCompile(
		`\$[^\s$][^$]*[^\s$]\$|\$[^\s$]\$|[\\][(\[]|[\\][a-zA-Z]{2,}` +
			`|<\s*(img|svg|math|table)\b|!\[[^\]]*\]\(|\bhttps?://\S+\.(png|jpe?g|gif|svg|webp)\b`)

	// A reference to something the student is expected to look at.
	unreadableReference = regexp.MustCompile(
		`(?i)\b(figure|fig\.|diagram|image|graph|chart|picture|illustration)\b[^.?!]{0,40}\b(above|below|shown|attached|given)\b` +
			`|\b(see|refer to|look at|shown in|given in)\b[^.?!]{0,20}\b(figure|fig\.|diagram|image|graph|chart|picture|illustration|table)\b`)
)

// hasUnreadableContent reports whether a question appears to depend on content
// that text-to-speech cannot convey faithfully. It is a heuristic and errs
// towards warning: a false warning costs a sentence, a missed one costs a
// student a question they cannot understand.
func hasUnreadableContent(q *session.StudentQuestionResponse) bool {
	texts := []string{q.Text}
	for _, o := range q.Options {
		texts = append(texts, o.OptionText)
	}
	for _, t := range texts {
		if unreadableMarkup.MatchString(t) || unreadableReference.MatchString(t) {
			return true
		}
	}
	return false
}

// sentence trims text and makes sure it ends with punctuation, so speech
// pauses between the question, its options and the instructions around them.
func sentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	switch text[len(text)-1] {
	case '.', '?', '!', ':', ';':
		return text
	}
	return text + "."
}

// readQuestion renders a question for speech, from the student-facing DTO.
// Each part is its own sentence so the synthesizer pauses between them.
func readQuestion(q *session.StudentQuestionResponse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Question %d of %d.", q.CurrentIndex, q.TotalQuestions)
	if t := sentence(q.Text); t != "" {
		b.WriteString(" " + t)
	}
	if len(q.Options) > 0 {
		b.WriteString(" The options are.")
		for _, o := range q.Options {
			fmt.Fprintf(&b, " Option %s. %s", o.OptionKey, sentence(o.OptionText))
		}
	}
	if hasUnreadableContent(q) {
		b.WriteString(" " + msgUnreadable)
	}
	return b.String()
}
