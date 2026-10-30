package document

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract/extracttest"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/pdf/pdftest"
)

// api is the real routes, handler and service on the in-memory fakes.
type api struct {
	t *testing.T
	h *harness
	e *echo.Echo
}

func newAPI(t *testing.T, mutate ...func(*Options)) *api {
	t.Helper()
	h := newHarness(t, mutate...)
	e := echo.New()
	// The same shape as the real error handler: an HTTPError becomes its status
	// and a JSON body with code, message and field errors.
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		var he *httperrs.HTTPError
		if errors.As(err, &he) {
			_ = c.JSON(he.Status, he)
			return
		}
		var ee *echo.HTTPError
		if errors.As(err, &ee) {
			_ = c.JSON(ee.Code, map[string]any{"code": "ECHO", "message": ee.Message})
			return
		}
		_ = c.JSON(500, map[string]any{"code": "INTERNAL_SERVER_ERROR"})
	}
	auth := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_id", c.Request().Header.Get("X-User"))
			c.Set("user_role", c.Request().Header.Get("X-Role"))
			return next(c)
		}
	}
	NewHandler(h.svc, h.opts.MaxBytes).RegisterRoutes(e.Group("/api/v1"), auth, nil, nil)
	return &api{t: t, h: h, e: e}
}

type resp struct {
	t *testing.T
	*httptest.ResponseRecorder
}

func (r resp) json() map[string]any {
	var m map[string]any
	require.NoError(r.t, json.Unmarshal(r.Body.Bytes(), &m), r.Body.String())
	return m
}

func (a *api) do(c Caller, method, path string, body io.Reader, contentType string) resp {
	a.t.Helper()
	req := httptest.NewRequest(method, "/api/v1/exams/"+a.h.examID.String()+"/documents"+path, body)
	if contentType != "" {
		req.Header.Set(echo.HeaderContentType, contentType)
	}
	req.Header.Set("X-User", c.ID.String())
	req.Header.Set("X-Role", string(c.Role))
	rec := httptest.NewRecorder()
	a.e.ServeHTTP(rec, req)
	return resp{t: a.t, ResponseRecorder: rec}
}

func (a *api) jsonReq(c Caller, method, path string, v any) resp {
	a.t.Helper()
	var buf bytes.Buffer
	if v != nil {
		require.NoError(a.t, json.NewEncoder(&buf).Encode(v))
	}
	return a.do(c, method, path, &buf, echo.MIMEApplicationJSON)
}

func multipartBody(t *testing.T, field, filename string, data []byte, extra map[string]string) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range extra {
		require.NoError(t, w.WriteField(k, v))
	}
	if field != "" {
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, filename))
		hdr.Set("Content-Type", "application/pdf")
		part, err := w.CreatePart(hdr)
		require.NoError(t, err)
		_, err = part.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}

func (a *api) upload(c Caller, filename string, data []byte) resp {
	a.t.Helper()
	body, ct := multipartBody(a.t, "file", filename, data, map[string]string{"note": "ignored"})
	return a.do(c, http.MethodPost, "", body, ct)
}

func TestAPI_UploadAcceptsAPDFAndReturnsItsStatus(t *testing.T) {
	a := newAPI(t)
	r := a.upload(a.h.teacher, "Physics.pdf", samplePDF())

	require.Equal(t, http.StatusAccepted, r.Code, r.Body.String())
	assert.Equal(t, "no-store", r.Header().Get("Cache-Control"))
	body := r.json()
	assert.Equal(t, "QUEUED", body["status"])
	assert.Equal(t, "Physics.pdf", body["filename"])
	assert.EqualValues(t, 1, body["pageCount"])
	assert.EqualValues(t, 1, body["attempt"])
	assert.NotEmpty(t, body["id"])
	assert.NotContains(t, r.Body.String(), "storageKey", "the storage key is internal")
	assert.NotContains(t, r.Body.String(), "sha256")
	assert.Len(t, a.h.queue.all(), 1)
}

