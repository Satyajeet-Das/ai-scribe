package document

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
)

// Limits on a draft question, enforced on every teacher edit. They match what
// extraction produces, so an edited question is never refused for being longer
// than one that was extracted.
const (
	maxDraftQuestions = 500
	maxLabelRunes     = 30
	maxTextRunes      = 8000
	maxInstructRunes  = 4000
	maxOptionsPerQ    = 12
	maxOptionKeyRunes = 10
	maxOptionRunes    = 2000
	maxTablesPerQ     = 10
	maxTableRunes     = 8000
	maxDiagramsPerQ   = 10
	maxDescRunes      = 2000
	maxRegionRunes    = 200
	maxPoints         = 1000
)

// OptionInput, TableInput and DiagramInput are the parts of a question a
// teacher can write.
type OptionInput struct {
	Label     string
	Text      string
	IsCorrect bool
}

type TableInput struct {
	Page     int
	Markdown string
}

type DiagramInput struct {
	Page        int
	Region      string
	Description string
	// Confirmed says the teacher has checked the description against the figure.
	Confirmed bool
}

// QuestionInput is the complete state a teacher wants for one draft question:
// a replacement, not a patch.
type QuestionInput struct {
	NumberLabel  string
	Type         string
	Instructions string
	Text         string
	Points       *int
	Options      []OptionInput
	Tables       []TableInput
	Diagrams     []DiagramInput
	// AcknowledgedWarnings are the codes of warnings the teacher has looked at.
	// Only warnings the system raised can be acknowledged; this cannot add,
	// remove or reword one.
	AcknowledgedWarnings []string
}

func runes(s string) int { return utf8.RuneCountInString(s) }

// validateInput checks a teacher's edit against the rules every draft question
// obeys. Completeness (marks set, enough options, diagrams confirmed) is not
// required here: a teacher may save work in progress. It is required to approve.
func validateInput(in QuestionInput, pageCount int) error {
	text := strings.TrimSpace(in.Text)
	if runes(text) < 2 {
		return invalid("text", "must be at least 2 characters")
	}
	if runes(in.Text) > maxTextRunes {
		return invalid("text", "must not exceed %d characters", maxTextRunes)
	}
	if runes(in.NumberLabel) > maxLabelRunes {
		return invalid("numberLabel", "must not exceed %d characters", maxLabelRunes)
	}
	if runes(in.Instructions) > maxInstructRunes {
		return invalid("instructions", "must not exceed %d characters", maxInstructRunes)
	}
	switch in.Type {
	case extract.TypeMCQ, extract.TypeEssay:
	default:
		return invalid("type", "must be one of: MCQ, ESSAY")
	}
	if in.Points != nil && (*in.Points < 0 || *in.Points > maxPoints) {
		return invalid("points", "must be between 0 and %d", maxPoints)
	}

	if in.Type == extract.TypeEssay && len(in.Options) > 0 {
		return invalid("options", "are only allowed on multiple-choice questions")
	}
	if len(in.Options) > maxOptionsPerQ {
		return invalid("options", "must not have more than %d entries", maxOptionsPerQ)
	}
	seen := map[string]bool{}
	correct := 0
	for i, o := range in.Options {
		f := fmt.Sprintf("options[%d]", i)
		key := strings.ToUpper(strings.TrimSpace(o.Label))
		if key == "" {
			return invalid(f+".label", "is required")
		}
		if runes(key) > maxOptionKeyRunes {
			return invalid(f+".label", "must not exceed %d characters", maxOptionKeyRunes)
		}
		for _, r := range key {
			if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
				return invalid(f+".label", "may only contain letters and digits")
			}
		}
		if seen[key] {
			return invalid(f+".label", "%q is used by another option", key)
		}
		seen[key] = true
		if strings.TrimSpace(o.Text) == "" {
			return invalid(f+".text", "is required")
		}
		if runes(o.Text) > maxOptionRunes {
			return invalid(f+".text", "must not exceed %d characters", maxOptionRunes)
		}
		if o.IsCorrect {
			correct++
		}
	}
	if correct > 1 {
		return invalid("options", "only one option can be marked correct")
	}

	if len(in.Tables) > maxTablesPerQ {
		return invalid("tables", "must not have more than %d entries", maxTablesPerQ)
	}
	for i, t := range in.Tables {
		f := fmt.Sprintf("tables[%d]", i)
		if t.Page < 1 || t.Page > pageCount {
			return invalid(f+".page", "must be between 1 and %d", pageCount)
		}
		if strings.TrimSpace(t.Markdown) == "" {
			return invalid(f+".markdown", "is required")
		}
		if runes(t.Markdown) > maxTableRunes {
			return invalid(f+".markdown", "must not exceed %d characters", maxTableRunes)
		}
	}
	if len(in.Diagrams) > maxDiagramsPerQ {
		return invalid("diagrams", "must not have more than %d entries", maxDiagramsPerQ)
	}
	for i, d := range in.Diagrams {
		f := fmt.Sprintf("diagrams[%d]", i)
		if d.Page < 1 || d.Page > pageCount {
			return invalid(f+".page", "must be between 1 and %d", pageCount)
		}
		if runes(d.Region) > maxRegionRunes {
			return invalid(f+".region", "must not exceed %d characters", maxRegionRunes)
		}
		if runes(d.Description) > maxDescRunes {
			return invalid(f+".description", "must not exceed %d characters", maxDescRunes)
		}
		if d.Confirmed && strings.TrimSpace(d.Description) == "" {
			return invalid(f+".confirmed", "a diagram with no description cannot be confirmed")
		}
	}
	return nil
}

