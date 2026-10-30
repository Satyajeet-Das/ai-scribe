package extract

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// groundThreshold is how much of a piece of wording must be found on the page
// for it to count as read from it. Below this, the wording is flagged: it may
// have been invented, reworded or misread.
const groundThreshold = 0.8

// tokens splits text into lower-case words, with compatibility forms folded
// (so "x²" and "x2", or a ligature and its letters, compare equal) and
// line-end hyphenation joined. Punctuation and spacing are ignored, so a
// reflowed paragraph still matches the page it came from.
func tokens(s string) []string {
	s = strings.ReplaceAll(s, "-\n", "")
	s = norm.NFKC.String(s)
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// corpus is the searchable form of some page text.
type corpus struct {
	toks  []string
	grams map[string]bool
	words map[string]bool
}

func newCorpus(text string) corpus {
	c := corpus{toks: tokens(text), grams: map[string]bool{}, words: map[string]bool{}}
	for i, t := range c.toks {
		c.words[t] = true
		if i+2 < len(c.toks) {
			c.grams[t+" "+c.toks[i+1]+" "+c.toks[i+2]] = true
		}
	}
	return c
}

func (c corpus) empty() bool { return len(c.toks) == 0 }

// found reports the fraction (0..1) of s that appears in the corpus. Pieces of
// three words or more are compared by their word triples, which tolerates a
// small misread; shorter ones must appear as an exact run of words.
func (c corpus) found(s string) float64 {
	t := tokens(s)
	switch {
	case len(t) == 0:
		return 1
	case len(t) < 3:
		return b2f(c.contains(t))
	}
	hit, total := 0, 0
	for i := 0; i+2 < len(t); i++ {
		total++
		if c.grams[t[i]+" "+t[i+1]+" "+t[i+2]] {
			hit++
		}
	}
	return float64(hit) / float64(total)
}

func (c corpus) contains(t []string) bool {
outer:
	for i := 0; i+len(t) <= len(c.toks); i++ {
		for j := range t {
			if c.toks[i+j] != t[j] {
				continue outer
			}
		}
		return true
	}
	return false
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// ground checks a question's wording against the text layer of the pages it
// claims to come from, then against the rest of the batch. It adds warnings and
// lowers confidence; it never changes or removes wording. pageText holds the
// text layer by page number; a page with none cannot be checked and is the
// vision model's reading.
func ground(q *Question, pageText map[int]string, batchPages []int) {
	claimed := &strings.Builder{}
	for _, p := range q.SourcePages {
		claimed.WriteString(pageText[p])
		claimed.WriteByte('\n')
	}
	all := &strings.Builder{}
	for _, p := range batchPages {
		all.WriteString(pageText[p])
		all.WriteByte('\n')
	}
	mine, batch := newCorpus(claimed.String()), newCorpus(all.String())

	if mine.empty() && batch.empty() {
		// Nothing to compare with: the wording is a model's reading of a picture.
		warn(q, WarnReadFromImage, "This was read from a picture of the page. Check every word against the original.", false)
		capConfidence(q, ConfidenceMedium)
		return
	}

	parts := []struct{ name, text string }{{"the question text", q.Text}, {"the instructions", q.Instructions}}
	for _, o := range q.Options {
		parts = append(parts, struct{ name, text string }{fmt.Sprintf("option %s", o.Label), o.Text})
	}

	var unverified, elsewhere []string
	for _, p := range parts {
		if strings.TrimSpace(p.text) == "" {
			continue
		}
		switch {
		case mine.found(p.text) >= groundThreshold:
		case batch.found(p.text) >= groundThreshold:
			elsewhere = append(elsewhere, p.name)
		default:
			unverified = append(unverified, p.name)
		}
	}
	if len(elsewhere) > 0 {
		warn(q, WarnPageMismatch, fmt.Sprintf("The wording of %s was found, but not on the page given as the source.", strings.Join(elsewhere, ", ")), false)
	}
	if len(unverified) > 0 {
		warn(q, WarnUnverifiedText,
			fmt.Sprintf("The wording of %s could not be found on the page. It may have been reworded or misread. Compare it with the original.", strings.Join(unverified, ", ")), true)
		q.Confidence = ConfidenceLow
	}

	// Marks must be printed on the page; one that is not is dropped, not guessed.
	if q.Points != nil && !mine.words[fmt.Sprint(*q.Points)] && !batch.words[fmt.Sprint(*q.Points)] {
		q.Points = nil
		warn(q, WarnMarksNotOnPage, "Marks were reported for this question but are not on the page, so none are set.", false)
	}
}

func capConfidence(q *Question, max string) {
	rank := map[string]int{ConfidenceLow: 0, ConfidenceMedium: 1, ConfidenceHigh: 2}
	if rank[q.Confidence] > rank[max] {
		q.Confidence = max
	}
}
