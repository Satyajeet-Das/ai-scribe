package pdf

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// RemovedLine is a header or footer line taken out of the text sent for
// extraction, recorded so that nothing the pipeline sets aside is lost silently.
type RemovedLine struct {
	Page int    `json:"page"`
	Text string `json:"text"`
}

// StripRepeated removes running headers, footers and page numbers: lines at the
// top or bottom of a page that repeat, ignoring digits, on at least half the
// pages. It needs three or more pages to tell a running line from a one-off.
// The pages are changed in place; the removed lines are returned.
func StripRepeated(pages []PageText) []RemovedLine {
	n := len(pages)
	if n < 3 {
		return nil
	}
	const edge = 2 // lines looked at at the top and at the bottom
	counts := map[string]int{}
	for _, p := range pages {
		seen := map[string]bool{}
		for _, i := range edgeIndexes(p, edge) {
			if contentLine.MatchString(p.Lines[i]) {
				continue
			}
			k := normaliseLine(p.Lines[i])
			if k != "" && !seen[k] {
				seen[k] = true
				counts[k]++
			}
		}
	}
	need := (n + 1) / 2
	var removed []RemovedLine
	for pi := range pages {
		p := &pages[pi]
		drop := map[int]bool{}
		for _, i := range edgeIndexes(*p, edge) {
			if !contentLine.MatchString(p.Lines[i]) && counts[normaliseLine(p.Lines[i])] >= need {
				drop[i] = true
			}
		}
		if len(drop) == 0 {
			continue
		}
		kept := make([]string, 0, len(p.Lines))
		keptYs := make([]float64, 0, len(p.Lines))
		for i, l := range p.Lines {
			if drop[i] {
				removed = append(removed, RemovedLine{Page: p.Number, Text: l})
				continue
			}
			kept = append(kept, l)
			if len(p.ys) == len(p.Lines) {
				keptYs = append(keptYs, p.ys[i])
			}
		}
		p.Lines = kept
		if len(p.ys) == len(keptYs)+len(drop) {
			p.ys = keptYs
		}
	}
	return removed
}

// edgeIndexes returns the indexes of the page's `edge` highest and `edge`
// lowest lines. Height decides, not reading order: in a two-column page a footer
// that sits in one column falls between the columns in reading order.
func edgeIndexes(p PageText, edge int) []int {
	n := len(p.Lines)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	if len(p.ys) == n {
		sort.SliceStable(order, func(a, b int) bool { return p.ys[order[a]] > p.ys[order[b]] })
	}
	var out []int
	for i := 0; i < n; i++ {
		if i < edge || i >= n-edge {
			out = append(out, order[i])
		}
	}
	return out
}

// normaliseLine makes the lines "Physics Paper 1 - page 3" and
// "Physics Paper 1 - page 4" the same key.
func normaliseLine(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsDigit(r):
			if !strings.HasSuffix(b.String(), "#") {
				b.WriteByte('#')
			}
		case unicode.IsSpace(r):
			if !strings.HasSuffix(b.String(), " ") {
				b.WriteByte(' ')
			}
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// contentLine matches a line that starts a question ("3. Define...", "Q4)",
// "(2) ...") or an answer option ("A. Newton", "(b) Joule"). Such a line is
// never a running header or footer, even if it repeats apart from its digits.
var contentLine = regexp.MustCompile(`(?i)^\s*(?:q(?:uestion)?\s*)?\(?\d{1,3}\s*[.):]\s*\S|^\s*\(?[a-e]\s*[.)]\s+\S`)
