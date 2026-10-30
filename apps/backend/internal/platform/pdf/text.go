package pdf

import (
	"bytes"
	"context"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	lpdf "github.com/ledongthuc/pdf"
)

// PageKind says what a page offers a text reader.
type PageKind string

const (
	// KindText has a usable text layer.
	KindText PageKind = "TEXT"
	// KindImageOnly has no usable text: a scan, or text drawn as pictures.
	KindImageOnly PageKind = "IMAGE_ONLY"
	// KindMixed has a text layer and large pictures (diagrams, figures).
	KindMixed PageKind = "MIXED"
)

// gutterHalf is half the width, in points, of the strip that must be empty for
// a page to count as two columns.
const gutterHalf = 6.0

// minTextChars is how much text a page needs to count as having a text layer.
const minTextChars = 20

// PageText is one page's text in reading order.
type PageText struct {
	Number int
	Kind   PageKind
	// Lines are the page's lines in reading order (columns left to right, each
	// top to bottom).
	Lines []string
	// ys is each line's height on the page (larger is higher), parallel to Lines.
	// Headers and footers are found by height, not by reading order, because in a
	// two-column page a footer can fall between the columns in reading order.
	ys []float64
	// Columns is 2 when a gutter was found, otherwise 1.
	Columns int
	Chars   int
}

// Text returns the page's lines joined by newlines.
func (p PageText) Text() string { return strings.Join(p.Lines, "\n") }

// TextLimits bound how much text is read.
type TextLimits struct {
	Limits
	// MaxRunesPerPage caps a page's text; the excess is dropped and reported by
	// Truncated.
	MaxRunesPerPage int
}

// ExtractText reads the text layer of every page.
func ExtractText(ctx context.Context, data []byte, lim TextLimits) ([]PageText, error) {
	if err := CheckSignature(data, lim.MaxBytes); err != nil {
		return nil, err
	}
	maxRunes := lim.MaxRunesPerPage
	if maxRunes <= 0 {
		maxRunes = 20000
	}
	return run(ctx, lim.timeout(), func() ([]PageText, error) {
		r, err := lpdf.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, classify(err)
		}
		n := r.NumPage()
		if n <= 0 {
			return nil, ErrNoPages
		}
		if lim.MaxPages > 0 && n > lim.MaxPages {
			return nil, ErrTooManyPages
		}
		pages := make([]PageText, 0, n)
		for i := 1; i <= n; i++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			var runs []run1
			// A page the parser cannot read counts as having no text layer; the
			// pipeline then treats it as an image page and says so.
			func() {
				defer func() { _ = recover() }()
				c := r.Page(i).Content()
				for _, t := range c.Text {
					if t.S == "" {
						continue
					}
					w := t.W
					if w <= 0 {
						w = float64(utf8.RuneCountInString(t.S)) * t.FontSize * 0.5
					}
					runs = append(runs, run1{x0: t.X, x1: t.X + w, y: t.Y, size: t.FontSize, s: t.S})
				}
			}()
			laid, cols := layoutRuns(runs)
			laid, _ = capRunes(laid, maxRunes)
			lines := make([]string, len(laid))
			ys := make([]float64, len(laid))
			for k, l := range laid {
				lines[k], ys[k] = l.s, l.y
			}
			chars := 0
			for _, l := range lines {
				chars += utf8.RuneCountInString(l)
			}
			kind := KindText
			if chars < minTextChars {
				kind = KindImageOnly
			}
			pages = append(pages, PageText{Number: i, Kind: kind, Lines: lines, ys: ys, Columns: cols, Chars: chars})
		}
		return pages, nil
	})
}

// run1 is one drawn piece of text. (Named to avoid clashing with run().)
type run1 struct {
	x0, x1, y, size float64
	s               string
}

// line is one line of a page with its height.
type line struct {
	s string
	y float64
}

func capRunes(lines []line, max int) ([]line, bool) {
	total := 0
	for i, l := range lines {
		total += utf8.RuneCountInString(l.s)
		if total > max {
			return lines[:i], true
		}
	}
	return lines, false
}

// row is the pieces of text that sit on one baseline.
type row []run1

// clusterRows groups pieces into rows, top of the page first (PDF y grows upwards).
func clusterRows(runs []run1) []row {
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].y > runs[j].y })
	var rows []row
	for _, r := range runs {
		tol := math.Max(2, 0.4*r.size)
		if n := len(rows); n > 0 && math.Abs(rows[n-1][0].y-r.y) <= tol {
			rows[n-1] = append(rows[n-1], r)
		} else {
			rows = append(rows, row{r})
		}
	}
	return rows
}