func TestAPI_UploadRejections(t *testing.T) {
	enc, err := pdftest.Encrypt(samplePDF())
	require.NoError(t, err)

	t.Run("not multipart", func(t *testing.T) {
		a := newAPI(t)
		r := a.do(a.h.teacher, http.MethodPost, "", bytes.NewReader(samplePDF()), "application/pdf")
		assert.Equal(t, http.StatusUnsupportedMediaType, r.Code)
		assert.Equal(t, "UNSUPPORTED_MEDIA_TYPE", r.json()["code"])
	})
	t.Run("no file part", func(t *testing.T) {
		a := newAPI(t)
		body, ct := multipartBody(t, "", "", nil, map[string]string{"note": "hello"})
		r := a.do(a.h.teacher, http.MethodPost, "", body, ct)
		assert.Equal(t, http.StatusBadRequest, r.Code)
		assert.Equal(t, "VALIDATION_ERROR", r.json()["code"])
	})
	t.Run("the file is under another field name", func(t *testing.T) {
		a := newAPI(t)
		body, ct := multipartBody(t, "document", "p.pdf", samplePDF(), nil)
		assert.Equal(t, http.StatusBadRequest, a.do(a.h.teacher, http.MethodPost, "", body, ct).Code)
	})
	t.Run("not a pdf, whatever its name", func(t *testing.T) {
		a := newAPI(t)
		r := a.upload(a.h.teacher, "exam.pdf", []byte("<html>hello</html>"))
		assert.Equal(t, http.StatusUnsupportedMediaType, r.Code)
		assert.Equal(t, "UNSUPPORTED_MEDIA_TYPE", r.json()["code"])
	})
	t.Run("damaged", func(t *testing.T) {
		a := newAPI(t)
		r := a.upload(a.h.teacher, "exam.pdf", samplePDF()[:300])
		assert.Equal(t, http.StatusUnprocessableEntity, r.Code)
		assert.Equal(t, "INVALID_PDF", r.json()["code"])
	})
	t.Run("password protected", func(t *testing.T) {
		a := newAPI(t)
		r := a.upload(a.h.teacher, "exam.pdf", enc)
		assert.Equal(t, http.StatusUnprocessableEntity, r.Code)
		assert.Contains(t, r.json()["message"], "password")
	})
	t.Run("over the size limit, cut off before the end", func(t *testing.T) {
		a := newAPI(t, func(o *Options) { o.MaxBytes = 1 << 20 })
		big := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), 4<<20)...)
		r := a.upload(a.h.teacher, "big.pdf", big)
		assert.Equal(t, http.StatusRequestEntityTooLarge, r.Code)
		assert.Equal(t, "PAYLOAD_TOO_LARGE", r.json()["code"])
		assert.Empty(t, a.h.store.Keys())
	})
	t.Run("the same file twice", func(t *testing.T) {
		a := newAPI(t)
		first := a.upload(a.h.teacher, "a.pdf", samplePDF())
		require.Equal(t, http.StatusAccepted, first.Code)
		r := a.upload(a.h.teacher, "b.pdf", samplePDF())
		assert.Equal(t, http.StatusConflict, r.Code)
		body := r.json()
		assert.Equal(t, "DOCUMENT_ALREADY_UPLOADED", body["code"])
		fields := body["errors"].([]any)[0].(map[string]any)
		assert.Equal(t, first.json()["id"], fields["error"], "the response names the upload it duplicates")
	})
	t.Run("a published exam", func(t *testing.T) {
		a := newAPI(t)
		a.h.setExamStatus(a.h.examID, exam.StatusPublished)
		r := a.upload(a.h.teacher, "a.pdf", samplePDF())
		assert.Equal(t, http.StatusConflict, r.Code)
		assert.Equal(t, "EXAM_NOT_DRAFT", r.json()["code"])
	})
	t.Run("extraction not configured", func(t *testing.T) {
		a := newAPI(t, func(o *Options) { o.Active = false })
		r := a.upload(a.h.teacher, "a.pdf", samplePDF())
		assert.Equal(t, http.StatusServiceUnavailable, r.Code)
		assert.Equal(t, "EXTRACTION_UNAVAILABLE", r.json()["code"])
	})
	t.Run("queue down", func(t *testing.T) {
		a := newAPI(t)
		a.h.queue.err = errors.New("redis down")
		r := a.upload(a.h.teacher, "a.pdf", samplePDF())
		assert.Equal(t, http.StatusServiceUnavailable, r.Code)
		assert.Equal(t, "QUEUE_UNAVAILABLE", r.json()["code"])
	})
}

