package document

import (
	"fmt"
	"regexp"
	"strings"
)

// ComposeText turns a reviewed draft question into the single text a question
// has in the exam. A question has one text field, and the exam room reads that
// text aloud, so everything a candidate needs has to be in it, in the order a
// reader would meet it:
//
//	instructions
//	the question
//	each table, as rows of text
//	each diagram, as "Diagram (page N): description"
//
// Approval has already required every diagram to have a description that a
// teacher confirmed, so none is left for a candidate to miss.
func ComposeText(q DraftQuestion) string {
	var parts []string
	if s := strings.TrimSpace(q.Instructions); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(q.Text); s != "" {
		parts = append(parts, s)
	}
	for _, t := range q.Tables {
		if rows := tableToText(t.Markdown); rows != "" {
			parts = append(parts, fmt.Sprintf("Table (page %d):\n%s", t.Page, rows))
		}
	}
	for _, d := range q.Diagrams {
		if s := strings.TrimSpace(d.Description); s != "" {
			parts = append(parts, fmt.Sprintf("Diagram (page %d): %s", d.Page, s))
		}
	}
	return strings.Join(parts, "\n\n")
}

var tableSeparator = regexp.MustCompile(`^\s*\|?\s*:?-{1,}:?\s*(\|\s*:?-{1,}:?\s*)*\|?\s*$`)

// tableToText writes a Markdown table as one line per row, cells separated by
// " | ", without the divider row, so it reads sensibly aloud and on screen.
func tableToText(md string) string {
	var rows []string
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || tableSeparator.MatchString(line) {
			continue
		}
		line = strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|")
		cells := strings.Split(line, "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		rows = append(rows, strings.Join(cells, " | "))
	}
	return strings.Join(rows, "\n")
}
