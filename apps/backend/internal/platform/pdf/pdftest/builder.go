// Package pdftest builds small PDFs for tests, so the suite needs no binary
// fixtures and no network. The files use the standard Helvetica font and raw
// gray images, which every PDF reader supports.
package pdftest

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Text is one piece of text drawn at a position (points, origin bottom left).
type Text struct {
	X, Y, Size float64
	S          string
}

// Image is an 8-bit gray picture drawn in a W x H box at X, Y.
type Image struct {
	X, Y, W, H float64
	PxW, PxH   int
}

// Page is one page of Text and Images.
type Page struct {
	Texts  []Text
	Images []Image
}

// Line returns a 12 point Text.
func Line(x, y float64, s string) Text { return Text{X: x, Y: y, Size: 12, S: s} }

func escape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)
	return r.Replace(s)
}

// Build writes a PDF of the pages (612 x 792 points each).
func Build(pages []Page) []byte {
	var buf bytes.Buffer
	var offsets []int
	add := func(body string) int {
		offsets = append(offsets, buf.Len())
		n := len(offsets)
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", n, body)
		return n
	}

	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	// Object numbers: 1 catalog, 2 pages, 3 font, then per page: page, content,
	// images. They are reserved up front so the page tree can refer to them.
	next := 4
	type ids struct {
		page, content int
		images        []int
	}
	all := make([]ids, len(pages))
	for i, p := range pages {
		all[i] = ids{page: next, content: next + 1}
		next += 2
		for range p.Images {
			all[i].images = append(all[i].images, next)
			next++
		}
	}

	kids := make([]string, len(pages))
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", all[i].page)
	}
	add("<< /Type /Catalog /Pages 2 0 R >>")
	add(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages)))
	add(fontObject())

	for i, p := range pages {
		var xobjs []string
		for j := range p.Images {
			xobjs = append(xobjs, fmt.Sprintf("/Im%d %d 0 R", j, all[i].images[j]))
		}
		res := "/Font << /F1 3 0 R >>"
		if len(xobjs) > 0 {
			res += " /XObject << " + strings.Join(xobjs, " ") + " >>"
		}
		add(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << %s >> /Contents %d 0 R >>", res, all[i].content))

		var c strings.Builder
		for _, t := range p.Texts {
			size := t.Size
			if size == 0 {
				size = 12
			}
			fmt.Fprintf(&c, "BT /F1 %.1f Tf %.1f %.1f Td (%s) Tj ET\n", size, t.X, t.Y, escape(t.S))
		}
		for j, im := range p.Images {
			fmt.Fprintf(&c, "q %.1f 0 0 %.1f %.1f %.1f cm /Im%d Do Q\n", im.W, im.H, im.X, im.Y, j)
		}
		add(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", c.Len(), c.String()))

		for _, im := range p.Images {
			px := make([]byte, im.PxW*im.PxH)
			for k := range px {
				px[k] = byte((k * 7) % 256)
			}
			add(fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8 /Length %d >>\nstream\n%s\nendstream",
				im.PxW, im.PxH, len(px), px))
		}
	}

	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return buf.Bytes()
}

// Encrypt returns the PDF protected with a user password.
func Encrypt(pdf []byte) ([]byte, error) {
	conf := model.NewAESConfiguration("user-pw", "owner-pw", 256)
	var out bytes.Buffer
	if err := api.Encrypt(bytes.NewReader(pdf), &out, conf); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// helveticaWidths are the Helvetica advance widths (1/1000 em) for characters
// 32 to 126, so a reader can place each glyph as it would in a real document.
var helveticaWidths = []int{
	278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278,
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 278, 278, 584, 584, 584, 556,
	1015, 667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778,
	667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 278, 278, 278, 469, 556,
	333, 556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556,
	556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, 334, 260, 334, 584,
}

func fontObject() string {
	ws := make([]string, len(helveticaWidths))
	for i, w := range helveticaWidths {
		ws[i] = fmt.Sprint(w)
	}
	return fmt.Sprintf("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 32 /LastChar 126 /Widths [%s] >>", strings.Join(ws, " "))
}