func TestAPI_Authorization(t *testing.T) {
	a := newAPI(t)
	d := a.h.processed()
	id := d.ID.String()

	calls := map[string]func(c Caller) resp{
		"upload": func(c Caller) resp { return a.upload(c, "x.pdf", samplePDF()) },
		"list":   func(c Caller) resp { return a.do(c, http.MethodGet, "", nil, "") },
		"get":    func(c Caller) resp { return a.do(c, http.MethodGet, "/"+id, nil, "") },
		"file":   func(c Caller) resp { return a.do(c, http.MethodGet, "/"+id+"/file", nil, "") },
		"draft":  func(c Caller) resp { return a.do(c, http.MethodGet, "/"+id+"/draft", nil, "") },
		"cancel": func(c Caller) resp { return a.do(c, http.MethodPost, "/"+id+"/cancel", nil, "") },
		"delete": func(c Caller) resp { return a.do(c, http.MethodDelete, "/"+id, nil, "") },
		"approve": func(c Caller) resp {
			return a.jsonReq(c, http.MethodPost, "/"+id+"/approve", map[string]any{"expectedDraftVersion": 1})
		},
		"retry": func(c Caller) resp { return a.jsonReq(c, http.MethodPost, "/"+id+"/retry", map[string]any{}) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, http.StatusNotFound, call(a.h.other).Code, "another teacher is told it does not exist")
			assert.Equal(t, http.StatusForbidden, call(a.h.student).Code, "a student is refused")
		})
	}
	assert.Equal(t, http.StatusOK, a.do(a.h.admin, http.MethodGet, "/"+id, nil, "").Code)
	assert.Equal(t, StatusReviewRequired, a.h.doc(d.ID).Status, "nothing the refused callers did changed the document")
}

