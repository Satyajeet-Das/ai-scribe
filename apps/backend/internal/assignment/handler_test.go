package assignment

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
)

func setupTestEcho() *echo.Echo {
	e := echo.New()
	return e
}

func setTestAuth(c echo.Context, userID uuid.UUID, role platformauth.Role) {
	c.Set("user_id", userID.String())
	c.Set("user_role", string(role))
	identity := &platformauth.AuthenticatedIdentity{
		UserID: userID,
		Role:   role,
	}
	platformauth.SetIdentity(c, identity)
}

func TestHandler_CreateAssignment(t *testing.T) {
	svc, _, _, userReader, publishedExamID, _, teacherCaller, studentCaller, _ := setupTestService()
	h := NewHandler(svc)
	e := setupTestEcho()

	studentID := uuid.New()
	roll := "23CS010"
	userReader.users[studentID] = makeTestUser(studentID, "STUDENT", true, "John", "Doe", &roll)

	t.Run("creates assignment with studentId successfully", func(t *testing.T) {
		reqBody := CreateAssignmentRequest{
			ExamID:    publishedExamID,
			StudentID: &studentID,
		}
		b, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/assignments", bytes.NewReader(b))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		setTestAuth(c, teacherCaller.ID, teacherCaller.Role)

		err := h.CreateAssignment(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp AssignmentResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, publishedExamID, resp.ExamID)
		assert.Equal(t, studentID, resp.StudentID)
		assert.Equal(t, StatusAssigned, resp.Status)
	})

	t.Run("creates assignment with rollNo successfully", func(t *testing.T) {
		stu2ID := uuid.New()
		roll2 := "23CS011"
		userReader.users[stu2ID] = makeTestUser(stu2ID, "STUDENT", true, "Jane", "Doe", &roll2)

		reqBody := CreateAssignmentRequest{
			ExamID: publishedExamID,
			RollNo: &roll2,
		}
		b, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/assignments", bytes.NewReader(b))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		setTestAuth(c, teacherCaller.ID, teacherCaller.Role)

		err := h.CreateAssignment(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp AssignmentResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, stu2ID, resp.StudentID)
		assert.Equal(t, "23CS011", resp.StudentRollNo)
	})

	t.Run("student cannot create assignments", func(t *testing.T) {
		reqBody := CreateAssignmentRequest{
			ExamID:    publishedExamID,
			StudentID: &studentID,
		}
		b, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/assignments", bytes.NewReader(b))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		setTestAuth(c, studentCaller.ID, studentCaller.Role)

		err := h.CreateAssignment(c)
		require.Error(t, err)
	})
}

func TestHandler_BulkAssign(t *testing.T) {
	svc, _, _, userReader, publishedExamID, _, teacherCaller, _, _ := setupTestService()
	h := NewHandler(svc)
	e := setupTestEcho()

	rollA := "23CS020"
	rollB := "23CS021"
	stuAID := uuid.New()
	stuBID := uuid.New()
	userReader.users[stuAID] = makeTestUser(stuAID, "STUDENT", true, "Alpha", "", &rollA)
	userReader.users[stuBID] = makeTestUser(stuBID, "STUDENT", true, "Beta", "", &rollB)

	reqBody := BulkAssignRequest{
		ExamID:      publishedExamID,
		RollNumbers: []string{rollA, rollB, "NONEXISTENT_ROLL"},
	}
	b, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assignments/bulk", bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setTestAuth(c, teacherCaller.ID, teacherCaller.Role)

	err := h.BulkAssign(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp BulkAssignResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.TotalAssigned)
	assert.Equal(t, 1, resp.TotalFailed)
	assert.Equal(t, "NONEXISTENT_ROLL", resp.Failed[0].Identifier)
}

func TestHandler_CheckAssignment(t *testing.T) {
	svc, _, _, userReader, publishedExamID, _, teacherCaller, studentCaller, _ := setupTestService()
	h := NewHandler(svc)
	e := setupTestEcho()

	t.Run("check for unassigned student", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/assignments/check?exam_id="+publishedExamID.String(), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		setTestAuth(c, studentCaller.ID, studentCaller.Role)

		err := h.CheckAssignment(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp CheckAssignmentResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.False(t, resp.Assigned)
	})

	t.Run("check for assigned student", func(t *testing.T) {
		// First assign student
		_, err := svc.CreateAssignment(context.Background(), CreateAssignmentRequest{
			ExamID:    publishedExamID,
			StudentID: &studentCaller.ID,
		}, teacherCaller)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/assignments/check?exam_id="+publishedExamID.String(), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		setTestAuth(c, studentCaller.ID, studentCaller.Role)

		err = h.CheckAssignment(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp CheckAssignmentResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.True(t, resp.Assigned)
		assert.NotNil(t, resp.AssignmentID)
	})
	_ = userReader
}

func TestHandler_ListExamAssignments(t *testing.T) {
	svc, _, _, _, publishedExamID, _, teacherCaller, _, _ := setupTestService()
	h := NewHandler(svc)
	e := setupTestEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assignments/exam/"+publishedExamID.String(), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("examId")
	c.SetParamValues(publishedExamID.String())
	setTestAuth(c, teacherCaller.ID, teacherCaller.Role)

	err := h.ListExamAssignments(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp AssignmentListResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Total)
}

func TestHandler_ListStudentAssignedExams(t *testing.T) {
	svc, repo, _, _, publishedExamID, _, teacherCaller, studentCaller, _ := setupTestService()
	h := NewHandler(svc)
	e := setupTestEcho()

	asgnID := uuid.New()
	repo.assignments[asgnID] = &Assignment{
		ExamID:     publishedExamID,
		StudentID:  studentCaller.ID,
		Status:     StatusAssigned,
		AssignedAt: time.Now().UTC(),
	}

	t.Run("student lists their own assigned exams", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/assignments/student/"+studentCaller.ID.String(), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("studentId")
		c.SetParamValues(studentCaller.ID.String())
		setTestAuth(c, studentCaller.ID, studentCaller.Role)

		err := h.ListStudentAssignedExams(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp StudentAssignedExamsListResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.Total)
		assert.Len(t, resp.Data, 1)
		assert.Equal(t, publishedExamID, resp.Data[0].ExamID)
	})

	t.Run("student cannot list other student's exams", func(t *testing.T) {
		otherID := uuid.New()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/assignments/student/"+otherID.String(), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("studentId")
		c.SetParamValues(otherID.String())
		setTestAuth(c, studentCaller.ID, studentCaller.Role)

		err := h.ListStudentAssignedExams(c)
		require.Error(t, err)
	})
	_ = teacherCaller
}

func TestHandler_RevokeAssignment(t *testing.T) {
	svc, repo, _, _, publishedExamID, _, teacherCaller, _, _ := setupTestService()
	h := NewHandler(svc)
	e := setupTestEcho()

	assignmentID := uuid.New()
	studentID := uuid.New()
	repo.assignments[assignmentID] = &Assignment{
		ExamID:    publishedExamID,
		StudentID: studentID,
		Status:    StatusAssigned,
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/assignments/"+assignmentID.String(), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(assignmentID.String())
	setTestAuth(c, teacherCaller.ID, teacherCaller.Role)

	err := h.RevokeAssignment(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func cContext() echo.Context {
	return echo.New().NewContext(nil, nil)
}
