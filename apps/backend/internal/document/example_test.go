package document

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract/extracttest"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/pdf/pdftest"
)

// The representative paper. Run `UPDATE_GOLDEN=1 go test ./internal/document -run TestExample`
// to rewrite the fixture and the golden response after changing it on purpose.
//
//	testdata/sample_exam_paper.pdf       the paper (a PDF, built by pdftest, so it is reproducible)
//	testdata/model_reply_1.json, _2.json what a model is scripted to answer for each batch
//	testdata/draft_response.golden.json  what GET …/draft returns after extraction
func samplePaper() []byte {
	// The same running header and footer on every page, as a real paper has.
	running := func(n int) []pdftest.Text {
		return []pdftest.Text{
			pdftest.Line(200, 770, "Physics Midterm - Paper 1"),
			pdftest.Line(250, 30, fmt.Sprintf("Page %d of 3", n)),
		}
	}
	p1 := append(running(1),
		pdftest.Line(72, 730, "Answer ALL questions. Calculators may be used."),
		pdftest.Line(72, 700, "1. What is the SI unit of force?"),
		pdftest.Line(90, 684, "A. Newton"), pdftest.Line(90, 668, "B. Joule"), pdftest.Line(90, 652, "C. Watt"), pdftest.Line(90, 636, "D. Pascal"),
		pdftest.Line(72, 600, "2. A car accelerates uniformly from rest. Use the table to find its acceleration. [4 marks]"),
		pdftest.Line(90, 584, "Time (s) | Speed (m/s)"), pdftest.Line(90, 568, "0 | 0"), pdftest.Line(90, 552, "5 | 20"),
		pdftest.Line(72, 510, "3. Figure 1 shows a block of mass 2 kg on a rough slope inclined at 30 degrees."),
		pdftest.Line(90, 494, "Which option correctly describes the friction on the block?"),
	)
	p2 := append(running(2),
		pdftest.Line(90, 730, "A. Friction acts up the slope"), pdftest.Line(90, 714, "B. Friction acts down the slope"), pdftest.Line(90, 698, "C. There is no friction"),
		pdftest.Line(72, 650, "4. Explain Newton's first law of motion. [5 marks]"),
	)
	p3 := append(running(3),
		pdftest.Line(72, 700, "5. State two examples of a vector quantity and two examples of a scalar quantity."),
	)
	return pdftest.Build([]pdftest.Page{{Texts: p1}, {Texts: p2}, {Texts: p3}})
}

var (
	uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	timeRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)
)

// normalise replaces what changes from run to run (ids, times) with fixed text.
func normalise(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = normalise(e)
		}
		return x
	case []any:
		for i, e := range x {
			x[i] = normalise(e)
		}
		return x
	case string:
		switch {
		case uuidRe.MatchString(x):
			return "<uuid>"
		case timeRe.MatchString(x):
			return "<time>"
		}
	}
	return v
}

func golden(t *testing.T, path string, got []byte) {
	binary := strings.HasSuffix(path, ".pdf")
	t.Helper()
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.WriteFile(path, got, 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing fixture: run with UPDATE_GOLDEN=1")
	if !binary {
		// Text fixtures may be checked out with Windows line endings.
		want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	}
	assert.Equal(t, string(want), string(got), path)
}

func TestExample_RepresentativePaperEndToEnd(t *testing.T) {
	paper := samplePaper()
	golden(t, "testdata/sample_exam_paper.pdf", paper)

	r1, err := os.ReadFile("testdata/model_reply_1.json")
	require.NoError(t, err)
	r2, err := os.ReadFile("testdata/model_reply_2.json")
	require.NoError(t, err)

	a := newAPI(t)
	a.h.model.Replies = []extracttest.Reply{{Body: string(r1)}, {Body: string(r2)}}
	// A small request size puts page 1 in one call and pages 2 and 3 in another,
	// so question 3 runs across a batch boundary and has to be joined.
	a.h.extractOpts = extract.Options{MaxCharsPerRequest: 300}
	a.h.rebuild()

	up := a.upload(a.h.teacher, "Physics Midterm - Paper 1.pdf", paper)
	require.Equal(t, http.StatusAccepted, up.Code, up.Body.String())
	id := up.json()["id"].(string)
	require.NoError(t, a.h.svc.Process(context.Background(), uuid.MustParse(id), 1, false))
	require.Equal(t, 2, a.h.model.Calls(), "two batches")

	r := a.do(a.h.teacher, http.MethodGet, "/"+id+"/draft", nil, "")
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	var body any
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &body))
	var pretty bytes.Buffer
	enc := json.NewEncoder(&pretty)
	enc.SetEscapeHTML(false) // keep <uuid> and <time> readable
	enc.SetIndent("", "  ")
	require.NoError(t, enc.Encode(normalise(body)))
	golden(t, "testdata/draft_response.golden.json", pretty.Bytes())

	// The things the golden file is there to show, asserted so they cannot be lost.
	draft := body.(map[string]any)
	qs := draft["questions"].([]any)
	require.Len(t, qs, 5)
	q3 := qs[2].(map[string]any)
	assert.Len(t, q3["options"], 3, "question 3 was joined across the page break")
	assert.Equal(t, []any{float64(1), float64(2)}, q3["sourcePages"])
	assert.Len(t, q3["diagrams"], 1)
	assert.Equal(t, false, q3["diagrams"].([]any)[0].(map[string]any)["confirmed"], "a described diagram still needs a teacher's confirmation")
	assert.Nil(t, qs[0].(map[string]any)["points"], "marks the paper did not print are left for the teacher")
	assert.EqualValues(t, 4, qs[1].(map[string]any)["points"], "marks the paper printed are kept")
	assert.Equal(t, "Answer ALL questions. Calculators may be used.", draft["documentInstructions"])

	pages := draft["document"].(map[string]any)["pages"].([]any)
	removed := pages[0].(map[string]any)["removedLines"]
	assert.Contains(t, removed, "Physics Midterm - Paper 1", "running headers are recorded, not silently dropped")
}
