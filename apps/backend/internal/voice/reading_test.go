package voice

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Satyajeet-Das/ai-scribe/internal/session"
)

func newQuestion(text string, options ...string) *session.StudentQuestionResponse {
	q := &session.StudentQuestionResponse{CurrentIndex: 2, TotalQuestions: 5, Text: text}
	keys := []string{"A", "B", "C", "D"}
	for i, o := range options {
		q.Options = append(q.Options, session.StudentOptionResponse{OptionKey: keys[i], OptionText: o})
	}
	return q
}

func TestReadQuestion_PausesBetweenPartsAndKeepsOrder(t *testing.T) {
	got := readQuestion(newQuestion("What is the capital of France", "Paris", "Rome."))
	assert.Equal(t, "Question 2 of 5. What is the capital of France. The options are. Option A. Paris. Option B. Rome.", got)
}

func TestReadQuestion_WithoutOptionsHasNoOptionsIntro(t *testing.T) {
	got := readQuestion(newQuestion("Explain photosynthesis?"))
	assert.Equal(t, "Question 2 of 5. Explain photosynthesis?", got)
}

func TestReadQuestion_WarnsAboutContentSpeechCannotConvey(t *testing.T) {
	cases := map[string]*session.StudentQuestionResponse{
		"inline latex":    newQuestion("Solve $x^2 + 3x = 0$ for x."),
		"latex command":   newQuestion(`Evaluate \frac{a}{b} when a is 4.`),
		"image tag":       newQuestion(`Name the part shown <img src="cell.png">`),
		"figure below":    newQuestion("Label the organelle shown in the figure below."),
		"refer to figure": newQuestion("Refer to the diagram and name the largest bone."),
		"option formula":  newQuestion("Which is correct?", "$a+b$", "c"),
	}
	for name, q := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, readQuestion(q), msgUnreadable)
		})
	}
}

func TestReadQuestion_PlainTextIsNotFlagged(t *testing.T) {
	for _, q := range []*session.StudentQuestionResponse{
		newQuestion("Which planet is closest to the sun?", "Mercury", "Venus"),
		newQuestion("Describe the picture of life in a village you know well."),
		newQuestion("The price rose by $5 and then fell by $3. What is the net change?"),
	} {
		assert.NotContains(t, readQuestion(q), msgUnreadable, q.Text)
	}
}
