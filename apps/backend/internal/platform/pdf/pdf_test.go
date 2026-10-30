package pdf_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/pdf"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/pdf/pdftest"
)

var lim = pdf.Limits{MaxBytes: 5 << 20, MaxPages: 10, Timeout: 20 * time.Second}

func paper(pages int) []byte {
	var ps []pdftest.Page
	for i := 1; i <= pages; i++ {
		ps = append(ps, pdftest.Page{Texts: []pdftest.Text{
			pdftest.Line(72, 750, "Physics Paper 1"),
			pdftest.Line(72, 700, fmt.Sprintf("%d. What is the unit of force on page %d?", i, i)),
			pdftest.Line(90, 680, "A. Newton"),
			pdftest.Line(90, 665, "B. Joule"),
			pdftest.Line(300, 30, fmt.Sprintf("Page %d of %d", i, pages)),
		}})
	}
	return pdftest.Build(ps)
}

func TestInspect_ValidPDF(t *testing.T) {
	info, err := pdf.Inspect(context.Background(), paper(3), lim)
	require.NoError(t, err)
	assert.Equal(t, 3, info.PageCount)
}

func TestInspect_Rejections(t *testing.T) {
	ctx := context.Background()
	valid := paper(2)

	cases := []struct {
		name string
		data []byte
		lim  pdf.Limits
		want error
	}{
		{"empty", nil, lim, pdf.ErrNotPDF},
		{"plain text named .pdf", []byte("hello, not a pdf"), lim, pdf.ErrNotPDF},
		{"html", []byte("<html><body>hi</body></html>"), lim, pdf.ErrNotPDF},
		{"png", append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...), lim, pdf.ErrNotPDF},
		{"executable", append([]byte("MZ"), make([]byte, 128)...), lim, pdf.ErrNotPDF},
		{"leading junk before the header", append([]byte("junk"), valid...), lim, pdf.ErrNotPDF},
		{"truncated upload", valid[:len(valid)/2], lim, pdf.ErrMalformed},
		{"header only", []byte("%PDF-1.4\n%%EOF\n"), lim, pdf.ErrMalformed},
		{"garbage between header and trailer", []byte("%PDF-1.4\n" + strings.Repeat("\x00\xff", 400) + "\n%%EOF\n"), lim, pdf.ErrMalformed},
		{"too large", valid, pdf.Limits{MaxBytes: 100, MaxPages: 10}, pdf.ErrTooLarge},
		{"too many pages", paper(4), pdf.Limits{MaxBytes: 5 << 20, MaxPages: 3}, pdf.ErrTooManyPages},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pdf.Inspect(ctx, tc.data, tc.lim)
			assert.ErrorIs(t, err, tc.want)
		})
	}
}

func TestInspect_EncryptedIsRefused(t *testing.T) {
	enc, err := pdftest.Encrypt(paper(2))
	require.NoError(t, err)
	_, err = pdf.Inspect(context.Background(), enc, lim)
	assert.ErrorIs(t, err, pdf.ErrEncrypted)
}

func TestInspect_ParserErrorsDoNotLeakFileContent(t *testing.T) {
	_, err := pdf.Inspect(context.Background(), []byte("%PDF-1.4\nSECRET-EXAM-CONTENT\n%%EOF\n"), lim)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SECRET-EXAM-CONTENT")
}

func TestInspect_RespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := pdf.Inspect(ctx, paper(2), pdf.Limits{MaxBytes: 5 << 20, MaxPages: 10, Timeout: time.Nanosecond})
	assert.Error(t, err)
}

func TestGate_LimitsConcurrencyAndReleases(t *testing.T) {
	g := pdf.NewGate(1)
	release, err := g.Acquire(context.Background())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = g.Acquire(ctx)
	assert.ErrorIs(t, err, pdf.ErrBusy, "a second caller waits, and gives up when its context ends")

	release()
	r2, err := g.Acquire(context.Background())
	require.NoError(t, err)
	r2()
}

