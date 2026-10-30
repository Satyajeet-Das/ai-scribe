package extract

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits on what a reply may contain. They bound what a misbehaving model can
// make the server hold and store.
const (
	maxReplyBytes      = 1 << 20
	maxQuestionsBatch  = 300
	maxOptions         = 12
	maxTextRunes       = 8000
	maxOptionRunes     = 2000
	maxInstructRunes   = 4000
	maxTableRunes      = 8000
	maxDescriptionRune = 2000
	maxLabelRunes      = 30
	maxUnassignedRunes = 2000
)

type rawOption struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

type rawDiagram struct {
	Page        int     `json:"page"`
	Region      *string `json:"region"`
	Description *string `json:"description"`
}

type rawQuestion struct {
	NumberLabel       string       `json:"numberLabel"`
	Type              string       `json:"type"`
	Instructions      *string      `json:"instructions"`
	Text              string       `json:"text"`
	Marks             *json.Number `json:"marks"`
	Options           []rawOption  `json:"options"`
	Tables            []Table      `json:"tables"`
	Diagrams          []rawDiagram `json:"diagrams"`
	Pages             []int        `json:"pages"`
	ContinuesPrevious bool         `json:"continuesPrevious"`
	Uncertain         bool         `json:"uncertain"`
}

type rawUnassigned struct {
	Page   int    `json:"page"`
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

type rawResult struct {
	DocumentInstructions *string         `json:"documentInstructions"`
	Questions            *[]rawQuestion  `json:"questions"`
	Unassigned           []rawUnassigned `json:"unassigned"`
}

// batchQuestion is a validated question plus what only the join step needs.
type batchQuestion struct {
	Question
	continues bool
	uncertain bool
}

type batchResult struct {
	instructions string
	questions    []batchQuestion
	unassigned   []Unassigned
}

// extractJSON finds the JSON object in a reply that may be wrapped in a code
// fence or surrounded by words.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end < start {
		return ""
	}
	return s[start : end+1]
}

// cleanText removes control characters other than newline and tab, normalises
// line endings and trims. Unicode, including mathematical notation, is kept.
func cleanText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r == utf8.RuneError, unicode.IsControl(r), r == ' ', r == ' ':
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// limit cuts s to max runes and reports whether it did.
func limit(s string, max int) (string, bool) {
	if utf8.RuneCountInString(s) <= max {
		return s, false
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:max])), true
}

func warn(q *Question, code, msg string, blocking bool) {
	for _, w := range q.Warnings {
		if w.Code == code && w.Message == msg {
			return
		}
	}
	q.Warnings = append(q.Warnings, Warning{Code: code, Message: msg, Blocking: blocking})
}

// normaliseLabel reduces an option label as printed ("(b)", "B.", "iii)") to the
// bare label in upper case.
func normaliseLabel(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "()[].:")
	s = strings.TrimSpace(s)
	return strings.ToUpper(s)
}

// parse validates one reply. pages are the page numbers the batch covered.
//
// A reply that is not the expected object is an error. Problems inside a
// question never remove it: they become warnings, so that nothing the model
// found is lost without the teacher being told.
func parse(reply string, pages []int) (*batchResult, error) {
	if len(reply) > maxReplyBytes {
		return nil, NewError(CodeInvalidOutput, false, "reply too large")
	}
	body := extractJSON(reply)
	if body == "" {
		return nil, NewError(CodeInvalidOutput, false, "reply is not a JSON object")
	}
	var raw rawResult
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, NewError(CodeInvalidOutput, false, "reply is not valid JSON")
	}
	if raw.Questions == nil {
		return nil, NewError(CodeInvalidOutput, false, "reply has no questions list")
	}
	if len(*raw.Questions) > maxQuestionsBatch {
		return nil, NewError(CodeInvalidOutput, false, "reply has too many questions")
	}

	inBatch := make(map[int]bool, len(pages))
	for _, p := range pages {
		inBatch[p] = true
	}
	firstPage := 0
	if len(pages) > 0 {
		firstPage = pages[0]
	}

	out := &batchResult{}
	if raw.DocumentInstructions != nil {
		out.instructions, _ = limit(cleanText(*raw.DocumentInstructions), maxInstructRunes)
	}

	for _, rq := range *raw.Questions {
		out.questions = append(out.questions, buildQuestion(rq, inBatch, firstPage))
	}
	for _, ru := range raw.Unassigned {
		text, _ := limit(cleanText(ru.Text), maxUnassignedRunes)
		if text == "" {
			continue
		}
		page := ru.Page
		if !inBatch[page] {
			page = firstPage
		}
		reason, _ := limit(cleanText(ru.Reason), 200)
		out.unassigned = append(out.unassigned, Unassigned{Page: page, Text: text, Reason: reason})
	}
	return out, nil
}