func TestAPI_ReadingDocumentsAndFiles(t *testing.T) {
	a := newAPI(t)
	data := samplePDF()
	up := a.upload(a.h.teacher, "Résumé of physics.pdf", data)
	require.Equal(t, http.StatusAccepted, up.Code)
	id := up.json()["id"].(string)

	list := a.do(a.h.teacher, http.MethodGet, "", nil, "")
	require.Equal(t, http.StatusOK, list.Code)
	assert.EqualValues(t, 1, list.json()["total"])

	get := a.do(a.h.teacher, http.MethodGet, "/"+id, nil, "")
	require.Equal(t, http.StatusOK, get.Code)
	body := get.json()
	assert.Equal(t, "QUEUED", body["status"])
	assert.Equal(t, map[string]any{"done": float64(0), "total": float64(1)}, body["progress"])

	file := a.do(a.h.teacher, http.MethodGet, "/"+id+"/file", nil, "")
	require.Equal(t, http.StatusOK, file.Code)
	assert.Equal(t, data, file.Body.Bytes())
	assert.Equal(t, "application/pdf", file.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", file.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "no-store", file.Header().Get("Cache-Control"))
	assert.True(t, strings.HasPrefix(file.Header().Get("Content-Disposition"), "attachment;"), "a download, never shown inline")
	assert.Equal(t, fmt.Sprint(len(data)), file.Header().Get("Content-Length"))

	assert.Equal(t, http.StatusBadRequest, a.do(a.h.teacher, http.MethodGet, "/not-a-uuid", nil, "").Code)
	assert.Equal(t, http.StatusNotFound, a.do(a.h.teacher, http.MethodGet, "/"+uuid.NewString(), nil, "").Code)
}

func TestAPI_ReviewAndApproveEndToEnd(t *testing.T) {
	a := newAPI(t)
	d := a.h.processed()
	base := "/" + d.ID.String()

	// The draft: arrays are arrays (never null), nothing is marked correct.
	r := a.do(a.h.teacher, http.MethodGet, base+"/draft", nil, "")
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	draft := r.json()
	assert.EqualValues(t, 1, draft["draftVersion"])
	qs := draft["questions"].([]any)
	require.Len(t, qs, 2)
	q1 := qs[0].(map[string]any)
	assert.Equal(t, "MCQ", q1["type"])
	assert.Equal(t, "EXTRACTED", q1["origin"])
	assert.Nil(t, q1["points"])
	assert.IsType(t, []any{}, q1["tables"])
	assert.IsType(t, []any{}, q1["diagrams"])
	assert.IsType(t, []any{}, q1["warnings"])
	assert.IsType(t, []any{}, draft["unassigned"])
	for _, o := range q1["options"].([]any) {
		assert.Equal(t, false, o.(map[string]any)["isCorrect"])
	}

	// Approving now says what is missing.
	r = a.jsonReq(a.h.teacher, http.MethodPost, base+"/approve", map[string]any{"expectedDraftVersion": 1})
	require.Equal(t, http.StatusConflict, r.Code)
	assert.Equal(t, "DRAFT_INCOMPLETE", r.json()["code"])
	errs := r.json()["errors"].([]any)
	assert.GreaterOrEqual(t, len(errs), 2)
	assert.Contains(t, errs[0].(map[string]any)["field"], "questions[1].")

	// Editing: a stale version, an invalid edit, a missing version, then a good edit.
	qid := q1["id"].(string)
	edit := map[string]any{"question": map[string]any{
		"numberLabel": "1", "type": "MCQ", "text": q1Text, "points": 2,
		"options": []map[string]any{{"label": "A", "text": "Newton", "isCorrect": true}, {"label": "B", "text": "Joule"}, {"label": "C", "text": "Watt"}},
	}}
	edit["expectedDraftVersion"] = 0
	assert.Equal(t, http.StatusConflict, a.jsonReq(a.h.teacher, http.MethodPut, base+"/draft/questions/"+qid, edit).Code)
	assert.Equal(t, "STALE_VERSION", a.jsonReq(a.h.teacher, http.MethodPut, base+"/draft/questions/"+qid, edit).json()["code"])

	delete(edit, "expectedDraftVersion")
	assert.Equal(t, http.StatusBadRequest, a.jsonReq(a.h.teacher, http.MethodPut, base+"/draft/questions/"+qid, edit).Code, "the version is required")

	edit["expectedDraftVersion"] = 1
	edit["question"].(map[string]any)["points"] = 5000
	bad := a.jsonReq(a.h.teacher, http.MethodPut, base+"/draft/questions/"+qid, edit)
	require.Equal(t, http.StatusBadRequest, bad.Code)
	assert.Equal(t, "points", bad.json()["errors"].([]any)[0].(map[string]any)["field"])

	edit["question"].(map[string]any)["points"] = 2
	ok := a.jsonReq(a.h.teacher, http.MethodPut, base+"/draft/questions/"+qid, edit)
	require.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
	assert.EqualValues(t, 2, ok.json()["draftVersion"])
	assert.Equal(t, true, ok.json()["question"].(map[string]any)["edited"])

	// Adding, reordering and deleting.
	add := a.jsonReq(a.h.teacher, http.MethodPost, base+"/draft/questions", map[string]any{
		"expectedDraftVersion": 2, "position": 1,
		"question": map[string]any{"type": "ESSAY", "text": "Describe the motion of a pendulum.", "points": 4},
	})
	require.Equal(t, http.StatusCreated, add.Code, add.Body.String())
	newID := add.json()["question"].(map[string]any)["id"].(string)

	order := a.do(a.h.teacher, http.MethodGet, base+"/draft", nil, "").json()["questions"].([]any)
	ids := []string{}
	for _, q := range order {
		ids = append(ids, q.(map[string]any)["id"].(string))
	}
	require.Equal(t, newID, ids[0], "inserted first, as asked")
	reorder := a.jsonReq(a.h.teacher, http.MethodPut, base+"/draft/reorder", map[string]any{"expectedDraftVersion": 3, "questionIds": []string{ids[2], ids[1], ids[0]}})
	require.Equal(t, http.StatusOK, reorder.Code, reorder.Body.String())
	assert.Equal(t, ids[2], reorder.json()["questions"].([]any)[0].(map[string]any)["id"])
	badOrder := a.jsonReq(a.h.teacher, http.MethodPut, base+"/draft/reorder", map[string]any{"expectedDraftVersion": 4, "questionIds": []string{ids[0]}})
	assert.Equal(t, http.StatusBadRequest, badOrder.Code)

	noVersion := a.do(a.h.teacher, http.MethodDelete, base+"/draft/questions/"+newID, nil, "")
	assert.Equal(t, http.StatusBadRequest, noVersion.Code, "the version is required on delete too")
	del := a.do(a.h.teacher, http.MethodDelete, base+"/draft/questions/"+newID+"?expectedDraftVersion=4", nil, "")
	require.Equal(t, http.StatusOK, del.Code, del.Body.String())
	assert.Len(t, del.json()["questions"], 2)

	// Marks for the second question, then approve.
	rest := a.do(a.h.teacher, http.MethodGet, base+"/draft", nil, "").json()
	second := rest["questions"].([]any)
	var essayID string
	for _, q := range second {
		if q.(map[string]any)["type"] == "ESSAY" {
			essayID = q.(map[string]any)["id"].(string)
		}
	}
	v := int(rest["draftVersion"].(float64))
	fix := a.jsonReq(a.h.teacher, http.MethodPut, base+"/draft/questions/"+essayID, map[string]any{
		"expectedDraftVersion": v,
		"question":             map[string]any{"numberLabel": "2", "type": "ESSAY", "text": q2Text, "points": 5},
	})
	require.Equal(t, http.StatusOK, fix.Code, fix.Body.String())

	// A draft exam is the only place approval writes.
	done := a.jsonReq(a.h.teacher, http.MethodPost, base+"/approve", map[string]any{"expectedDraftVersion": int(fix.json()["draftVersion"].(float64))})
	require.Equal(t, http.StatusOK, done.Code, done.Body.String())
	out := done.json()
	assert.Equal(t, "COMPLETED", out["document"].(map[string]any)["status"])
	created := out["questions"].([]any)
	require.Len(t, created, 2)
	assert.EqualValues(t, 1, created[0].(map[string]any)["questionNumber"])

	again := a.jsonReq(a.h.teacher, http.MethodPost, base+"/approve", map[string]any{"expectedDraftVersion": 99})
	assert.Equal(t, http.StatusConflict, again.Code)
	assert.Equal(t, "ALREADY_APPROVED", again.json()["code"])
	assert.Len(t, a.h.w.questions, 2, "approving twice created nothing more")

	edit2 := a.jsonReq(a.h.teacher, http.MethodPut, base+"/draft/questions/"+essayID, map[string]any{
		"expectedDraftVersion": 99, "question": map[string]any{"type": "ESSAY", "text": "changed after approval"}})
	assert.Equal(t, http.StatusConflict, edit2.Code)
}

func TestAPI_ApproveNeedsTheVersion(t *testing.T) {
	a := newAPI(t)
	d := a.h.processed()
	r := a.jsonReq(a.h.teacher, http.MethodPost, "/"+d.ID.String()+"/approve", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, r.Code)
	assert.Empty(t, a.h.w.questions)
}

func TestAPI_CancelRetryDelete(t *testing.T) {
	a := newAPI(t)
	up := a.upload(a.h.teacher, "a.pdf", samplePDF())
	id := up.json()["id"].(string)

	cancel := a.do(a.h.teacher, http.MethodPost, "/"+id+"/cancel", nil, "")
	require.Equal(t, http.StatusOK, cancel.Code)
	assert.Equal(t, "CANCELLED", cancel.json()["status"])

	retry := a.do(a.h.teacher, http.MethodPost, "/"+id+"/retry", nil, "") // no body at all
	require.Equal(t, http.StatusAccepted, retry.Code, retry.Body.String())
	assert.Equal(t, "QUEUED", retry.json()["status"])
	assert.EqualValues(t, 2, retry.json()["attempt"])

	again := a.do(a.h.teacher, http.MethodPost, "/"+id+"/retry", nil, "")
	assert.Equal(t, http.StatusConflict, again.Code, "already queued")
	assert.Equal(t, "INVALID_STATE", again.json()["code"])

	del := a.do(a.h.teacher, http.MethodDelete, "/"+id, nil, "")
	assert.Equal(t, http.StatusNoContent, del.Code)
	assert.Equal(t, http.StatusNotFound, a.do(a.h.teacher, http.MethodGet, "/"+id, nil, "").Code)
}

func TestAPI_FailedDocumentsExplainThemselvesInOurWords(t *testing.T) {
	a := newAPI(t)
	a.h.model.Replies = nil
	a.h.model.Replies = append(a.h.model.Replies, replyFailure(extract.NewError(extract.CodeRateLimited, true, "HTTP 429 from api.groq.com")))
	a.h.rebuild()
	up := a.upload(a.h.teacher, "a.pdf", samplePDF())
	id := uuid.MustParse(up.json()["id"].(string))
	require.NoError(t, a.h.svc.Process(context.Background(), id, 1, true))

	r := a.do(a.h.teacher, http.MethodGet, "/"+id.String(), nil, "")
	body := r.json()
	assert.Equal(t, "FAILED", body["status"])
	assert.Equal(t, "PROVIDER_RATE_LIMITED", body["failureCode"])
	assert.NotContains(t, body["failureMessage"], "groq.com")
	draft := a.do(a.h.teacher, http.MethodGet, "/"+id.String()+"/draft", nil, "")
	assert.Equal(t, http.StatusConflict, draft.Code, "there is no draft to show for a failed document")
}

func replyFailure(err error) extracttest.Reply { return extracttest.Reply{Err: err} }