func TestExtractText_ReadingOrderAndKinds(t *testing.T) {
	data := pdftest.Build([]pdftest.Page{
		{Texts: []pdftest.Text{
			pdftest.Line(72, 700, "1. What is the unit of force?"),
			pdftest.Line(90, 680, "A. Newton"),
			pdftest.Line(90, 665, "B. Joule"),
		}},
		{Images: []pdftest.Image{{X: 50, Y: 50, W: 500, H: 700, PxW: 200, PxH: 280}}},
	})
	pages, err := pdf.ExtractText(context.Background(), data, pdf.TextLimits{Limits: lim})
	require.NoError(t, err)
	require.Len(t, pages, 2)

	assert.Equal(t, pdf.KindText, pages[0].Kind)
	assert.Equal(t, []string{"1. What is the unit of force?", "A. Newton", "B. Joule"}, pages[0].Lines)
	assert.Equal(t, 1, pages[0].Columns)

	assert.Equal(t, pdf.KindImageOnly, pages[1].Kind, "a page with no text layer is a scan")
	assert.Empty(t, pages[1].Lines)
}

func TestExtractText_TwoColumnsAreReadLeftThenRight(t *testing.T) {
	var texts []pdftest.Text
	texts = append(texts, pdftest.Line(150, 750, "Section A - Answer all questions"))
	left := []string{"1. Left column question one", "2. Left column question two", "3. Left column question three", "4. Left column question four"}
	right := []string{"5. Right column question five", "6. Right column question six", "7. Right column question seven", "8. Right column question eight"}
	for i := range left {
		y := 700 - float64(i)*20
		texts = append(texts, pdftest.Line(60, y, left[i]), pdftest.Line(340, y, right[i]))
	}
	pages, err := pdf.ExtractText(context.Background(), pdftest.Build([]pdftest.Page{{Texts: texts}}), pdf.TextLimits{Limits: lim})
	require.NoError(t, err)
	require.Len(t, pages, 1)

	want := append([]string{"Section A - Answer all questions"}, append(left, right...)...)
	assert.Equal(t, 2, pages[0].Columns)
	assert.Equal(t, want, pages[0].Lines, "left column first, then right, with the full-width title kept first")
}

func TestExtractText_OversizedPageIsCapped(t *testing.T) {
	var texts []pdftest.Text
	for i := 0; i < 60; i++ {
		texts = append(texts, pdftest.Line(60, 760-float64(i)*12, strings.Repeat("word ", 10)))
	}
	pages, err := pdf.ExtractText(context.Background(), pdftest.Build([]pdftest.Page{{Texts: texts}}),
		pdf.TextLimits{Limits: lim, MaxRunesPerPage: 200})
	require.NoError(t, err)
	assert.LessOrEqual(t, pages[0].Chars, 200)
}

func TestExtractText_RefusesWhatInspectRefuses(t *testing.T) {
	_, err := pdf.ExtractText(context.Background(), []byte("not a pdf"), pdf.TextLimits{Limits: lim})
	assert.ErrorIs(t, err, pdf.ErrNotPDF)
	_, err = pdf.ExtractText(context.Background(), paper(4), pdf.TextLimits{Limits: pdf.Limits{MaxBytes: 5 << 20, MaxPages: 2}})
	assert.ErrorIs(t, err, pdf.ErrTooManyPages)
}

func TestStripRepeated_RemovesHeadersFootersAndPageNumbers(t *testing.T) {
	pages, err := pdf.ExtractText(context.Background(), paper(4), pdf.TextLimits{Limits: lim})
	require.NoError(t, err)

	removed := pdf.StripRepeated(pages)

	for _, p := range pages {
		assert.NotContains(t, p.Text(), "Physics Paper 1", "running header removed from page %d", p.Number)
		assert.NotContains(t, p.Text(), " of 4", "page number removed from page %d", p.Number)
		assert.Contains(t, p.Text(), "What is the unit of force")
		assert.Contains(t, p.Text(), "A. Newton")
	}
	assert.Len(t, removed, 8, "two lines on each of four pages are recorded, not silently dropped")
	assert.Equal(t, "Physics Paper 1", removed[0].Text)
}