// layoutRuns puts a page's text pieces into reading order. Pieces on one
// baseline become a line. When a gutter runs down the page (a two-column
// layout), each stretch of two-column lines is read left column first; lines
// that cross the gutter (titles, instructions) stay where they are.
func layoutRuns(runs []run1) ([]line, int) {
	if len(runs) == 0 {
		return nil, 1
	}
	rows := clusterRows(runs)
	gutter, ok := findGutter(rows)

	if !ok {
		out := make([]line, 0, len(rows))
		for _, rw := range rows {
			if s := joinRow(rw); s != "" {
				out = append(out, line{s: s, y: rw[0].y})
			}
		}
		return out, 1
	}

	var out []line
	var left, right []line
	flush := func() {
		out = append(out, left...)
		out = append(out, right...)
		left, right = nil, nil
	}
	for _, rw := range rows {
		var l, r []run1
		crosses := false
		for _, x := range rw {
			switch {
			case isInk(x) && x.x0 < gutter+gutterHalf && x.x1 > gutter-gutterHalf:
				crosses = true
			case (x.x0+x.x1)/2 < gutter:
				l = append(l, x)
			default:
				r = append(r, x)
			}
		}
		y := rw[0].y
		if crosses {
			flush()
			if s := joinRow(rw); s != "" {
				out = append(out, line{s: s, y: y})
			}
			continue
		}
		if s := joinRow(l); s != "" {
			left = append(left, line{s: s, y: y})
		}
		if s := joinRow(r); s != "" {
			right = append(right, line{s: s, y: y})
		}
	}
	flush()
	return out, 2
}

func isInk(r run1) bool { return strings.TrimSpace(r.s) != "" }

// Thresholds for calling a page two-column. They are deliberately strict: taking
// a one-column page for two columns splits every line in half and scrambles the
// text, while missing a real two-column page only reads the columns line by line.
const (
	minColumnRows = 3   // rows with text on both sides of the gutter
	minColumnFrac = 0.5 // ...as a share of the rows that have text
)

// findGutter looks for a vertical strip, near the middle of the text, that no
// text crosses, and checks it against the rows: several rows must have text on
// both sides of it, and those must be most of the page. A strip that merely
// happens to be empty on a one-column page (the ragged ends of short lines) does
// not pass, because no row has text on both sides of it.
func findGutter(rows []row) (float64, bool) {
	var ink []run1
	for _, rw := range rows {
		for _, r := range rw {
			if isInk(r) {
				ink = append(ink, r)
			}
		}
	}
	if len(ink) < 8 {
		return 0, false
	}
	minX, maxX := math.Inf(1), math.Inf(-1)
	for _, r := range ink {
		minX = math.Min(minX, r.x0)
		maxX = math.Max(maxX, r.x1)
	}
	width := maxX - minX
	if width < 200 {
		return 0, false
	}

	// Count the pieces that cross a strip centred at each candidate position.
	const step = 2.0
	lo, hi := minX+0.35*width, minX+0.65*width
	var xs []float64
	var cross []int
	best := math.MaxInt
	for x := lo; x <= hi; x += step {
		c := 0
		for _, r := range ink {
			if r.x0 < x+gutterHalf && r.x1 > x-gutterHalf {
				c++
			}
		}
		xs, cross = append(xs, x), append(cross, c)
		if c < best {
			best = c
		}
	}
	if float64(best) > 0.1*float64(len(ink)) {
		return 0, false
	}
	// The middle of the longest stretch of positions that are equally empty.
	bestStart, bestLen, curStart, curLen := 0, 0, 0, 0
	for i, c := range cross {
		if c == best {
			if curLen == 0 {
				curStart = i
			}
			curLen++
			if curLen > bestLen {
				bestStart, bestLen = curStart, curLen
			}
		} else {
			curLen = 0
		}
	}
	gutter := xs[bestStart+bestLen/2]

	// Confirm against whole rows.
	both, withText := 0, 0
	for _, rw := range rows {
		var hasL, hasR, crosses bool
		for _, r := range rw {
			if !isInk(r) {
				continue
			}
			switch {
			case r.x0 < gutter+gutterHalf && r.x1 > gutter-gutterHalf:
				crosses = true
			case (r.x0+r.x1)/2 < gutter:
				hasL = true
			default:
				hasR = true
			}
		}
		if hasL || hasR || crosses {
			withText++
		}
		if hasL && hasR && !crosses {
			both++
		}
	}
	if both < minColumnRows || float64(both) < minColumnFrac*float64(withText) {
		return 0, false
	}
	return gutter, true
}

// joinRow joins pieces on one baseline, left to right, adding a space where
// the drawn gap is wider than a narrow space.
func joinRow(rs []run1) string {
	if len(rs) == 0 {
		return ""
	}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].x0 < rs[j].x0 })
	var b strings.Builder
	for i, r := range rs {
		if i > 0 {
			prev := rs[i-1]
			gap := r.x0 - prev.x1
			cur := b.String()
			if gap > 0.2*math.Max(r.size, 1) && !strings.HasSuffix(cur, " ") && !strings.HasPrefix(r.s, " ") {
				b.WriteByte(' ')
			}
		}
		b.WriteString(r.s)
	}
	return strings.TrimSpace(b.String())
}
