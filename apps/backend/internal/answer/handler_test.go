package answer

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

// stubService answers SubmitAnswer with whatever the test sets.
type stubService struct {
	got    *SubmitAnswerRequest
	answer *Answer
	err    error
}

func (s *stubService) SubmitAnswer(_ context.Context, _, _ uuid.UUID, req SubmitAnswerRequest, _ uuid.UUID) (*Answer, error) {
	s.got = &req
	return s.answer, s.err
}

func (s *stubService) GetAnswer(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*Answer, error) {
	return s.answer, s.err
}

func (s *stubService) ListAnswers(context.Context, uuid.UUID, uuid.UUID) ([]Answer, error) {
	return nil, s.err
}

func putAnswer(t *testing.T, svc Service, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("session_id", "question_id")
	c.SetParamValues(uuid.NewString(), uuid.NewString())
	c.Set("user_id", uuid.NewString())
	return rec, NewHandler(svc).SubmitAnswer(c)
}

func TestSubmitAnswer_PassesTheRevisionTheClientSawAndReturnsTheNewOne(t *testing.T) {
	svc := &stubService{answer: &Answer{TextAnswer: "light into sugar", RevisionNo: 4}}
	rec, err := putAnswer(t, svc, `{"textAnswer":"light into sugar","expectedRevisionNo":3}`)
	require.NoError(t, err)

	require.NotNil(t, svc.got.ExpectedRevisionNo)
	assert.Equal(t, 3, *svc.got.ExpectedRevisionNo)
	var body AnswerResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, 4, body.RevisionNo)
}

func TestSubmitAnswer_AStaleSaveIsAConflictWithItsOwnCode(t *testing.T) {
	_, err := putAnswer(t, &stubService{err: ErrAnswerChanged}, `{"textAnswer":"late","expectedRevisionNo":1}`)

	var he *httperrs.HTTPError
	require.True(t, errors.As(err, &he))
	assert.Equal(t, http.StatusConflict, he.Status)
	assert.Equal(t, "ANSWER_CHANGED", he.Code)
}

func TestSubmitAnswer_RefusesBadRevisionsAndOverlongText(t *testing.T) {
	cases := map[string]string{
		"negative revision":   `{"textAnswer":"x","expectedRevisionNo":-1}`,
		"text over the limit": `{"textAnswer":"` + strings.Repeat("a", 50001) + `"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &stubService{answer: &Answer{}}
			_, err := putAnswer(t, svc, body)

			var he *httperrs.HTTPError
			require.True(t, errors.As(err, &he))
			assert.Equal(t, http.StatusBadRequest, he.Status)
			assert.Nil(t, svc.got, "nothing reaches the service")
		})
	}

	t.Run("text at the limit is accepted, counted in characters", func(t *testing.T) {
		svc := &stubService{answer: &Answer{}}
		_, err := putAnswer(t, svc, `{"textAnswer":"`+strings.Repeat("न", 50000)+`"}`)
		require.NoError(t, err)
	})
}