// applyInput writes a teacher's edit over a draft question. What the system
// knows and the teacher cannot change (source pages, the warnings it raised,
// where the question came from) is kept; the teacher's acknowledgements are
// taken from the request.
func applyInput(q *DraftQuestion, in QuestionInput) {
	q.NumberLabel = strings.TrimSpace(in.NumberLabel)
	q.Type = in.Type
	q.Instructions = strings.TrimSpace(in.Instructions)
	q.Text = strings.TrimSpace(in.Text)
	q.Points = in.Points

	q.Options = make([]extract.Option, 0, len(in.Options))
	for _, o := range in.Options {
		q.Options = append(q.Options, extract.Option{
			Label: strings.ToUpper(strings.TrimSpace(o.Label)), Text: strings.TrimSpace(o.Text), IsCorrect: o.IsCorrect,
		})
	}
	q.Tables = make([]extract.Table, 0, len(in.Tables))
	for _, t := range in.Tables {
		q.Tables = append(q.Tables, extract.Table{Page: t.Page, Markdown: strings.TrimSpace(t.Markdown)})
	}

	// A description keeps its source until the teacher changes it; one that is
	// new, or reworded, is the teacher's.
	old := q.Diagrams
	q.Diagrams = make([]extract.Diagram, 0, len(in.Diagrams))
	for i, d := range in.Diagrams {
		desc := strings.TrimSpace(d.Description)
		src := ""
		if desc != "" {
			src = "TEACHER"
			if i < len(old) && strings.TrimSpace(old[i].Description) == desc && old[i].DescriptionSource != "" {
				src = old[i].DescriptionSource
			}
		}
		q.Diagrams = append(q.Diagrams, extract.Diagram{
			Page: d.Page, Region: strings.TrimSpace(d.Region), Description: desc, DescriptionSource: src,
			Confirmed: d.Confirmed && desc != "",
		})
	}

	ack := map[string]bool{}
	for _, c := range in.AcknowledgedWarnings {
		ack[c] = true
	}
	for i := range q.Warnings {
		q.Warnings[i].Acknowledged = q.Warnings[i].Blocking && ack[q.Warnings[i].Code]
	}
	q.Edited = true
}

// problemsForApproval lists everything that stops these questions being added
// to the exam. Unlike an edit, approval needs a complete question.
func problemsForApproval(qs []DraftQuestion) []Problem {
	var out []Problem
	if len(qs) == 0 {
		return []Problem{{Field: "questions", Message: "The draft has no questions to add."}}
	}
	for _, q := range qs {
		add := func(field, format string, args ...any) {
			out = append(out, Problem{QuestionID: q.ID, Position: q.Position, Field: field, Message: fmt.Sprintf(format, args...)})
		}
		if runes(strings.TrimSpace(q.Text)) < 2 {
			add("text", "The question has no text.")
		}
		if q.Points == nil {
			add("points", "Set the marks for this question.")
		}
		switch q.Type {
		case extract.TypeMCQ:
			if len(q.Options) < 2 {
				add("options", "A multiple-choice question needs at least 2 options.")
			}
		case extract.TypeEssay:
			if len(q.Options) > 0 {
				add("options", "A written question cannot have options.")
			}
		default:
			add("type", "The question type is not valid.")
		}
		keys := map[string]bool{}
		correct := 0
		for i, o := range q.Options {
			key := strings.ToUpper(strings.TrimSpace(o.Label))
			if key == "" || runes(key) > maxOptionKeyRunes {
				add(fmt.Sprintf("options[%d].label", i), "Option %d needs a label of 1 to %d characters.", i+1, maxOptionKeyRunes)
			}
			if keys[key] {
				add(fmt.Sprintf("options[%d].label", i), "Two options are labelled %q.", key)
			}
			keys[key] = true
			if strings.TrimSpace(o.Text) == "" {
				add(fmt.Sprintf("options[%d].text", i), "Option %s has no text.", key)
			}
			if o.IsCorrect {
				correct++
			}
		}
		if correct > 1 {
			add("options", "More than one option is marked correct.")
		}
		for i, d := range q.Diagrams {
			if strings.TrimSpace(d.Description) == "" {
				add(fmt.Sprintf("diagrams[%d].description", i), "The diagram on page %d needs a description so it can be read aloud.", d.Page)
			} else if !d.Confirmed {
				add(fmt.Sprintf("diagrams[%d].confirmed", i), "Check the description of the diagram on page %d against the figure and confirm it.", d.Page)
			}
		}
		for _, w := range q.Warnings {
			if w.Blocking && !w.Acknowledged {
				add("warnings."+w.Code, "Check this and acknowledge it: %s", w.Message)
			}
		}
	}
	return out
}