func TestStripRepeated_NeedsThreePages(t *testing.T) {
	pages, err := pdf.ExtractText(context.Background(), paper(2), pdf.TextLimits{Limits: lim})
	require.NoError(t, err)
	assert.Empty(t, pdf.StripRepeated(pages))
	assert.Contains(t, pages[0].Text(), "Physics Paper 1")
}

func TestExtractImages_ReturnsBigPicturesAndCapsThem(t *testing.T) {
	data := pdftest.Build([]pdftest.Page{
		{Texts: []pdftest.Text{pdftest.Line(72, 750, "Figure 1 shows a circuit")},
			Images: []pdftest.Image{{X: 100, Y: 300, W: 300, H: 300, PxW: 200, PxH: 200}}},
		{Texts: []pdftest.Text{pdftest.Line(72, 750, "No picture here")}},
	})
	imgs, skipped, err := pdf.ExtractImages(context.Background(), data, []int{1, 2}, pdf.ImageLimits{Limits: lim})
	require.NoError(t, err)
	assert.Empty(t, skipped)
	require.Len(t, imgs[1], 1)
	assert.Equal(t, 200, imgs[1][0].Width)
	assert.Contains(t, []string{"png", "jpg"}, imgs[1][0].Format)
	assert.NotEmpty(t, imgs[1][0].Data)
	assert.Empty(t, imgs[2])

	// A picture bigger than the cap is not returned, and is counted as skipped.
	_, skipped, err = pdf.ExtractImages(context.Background(), data, []int{1}, pdf.ImageLimits{Limits: lim, MaxImageBytes: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, skipped[1])
}

func TestExtractImages_RefusesNonPDF(t *testing.T) {
	_, _, err := pdf.ExtractImages(context.Background(), bytes.Repeat([]byte("x"), 100), []int{1}, pdf.ImageLimits{Limits: lim})
	assert.ErrorIs(t, err, pdf.ErrNotPDF)
}

func TestStripRepeated_FindsFootersByHeightEvenInATwoColumnPage(t *testing.T) {
	var ps []pdftest.Page
	for i := 1; i <= 3; i++ {
		var texts []pdftest.Text
		// A header and a footer that sit entirely in the left half of the page.
		texts = append(texts, pdftest.Line(60, 760, "Physics Paper 1"), pdftest.Line(60, 30, fmt.Sprintf("Page %d of 3", i)))
		for j := 0; j < 4; j++ {
			y := 700 - float64(j)*22
			texts = append(texts,
				pdftest.Line(60, y, fmt.Sprintf("%d. Left column question %d on page %d", j+1, j+1, i)),
				pdftest.Line(340, y, fmt.Sprintf("%d. Right column question %d on page %d", j+5, j+5, i)))
		}
		ps = append(ps, pdftest.Page{Texts: texts})
	}
	pages, err := pdf.ExtractText(context.Background(), pdftest.Build(ps), pdf.TextLimits{Limits: lim})
	require.NoError(t, err)
	require.Equal(t, 2, pages[0].Columns)

	removed := pdf.StripRepeated(pages)

	assert.Len(t, removed, 6, "a header and a footer on each of three pages")
	for _, p := range pages {
		assert.NotContains(t, p.Text(), "Page ")
		assert.NotContains(t, p.Text(), "Physics Paper 1")
		assert.Contains(t, p.Text(), "Left column question 4")
		assert.Contains(t, p.Text(), "Right column question 8")
	}
}

func TestExtractImages_FilesPicturesUnderTheirOwnPageWhateverTheOrder(t *testing.T) {
	var ps []pdftest.Page
	for i := 1; i <= 7; i++ { // more pages than one pass reads, with pictures on some
		p := pdftest.Page{Texts: []pdftest.Text{pdftest.Line(72, 750, fmt.Sprintf("page %d", i))}}
		if i == 2 || i == 6 {
			p.Images = []pdftest.Image{{X: 50, Y: 100, W: 300, H: 300, PxW: 180 + i, PxH: 180 + i}}
		}
		ps = append(ps, p)
	}
	for run := 0; run < 5; run++ { // the parser's page order varies from run to run
		imgs, _, err := pdf.ExtractImages(context.Background(), pdftest.Build(ps), []int{1, 2, 3, 4, 5, 6, 7}, pdf.ImageLimits{Limits: lim})
		require.NoError(t, err)
		require.Len(t, imgs, 2, "only the pages that have pictures")
		require.Len(t, imgs[2], 1)
		require.Len(t, imgs[6], 1)
		assert.Equal(t, 182, imgs[2][0].Width)
		assert.Equal(t, 186, imgs[6][0].Width)
	}
}

func TestExtractText_AOneColumnPageIsNeverSplitIntoTwo(t *testing.T) {
	// Short indented option lines, a long question line and a header and footer
	// that reach further right than the options: the ragged right edge of the
	// short lines leaves an empty strip, but no line has text on both sides of it.
	pages := []pdftest.Page{{Texts: []pdftest.Text{
		pdftest.Line(200, 770, "Physics Midterm - Paper 1"),
		pdftest.Line(90, 730, "A. Friction acts up the slope"),
		pdftest.Line(90, 714, "B. Friction acts down the slope"),
		pdftest.Line(90, 698, "C. There is no friction"),
		pdftest.Line(72, 650, "4. Explain Newton's first law of motion. [5 marks]"),
		pdftest.Line(250, 30, "Page 2 of 3"),
	}}}
	got, err := pdf.ExtractText(context.Background(), pdftest.Build(pages), pdf.TextLimits{Limits: lim})
	require.NoError(t, err)
	assert.Equal(t, 1, got[0].Columns, "a one-column page stays one column")
	assert.Equal(t, []string{
		"Physics Midterm - Paper 1",
		"A. Friction acts up the slope",
		"B. Friction acts down the slope",
		"C. There is no friction",
		"4. Explain Newton's first law of motion. [5 marks]",
		"Page 2 of 3",
	}, got[0].Lines, "every line is whole and in order")
}

func TestExtractText_ALongOneColumnParagraphIsNotSplit(t *testing.T) {
	var texts []pdftest.Text
	for i := 0; i < 12; i++ {
		texts = append(texts, pdftest.Line(60, 740-float64(i)*16, fmt.Sprintf("Line %d of a long paragraph that runs the full width of the page without any columns", i+1)))
	}
	got, err := pdf.ExtractText(context.Background(), pdftest.Build([]pdftest.Page{{Texts: texts}}), pdf.TextLimits{Limits: lim})
	require.NoError(t, err)
	assert.Equal(t, 1, got[0].Columns)
	require.Len(t, got[0].Lines, 12)
	assert.Equal(t, "Line 1 of a long paragraph that runs the full width of the page without any columns", got[0].Lines[0])
}

func TestExtractText_TwoColumnsNeedSeveralRowsWithTextOnBothSides(t *testing.T) {
	// Two rows of two short columns are not enough evidence of a layout.
	texts := []pdftest.Text{
		pdftest.Line(60, 700, "1. Left first question text"), pdftest.Line(340, 700, "3. Right first question text"),
		pdftest.Line(60, 680, "2. Left second question text"), pdftest.Line(340, 680, "4. Right second question text"),
	}
	got, err := pdf.ExtractText(context.Background(), pdftest.Build([]pdftest.Page{{Texts: texts}}), pdf.TextLimits{Limits: lim})
	require.NoError(t, err)
	assert.Equal(t, 1, got[0].Columns)
	assert.Equal(t, "1. Left first question text 3. Right first question text", got[0].Lines[0],
		"read row by row, which keeps each row whole rather than guessing")
}
