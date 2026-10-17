package grading

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
)

// stubService records what the handler passed on and returns canned results.
type stubService struct {
	err    error
	gotReq SaveMarkRequest
	caller Caller
	called int
}

func (s *stubService) GetMarking(_ context.Context, _, _ uuid.UUID, c Caller) (*Marking, error) {
	s.called++
	s.caller = c
	return &Marking{}, s.err
}

func (s *stubService) SaveMark(_ context.Context, _, _, _ uuid.UUID, r SaveMarkRequest, c Caller, _ AccessMeta) (*SaveMarkResult, error) {
	s.called++
	s.gotReq, s.caller = r, c
	if s.err != nil {
		return nil, s.err
	}
	half := 15
	return &SaveMarkResult{Line: Line{MaxHalf: 20, EffectiveHalf: &half, Source: SourceTeacher, Version: 1}}, nil
}

func (s *stubService) Finalise(context.Context, uuid.UUID, uuid.UUID, FinaliseRequest, Caller, AccessMeta) (*Marking, error) {
	s.called++
	return &Marking{}, s.err
}

func (s *stubService) Reopen(context.Context, uuid.UUID, uuid.UUID, ReopenRequest, Caller, AccessMeta) (*Marking, error) {
	s.called++
	return &Marking{}, s.err
}

func do(t *testing.T, h echo.HandlerFunc, role, method, body string, params map[string]string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	names, values := []string{"exam_id", "student_id", "question_id"}, []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	for i, n := range names {
		if v, ok := params[n]; ok {
			values[i] = v
		}
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	c.Set("user_id", uuid.NewString())
	c.Set("user_role", role)
	err := h(c)
	return rec, err
}

func statusOf(t *testing.T, err error) (int, *httperrs.HTTPError) {
	t.Helper()
	var he *httperrs.HTTPError
	require.ErrorAs(t, err, &he)
	return he.Status, he
}

func saveBody(marks string) string {
	return `{"sessionId":"` + uuid.NewString() + `","marks":` + marks + `,"comment":"ok","expectedVersion":2}`
}

func TestSaveMarkHandler_ParsesMarksStrictly(t *testing.T) {
	for marks, want := range map[string]int{"7.5": 15, "7": 14, "0": 0, "10.0": 20} {
		svc := &stubService{}
		rec, err := do(t, NewHandler(svc).SaveMark, "TEACHER", http.MethodPut, saveBody(marks), nil)
		require.NoError(t, err, marks)
		assert.Equal(t, http.StatusOK, rec.Code)
		require.NotNil(t, svc.gotReq.MarksHalf, marks)
		assert.Equal(t, want, *svc.gotReq.MarksHalf, marks)
		assert.Equal(t, 2, svc.gotReq.ExpectedVersion)
	}

	svc := &stubService{}
	_, err := do(t, NewHandler(svc).SaveMark, "TEACHER", http.MethodPut, saveBody("null"), nil)
	require.NoError(t, err)
	assert.Nil(t, svc.gotReq.MarksHalf, "null clears the teacher's mark")

	for _, bad := range []string{"7.25", "-1", "1e1", `"abc"`, "0.3", "1000000000"} {
		svc := &stubService{}
		_, err := do(t, NewHandler(svc).SaveMark, "TEACHER", http.MethodPut, saveBody(bad), nil)
		status, he := statusOf(t, err)
		assert.Equal(t, http.StatusBadRequest, status, bad)
		if bad != `"abc"` { // text is refused while decoding, before the marks rules run
			assert.Equal(t, "VALIDATION_ERROR", he.Code, bad)
		}
		assert.Equal(t, 0, svc.called, "%s never reaches the service", bad)
	}
}

func TestHandlers_RejectMalformedRequests(t *testing.T) {
	h := NewHandler(&stubService{})
	cases := map[string]struct {
		fn     echo.HandlerFunc
		body   string
		params map[string]string
	}{
		"bad exam id":      {h.Get, "", map[string]string{"exam_id": "nope"}},
		"bad student id":   {h.Get, "", map[string]string{"student_id": "nope"}},
		"bad question id":  {h.SaveMark, saveBody("1"), map[string]string{"question_id": "nope"}},
		"save no session":  {h.SaveMark, `{"marks":1}`, nil},
		"save bad json":    {h.SaveMark, `{`, nil},
		"finalise no sess": {h.Finalise, `{}`, nil},
		"reopen no sess":   {h.Reopen, `{"reason":"because"}`, nil},
	}
	for name, tc := range cases {
		_, err := do(t, tc.fn, "TEACHER", http.MethodPost, tc.body, tc.params)
		status, _ := statusOf(t, err)
		assert.Equal(t, http.StatusBadRequest, status, name)
	}
}

func TestHandlers_TakeTheCallerFromTheTokenNotTheBody(t *testing.T) {
	svc := &stubService{}
	body := `{"sessionId":"` + uuid.NewString() + `","marks":1,"expectedVersion":0,"userId":"` + uuid.NewString() + `","role":"ADMIN"}`
	_, err := do(t, NewHandler(svc).SaveMark, "teacher", http.MethodPut, body, nil)
	require.NoError(t, err)
	assert.Equal(t, "TEACHER", string(svc.caller.Role))
}

func TestHandlers_MapErrors(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{ErrExamNotFound, 404, "NOT_FOUND"},
		{ErrForbiddenRole, 403, "FORBIDDEN"},
		{ErrStaleVersion, 409, "STALE_VERSION"},
		{ErrPaperFinalised, 409, "PAPER_FINALISED"},
		{ErrSessionNotEnded, 409, "SESSION_NOT_ENDED"},
		{ErrSessionChanged, 409, "SESSION_CHANGED"},
		{&IncompleteError{Questions: []int{2, 5}}, 409, "PAPER_INCOMPLETE"},
		{ErrAuditUnavailable, 503, "AUDIT_UNAVAILABLE"},
		{&ValidationError{Field: "comment", Message: "the comment is too long"}, 400, "VALIDATION_ERROR"},
		{errors.New("pq: connection refused at 10.0.0.5"), 500, ""},
	}
	for _, tc := range cases {
		h := NewHandler(&stubService{err: tc.err})
		_, err := do(t, h.Finalise, "TEACHER", http.MethodPost, `{"sessionId":"`+uuid.NewString()+`"}`, nil)
		status, he := statusOf(t, err)
		assert.Equal(t, tc.status, status, tc.err.Error())
		if tc.code != "" {
			assert.Equal(t, tc.code, he.Code)
		}
		assert.NotContains(t, he.Message, "10.0.0.5", "internal detail never reaches the client")
	}

	h := NewHandler(&stubService{err: &IncompleteError{Questions: []int{2, 5}}})
	_, err := do(t, h.Finalise, "TEACHER", http.MethodPost, `{"sessionId":"`+uuid.NewString()+`"}`, nil)
	_, he := statusOf(t, err)
	assert.Contains(t, he.Message, "2, 5")
	assert.Len(t, he.Errors, 2)
}