func buildQuestion(rq rawQuestion, inBatch map[int]bool, firstPage int) batchQuestion {
	q := Question{
		Options: []Option{}, Tables: []Table{}, Diagrams: []Diagram{}, Warnings: []Warning{},
		Confidence: ConfidenceHigh,
	}
	bq := batchQuestion{continues: rq.ContinuesPrevious, uncertain: rq.Uncertain}

	label, _ := limit(cleanText(rq.NumberLabel), maxLabelRunes)
	q.NumberLabel = label
	q.Text, _ = limit(cleanText(rq.Text), maxTextRunes)
	if utf8.RuneCountInString(cleanText(rq.Text)) > maxTextRunes {
		warn(&q, WarnTextTooLong, "The question text was longer than allowed and was cut. Check the original.", true)
	}
	if rq.Instructions != nil {
		q.Instructions, _ = limit(cleanText(*rq.Instructions), maxInstructRunes)
	}

	// Source pages: only pages the batch covered.
	seen := map[int]bool{}
	for _, p := range rq.Pages {
		if inBatch[p] && !seen[p] {
			seen[p] = true
			q.SourcePages = append(q.SourcePages, p)
		}
	}
	if len(q.SourcePages) == 0 {
		q.SourcePages = []int{firstPage}
		warn(&q, WarnPageUnknown, "The page this question came from was not given; the first page of its section is shown.", false)
	}

	// Options.
	labels := map[string]bool{}
	for i, ro := range rq.Options {
		if i >= maxOptions {
			warn(&q, WarnTextTooLong, fmt.Sprintf("More than %d options were found; the rest were left out. Check the original.", maxOptions), true)
			break
		}
		text, cut := limit(cleanText(ro.Text), maxOptionRunes)
		lbl := normaliseLabel(ro.Label)
		if text == "" && lbl == "" {
			continue
		}
		if cut {
			warn(&q, WarnTextTooLong, "An option was longer than allowed and was cut. Check the original.", true)
		}
		if labels[lbl] {
			warn(&q, WarnDuplicateOption, fmt.Sprintf("Two options are labelled %q.", lbl), false)
		}
		labels[lbl] = true
		q.Options = append(q.Options, Option{Label: lbl, Text: text})
	}

	// Type: as stated when valid, otherwise from the shape of the question.
	switch strings.ToUpper(strings.TrimSpace(rq.Type)) {
	case TypeMCQ:
		q.Type = TypeMCQ
	case TypeEssay:
		q.Type = TypeEssay
	default:
		if len(q.Options) >= 2 {
			q.Type = TypeMCQ
		} else {
			q.Type = TypeEssay
		}
		warn(&q, WarnTypeUncertain, "The question type was not clear and was chosen from the layout. Check it.", false)
	}
	if q.Type == TypeEssay && len(q.Options) >= 2 {
		warn(&q, WarnTypeUncertain, "This is marked as a written question but has options. It may be multiple choice, or its parts (a), (b) may have been read as options.", false)
	}

	// Tables.
	for _, t := range rq.Tables {
		md, _ := limit(cleanText(t.Markdown), maxTableRunes)
		if md == "" {
			continue
		}
		page := t.Page
		if !inBatch[page] {
			page = q.SourcePages[0]
		}
		q.Tables = append(q.Tables, Table{Page: page, Markdown: md})
	}

	// Diagrams. The model's description is only its reading of the figure and is
	// never treated as confirmed.
	for _, d := range rq.Diagrams {
		page := d.Page
		if !inBatch[page] {
			page = q.SourcePages[0]
		}
		dg := Diagram{Page: page}
		if d.Region != nil {
			dg.Region, _ = limit(cleanText(*d.Region), 200)
		}
		if d.Description != nil {
			dg.Description, _ = limit(cleanText(*d.Description), maxDescriptionRune)
		}
		if dg.Description != "" {
			dg.DescriptionSource = "MODEL"
		} else {
			warn(&q, WarnDiagramNeedsText, fmt.Sprintf("A diagram on page %d has no description. Add one so it can be read aloud.", page), false)
		}
		q.Diagrams = append(q.Diagrams, dg)
	}

	// Marks, when the source printed them.
	if rq.Marks != nil {
		if n, err := rq.Marks.Int64(); err == nil && n >= 0 && n <= 1000 {
			v := int(n)
			q.Points = &v
		}
	}

	if rq.Uncertain {
		q.Confidence = ConfidenceMedium
		warn(&q, WarnModelUncertain, "The reader was not sure about this question. Check it against the original.", false)
	}
	bq.Question = q
	return bq
}
