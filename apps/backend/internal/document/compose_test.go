package document

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
)

func TestComposeText(t *testing.T) {
	cases := []struct {
		name string
		q    DraftQuestion
		want string
	}{
		{"plain question", DraftQuestion{Text: " What is 2 + 2? "}, "What is 2 + 2?"},
		{"instructions come first", DraftQuestion{Instructions: "Answer in full sentences.", Text: "Define velocity."},
			"Answer in full sentences.\n\nDefine velocity."},
		{"table as rows without the divider", DraftQuestion{Text: "Read the table.", Tables: []extract.Table{{Page: 2, Markdown: "| a | b |\n|---|:-:|\n| 1 | 2 |"}}},
			"Read the table.\n\nTable (page 2):\na | b\n1 | 2"},
		{"table with no outer bars", DraftQuestion{Text: "T", Tables: []extract.Table{{Page: 1, Markdown: "x | y\n--- | ---\n3 | 4"}}},
			"T\n\nTable (page 1):\nx | y\n3 | 4"},
		{"a table with nothing in it adds nothing", DraftQuestion{Text: "T", Tables: []extract.Table{{Page: 1, Markdown: "|---|---|"}}}, "T"},
		{"diagram", DraftQuestion{Text: "Use the figure.", Diagrams: []extract.Diagram{{Page: 3, Description: " A prism splitting light. "}}},
			"Use the figure.\n\nDiagram (page 3): A prism splitting light."},
		{"a diagram without a description adds nothing", DraftQuestion{Text: "Use the figure.", Diagrams: []extract.Diagram{{Page: 3}}}, "Use the figure."},
		{"everything, in order", DraftQuestion{
			Instructions: "I.", Text: "Q.",
			Tables:   []extract.Table{{Page: 1, Markdown: "| a |\n|---|\n| 1 |"}, {Page: 2, Markdown: "| b |\n|---|\n| 2 |"}},
			Diagrams: []extract.Diagram{{Page: 1, Description: "D1"}, {Page: 2, Description: "D2"}},
		}, "I.\n\nQ.\n\nTable (page 1):\na\n1\n\nTable (page 2):\nb\n2\n\nDiagram (page 1): D1\n\nDiagram (page 2): D2"},
		{"unicode and maths survive", DraftQuestion{Text: "Résoudre 2x² − 3x + 1 = 0, x ∈ ℝ"}, "Résoudre 2x² − 3x + 1 = 0, x ∈ ℝ"},
		{"windows line endings in a table", DraftQuestion{Text: "T", Tables: []extract.Table{{Page: 1, Markdown: "| a |\r\n|---|\r\n| 1 |"}}}, "T\n\nTable (page 1):\na\n1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ComposeText(tc.q))
		})
	}
}

func TestProblemsForApproval_AcceptsACompleteQuestion(t *testing.T) {
	pts := 3
	q := DraftQuestion{
		Text: "Which is a vector?", Type: extract.TypeMCQ, Points: &pts,
		Options:  []extract.Option{{Label: "A", Text: "Mass"}, {Label: "B", Text: "Velocity", IsCorrect: true}},
		Diagrams: []extract.Diagram{{Page: 1, Description: "d", Confirmed: true}},
		Warnings: []extract.Warning{{Code: "X", Blocking: true, Acknowledged: true}, {Code: "Y", Blocking: false}},
	}
	assert.Empty(t, problemsForApproval([]DraftQuestion{q}))
}

func TestProblemsForApproval_ChecksStructureEvenIfTheDatabaseAllowedIt(t *testing.T) {
	pts := 1
	q := DraftQuestion{
		Text: "x", Type: extract.TypeEssay, Points: &pts,
		Options: []extract.Option{{Label: "A", Text: "stray"}},
	}
	fields := map[string]bool{}
	for _, p := range problemsForApproval([]DraftQuestion{q}) {
		fields[p.Field] = true
	}
	assert.True(t, fields["text"], "one character is not a question")
	assert.True(t, fields["options"], "a written question has no options")

	q = DraftQuestion{Text: "A question?", Type: extract.TypeMCQ, Points: &pts,
		Options: []extract.Option{{Label: "A", Text: "one", IsCorrect: true}, {Label: "a", Text: "two", IsCorrect: true}, {Label: "", Text: ""}}}
	fields = map[string]bool{}
	for _, p := range problemsForApproval([]DraftQuestion{q}) {
		fields[p.Field] = true
	}
	assert.True(t, fields["options[1].label"], "labels are compared without regard to case")
	assert.True(t, fields["options[2].label"])
	assert.True(t, fields["options[2].text"])
	assert.True(t, fields["options"], "two correct options")
}