func TestHandlers_ResponsesAreNotCacheableAndUseMarks(t *testing.T) {
	rec, err := do(t, NewHandler(&stubService{}).SaveMark, "TEACHER", http.MethodPut, saveBody("7.5"), nil)
	require.NoError(t, err)
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))

	var out struct {
		Question struct {
			EffectiveMarks float64 `json:"effectiveMarks"`
			MaxMarks       float64 `json:"maxMarks"`
		} `json:"question"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, 7.5, out.Question.EffectiveMarks)
	assert.Equal(t, 10.0, out.Question.MaxMarks)
}

func TestHandlers_ARealServiceRefusesStudentsAndProctorsOnEveryRoute(t *testing.T) {
	f := newFixture()
	h := NewHandler(f.svc)
	session := `{"sessionId":"` + uuid.NewString() + `","reason":"some reason"}`
	params := map[string]string{"exam_id": f.examID.String(), "student_id": f.studentID.String()}
	for _, role := range []string{"STUDENT", "PROCTOR", ""} {
		for name, fn := range map[string]echo.HandlerFunc{"get": h.Get, "save": h.SaveMark, "finalise": h.Finalise, "reopen": h.Reopen} {
			_, err := do(t, fn, role, http.MethodPost, session, params)
			status, _ := statusOf(t, err)
			assert.Equal(t, http.StatusForbidden, status, role+" "+name)
		}
	}
	assert.Empty(t, f.reader.order, "a refused caller reads and writes nothing")
}

func TestRoutes(t *testing.T) {
	e := echo.New()
	NewHandler(&stubService{}).RegisterRoutes(e.Group("/api/v1"), nil, nil)
	got := map[string]bool{}
	for _, r := range e.Routes() {
		got[r.Method+" "+r.Path] = true
	}
	base := "/api/v1/exams/:exam_id/responses/:student_id/marking"
	for _, want := range []string{"GET " + base, "PUT " + base + "/questions/:question_id", "POST " + base + "/finalise", "POST " + base + "/reopen"} {
		assert.True(t, got[want], want)
	}
}
