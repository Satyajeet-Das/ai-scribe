package exam

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
)

func setupTestHandler() (*Handler, *mockExamRepo, *echo.Echo) {
	repo := newMockExamRepo()
	logger := zerolog.Nop()
	svc := NewService(repo, &logger)
	h := NewHandler(svc)

	e := echo.New()
	return h, repo, e
}

func setAuth(c echo.Context, userID uuid.UUID, role platformauth.Role) {
	identity := &platformauth.AuthenticatedIdentity{
		UserID: userID,
		Role:   role,
	}
	platformauth.SetIdentity(c, identity)
}

func TestHandler_CreateExam(t *testing.T) {
	h, _, e := setupTestHandler()
	teacherID := uuid.New()

	t.Run("creates exam successfully", func(t *testing.T) {
		body := `{"title": "Biology Final", "subject": "Biology", "durationMins": 90}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/exams", bytes.NewBufferString(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		setAuth(c, teacherID, platformauth.RoleTeacher)

		err := h.CreateExam(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp ExamResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "Biology Final", resp.Title)
		assert.Equal(t, StatusDraft, resp.Status)
		assert.Equal(t, teacherID, resp.CreatedBy)
	})

	t.Run("fails validation on missing fields", func(t *testing.T) {
		body := `{"title": "", "subject": "", "durationMins": 0}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/exams", bytes.NewBufferString(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		setAuth(c, teacherID, platformauth.RoleTeacher)

		err := h.CreateExam(c)
		assert.Error(t, err)
	})
}

func TestHandler_GetExam(t *testing.T) {
	h, _, e := setupTestHandler()
	teacherID := uuid.New()
	otherTeacherID := uuid.New()

	// Create an exam first
	body := `{"title": "Math 101", "subject": "Math", "durationMins": 60}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exams", bytes.NewBufferString(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuth(c, teacherID, platformauth.RoleTeacher)
	require.NoError(t, h.CreateExam(c))

	var created ExamResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	t.Run("owner gets exam details", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/exams/"+created.ID.String(), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(created.ID.String())
		setAuth(c, teacherID, platformauth.RoleTeacher)

		err := h.GetExam(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp ExamResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, created.ID, resp.ID)
	})

	t.Run("other teacher gets 403 forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/exams/"+created.ID.String(), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(created.ID.String())
		setAuth(c, otherTeacherID, platformauth.RoleTeacher)

		err := h.GetExam(c)
		require.Error(t, err)
	})

	t.Run("invalid UUID gets 400 bad request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/exams/not-a-uuid", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("not-a-uuid")
		setAuth(c, teacherID, platformauth.RoleTeacher)

		err := h.GetExam(c)
		require.Error(t, err)
	})
}

func TestHandler_ListExams(t *testing.T) {
	h, _, e := setupTestHandler()
	teacherID := uuid.New()

	for _, title := range []string{"Physics", "Chemistry", "Math"} {
		body, _ := json.Marshal(CreateExamRequest{
			Title:        title,
			Subject:      "Science",
			DurationMins: 45,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/exams", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		setAuth(c, teacherID, platformauth.RoleTeacher)
		require.NoError(t, h.CreateExam(c))
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exams?limit=10&offset=0&subject=Science", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuth(c, teacherID, platformauth.RoleTeacher)

	err := h.ListExams(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var listResp ExamListResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	assert.Equal(t, 3, listResp.Total)
	assert.Len(t, listResp.Exams, 3)
	assert.Len(t, listResp.Data, 3)
}

func TestHandler_LifecycleEndpoints(t *testing.T) {
	h, repo, e := setupTestHandler()
	teacherID := uuid.New()

	// Create draft exam
	body, _ := json.Marshal(CreateExamRequest{
		Title:        "Literature Midterm",
		Subject:      "English",
		DurationMins: 60,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exams", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuth(c, teacherID, platformauth.RoleTeacher)
	require.NoError(t, h.CreateExam(c))

	var examResp ExamResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &examResp))
	examID := examResp.ID.String()

	t.Run("publish exam", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/exams/"+examID+"/publish", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(examID)
		setAuth(c, teacherID, platformauth.RoleTeacher)

		err := h.PublishExam(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var pubResp ExamResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pubResp))
		assert.Equal(t, StatusPublished, pubResp.Status)
	})

	t.Run("unpublish exam", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/exams/"+examID+"/unpublish", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(examID)
		setAuth(c, teacherID, platformauth.RoleTeacher)

		err := h.UnpublishExam(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var unpubResp ExamResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &unpubResp))
		assert.Equal(t, StatusDraft, unpubResp.Status)
	})

	t.Run("archive exam", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/exams/"+examID+"/archive", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(examID)
		setAuth(c, teacherID, platformauth.RoleTeacher)

		err := h.ArchiveExam(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var archResp ExamResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &archResp))
		assert.Equal(t, StatusArchived, archResp.Status)
	})

	t.Run("delete exam", func(t *testing.T) {
		// New draft exam to delete
		newExam, err := h.service.CreateExam(c.Request().Context(), CreateExamRequest{
			Title:        "To be deleted",
			Subject:      "Test",
			DurationMins: 30,
		}, Caller{ID: teacherID, Role: platformauth.RoleTeacher})
		require.NoError(t, err)

		deleteID := newExam.ID.String()

		// If it has relations, delete fails
		repo.hasRelations[newExam.ID] = true
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/exams/"+deleteID, nil)
		rec := httptest.NewRecorder()
		cDel := e.NewContext(req, rec)
		cDel.SetParamNames("id")
		cDel.SetParamValues(deleteID)
		setAuth(cDel, teacherID, platformauth.RoleTeacher)
		err = h.DeleteExam(cDel)
		assert.Error(t, err)

		// Without relations, delete succeeds
		repo.hasRelations[newExam.ID] = false
		rec2 := httptest.NewRecorder()
		cDel2 := e.NewContext(req, rec2)
		cDel2.SetParamNames("id")
		cDel2.SetParamValues(deleteID)
		setAuth(cDel2, teacherID, platformauth.RoleTeacher)
		err = h.DeleteExam(cDel2)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec2.Code)
	})
}
