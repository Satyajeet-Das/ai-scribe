package assignment

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/validation"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) RegisterRoutes(g *echo.Group, authMiddleware echo.MiddlewareFunc) {
	assignments := g.Group("/assignments")
	if authMiddleware != nil {
		assignments.Use(authMiddleware)
	}

	assignments.GET("", h.ListAssignments)
	assignments.GET("/exam/:examId", h.ListExamAssignments)
	assignments.POST("", h.CreateAssignment)
	assignments.POST("/bulk", h.BulkAssign)
	assignments.GET("/check", h.CheckAssignment)
	assignments.GET("/student/:studentId", h.ListStudentAssignedExams)
	assignments.GET("/my-exams", h.ListMyAssignedExams)
	assignments.GET("/:id", h.GetAssignment)
	assignments.POST("/:id/revoke", h.RevokeAssignment)
	assignments.DELETE("/:id", h.RevokeAssignment)

	// Nested routes under /exams/:exam_id/assignments
	examAssignments := g.Group("/exams/:exam_id/assignments")
	if authMiddleware != nil {
		examAssignments.Use(authMiddleware)
	}
	examAssignments.GET("", h.ListExamAssignmentsNested)
	examAssignments.POST("", h.CreateAssignmentNested)
	examAssignments.POST("/bulk", h.BulkAssignNested)
	examAssignments.GET("/check", h.CheckAssignmentNested)
	examAssignments.DELETE("/:student_id", h.RevokeAssignmentByExamAndStudent)
}

func (h *Handler) GetAssignment(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid assignment ID format", false, nil, nil, nil)
	}

	a, err := h.service.GetAssignment(c.Request().Context(), id)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, ToAssignmentResponse(a))
}

func (h *Handler) ListExamAssignments(c echo.Context) error {
	idParam := c.Param("examId")
	examID, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	caller := getCaller(c)
	var search *string
	if q := c.QueryParam("q"); q != "" {
		search = &q
	} else if s := c.QueryParam("search"); s != "" {
		search = &s
	}

	assignments, total, err := h.service.ListAssignments(c.Request().Context(), 100, 0, &examID, nil, nil, search, caller)
	if err != nil {
		return h.mapError(err)
	}

	resList := ToAssignmentResponseList(assignments)
	return c.JSON(http.StatusOK, AssignmentListResponse{
		Data:        resList,
		Assignments: resList,
		Total:       total,
		Limit:       100,
		Offset:      0,
	})
}

func (h *Handler) ListExamAssignmentsNested(c echo.Context) error {
	idParam := c.Param("exam_id")
	examID, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	limit := parsePositiveInt(c.QueryParam("limit"), 20)
	offset := parseNonNegativeInt(c.QueryParam("offset"), 0)

	var search *string
	if q := c.QueryParam("q"); q != "" {
		search = &q
	} else if s := c.QueryParam("search"); s != "" {
		search = &s
	}

	var statusFilter *Status
	if st := c.QueryParam("status"); st != "" {
		s := Status(st)
		statusFilter = &s
	}

	caller := getCaller(c)
	assignments, total, err := h.service.ListAssignments(c.Request().Context(), limit, offset, &examID, nil, statusFilter, search, caller)
	if err != nil {
		return h.mapError(err)
	}

	resList := ToAssignmentResponseList(assignments)
	return c.JSON(http.StatusOK, AssignmentListResponse{
		Data:        resList,
		Assignments: resList,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
	})
}

func (h *Handler) ListAssignments(c echo.Context) error {
	limit := parsePositiveInt(c.QueryParam("limit"), 20)
	offset := parseNonNegativeInt(c.QueryParam("offset"), 0)

	var examID *uuid.UUID
	if e := c.QueryParam("exam_id"); e != "" {
		if id, err := uuid.Parse(e); err == nil {
			examID = &id
		}
	}

	var studentID *uuid.UUID
	if s := c.QueryParam("student_id"); s != "" {
		if id, err := uuid.Parse(s); err == nil {
			studentID = &id
		}
	}

	var statusFilter *Status
	if st := c.QueryParam("status"); st != "" {
		s := Status(st)
		statusFilter = &s
	}

	var search *string
	if q := c.QueryParam("q"); q != "" {
		search = &q
	} else if s := c.QueryParam("search"); s != "" {
		search = &s
	}

	caller := getCaller(c)
	assignments, total, err := h.service.ListAssignments(c.Request().Context(), limit, offset, examID, studentID, statusFilter, search, caller)
	if err != nil {
		return h.mapError(err)
	}

	resList := ToAssignmentResponseList(assignments)
	return c.JSON(http.StatusOK, AssignmentListResponse{
		Data:        resList,
		Assignments: resList,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
	})
}

func (h *Handler) CreateAssignment(c echo.Context) error {
	var req CreateAssignmentRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	if req.ExamID == uuid.Nil {
		if eParam := c.Param("exam_id"); eParam != "" {
			if parsed, err := uuid.Parse(eParam); err == nil {
				req.ExamID = parsed
			}
		}
	}

	caller := getCaller(c)
	a, err := h.service.CreateAssignment(c.Request().Context(), req, caller)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusCreated, ToAssignmentResponse(a))
}

func (h *Handler) CreateAssignmentNested(c echo.Context) error {
	examIDParam := c.Param("exam_id")
	examID, err := uuid.Parse(examIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	var req CreateAssignmentRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}
	req.ExamID = examID

	caller := getCaller(c)
	a, err := h.service.CreateAssignment(c.Request().Context(), req, caller)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusCreated, ToAssignmentResponse(a))
}

func (h *Handler) BulkAssign(c echo.Context) error {
	var req BulkAssignRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	if req.ExamID == uuid.Nil {
		if eParam := c.Param("exam_id"); eParam != "" {
			if parsed, err := uuid.Parse(eParam); err == nil {
				req.ExamID = parsed
			}
		}
	}

	caller := getCaller(c)
	resp, err := h.service.BulkAssign(c.Request().Context(), req, caller)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusCreated, resp)
}

func (h *Handler) BulkAssignNested(c echo.Context) error {
	examIDParam := c.Param("exam_id")
	examID, err := uuid.Parse(examIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	var req BulkAssignRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}
	req.ExamID = examID

	caller := getCaller(c)
	resp, err := h.service.BulkAssign(c.Request().Context(), req, caller)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusCreated, resp)
}

func (h *Handler) CheckAssignment(c echo.Context) error {
	examIDStr := c.QueryParam("exam_id")
	if examIDStr == "" {
		examIDStr = c.Param("exam_id")
	}
	examID, err := uuid.Parse(examIDStr)
	if err != nil {
		return httperrs.NewBadRequestError("Missing or invalid exam_id query parameter", false, nil, nil, nil)
	}

	caller := getCaller(c)
	studentIDStr := c.QueryParam("student_id")
	var studentID uuid.UUID

	if studentIDStr != "" {
		parsed, err := uuid.Parse(studentIDStr)
		if err != nil {
			return httperrs.NewBadRequestError("Invalid student_id format", false, nil, nil, nil)
		}
		studentID = parsed
	} else if caller.IsStudent() {
		studentID = caller.ID
	} else {
		return httperrs.NewBadRequestError("student_id is required for non-student users", false, nil, nil, nil)
	}

	a, isAssigned, err := h.service.CheckAssignment(c.Request().Context(), examID, studentID, caller)
	if err != nil {
		return h.mapError(err)
	}

	resp := CheckAssignmentResponse{
		Assigned: isAssigned,
	}
	if a != nil {
		resp.AssignmentID = &a.ID
		resp.Status = &a.Status
		resp.AssignedAt = &a.AssignedAt
		asgnResp := ToAssignmentResponse(a)
		resp.Assignment = &asgnResp
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) CheckAssignmentNested(c echo.Context) error {
	return h.CheckAssignment(c)
}

func (h *Handler) ListStudentAssignedExams(c echo.Context) error {
	idParam := c.Param("studentId")
	studentID, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid student ID format", false, nil, nil, nil)
	}

	limit := parsePositiveInt(c.QueryParam("limit"), 20)
	offset := parseNonNegativeInt(c.QueryParam("offset"), 0)

	caller := getCaller(c)
	exams, total, err := h.service.ListStudentAssignedExams(c.Request().Context(), studentID, limit, offset, caller)
	if err != nil {
		return h.mapError(err)
	}

	resList := ToStudentAssignedExamResponseList(exams)
	return c.JSON(http.StatusOK, StudentAssignedExamsListResponse{
		Data:   resList,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

func (h *Handler) ListMyAssignedExams(c echo.Context) error {
	caller := getCaller(c)
	if caller.ID == uuid.Nil {
		return httperrs.NewUnauthorizedError("Authentication required", false)
	}

	limit := parsePositiveInt(c.QueryParam("limit"), 20)
	offset := parseNonNegativeInt(c.QueryParam("offset"), 0)

	exams, total, err := h.service.ListStudentAssignedExams(c.Request().Context(), caller.ID, limit, offset, caller)
	if err != nil {
		return h.mapError(err)
	}

	resList := ToStudentAssignedExamResponseList(exams)
	return c.JSON(http.StatusOK, StudentAssignedExamsListResponse{
		Data:   resList,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

func (h *Handler) RevokeAssignment(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid assignment ID format", false, nil, nil, nil)
	}

	caller := getCaller(c)
	if err := h.service.RevokeAssignment(c.Request().Context(), id, caller); err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Assignment successfully revoked",
	})
}

func (h *Handler) RevokeAssignmentByExamAndStudent(c echo.Context) error {
	examIDParam := c.Param("exam_id")
	examID, err := uuid.Parse(examIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	studentIDParam := c.Param("student_id")
	studentID, err := uuid.Parse(studentIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid student ID format", false, nil, nil, nil)
	}

	caller := getCaller(c)
	if err := h.service.RevokeAssignmentByExamAndStudent(c.Request().Context(), examID, studentID, caller); err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Student assignment successfully revoked from exam",
	})
}

func (h *Handler) mapError(err error) error {
	if errors.Is(err, ErrAssignmentNotFound) || errors.Is(err, exam.ErrExamNotFound) || errors.Is(err, ErrStudentNotFound) {
		return httperrs.NewNotFoundError(err.Error(), true, nil)
	}
	if errors.Is(err, ErrUnauthorized) {
		return httperrs.NewForbiddenError("Forbidden: you are not authorized to perform this assignment action", false)
	}
	if errors.Is(err, ErrDuplicateAssignment) {
		return httperrs.NewConflictError(err.Error(), true)
	}
	if errors.Is(err, ErrExamNotPublished) ||
		errors.Is(err, ErrExamArchived) ||
		errors.Is(err, ErrAssignmentRevoked) ||
		errors.Is(err, ErrAssignmentAlreadyRevoked) ||
		errors.Is(err, ErrInvalidAssignmentState) ||
		errors.Is(err, ErrStudentIneligible) ||
		errors.Is(err, ErrNoStudentsProvided) ||
		errors.Is(err, ErrBatchTooLarge) ||
		errors.Is(err, ErrMismatchedStudent) {
		return httperrs.NewBadRequestError(err.Error(), false, nil, nil, nil)
	}
	return httperrs.NewInternalServerError()
}

func getCaller(c echo.Context) Caller {
	userIDStr, _ := c.Get("user_id").(string)
	id, _ := uuid.Parse(userIDStr)
	roleStr, _ := c.Get("user_role").(string)
	return Caller{
		ID:   id,
		Role: platformauth.Role(strings.ToUpper(roleStr)),
	}
}

func parsePositiveInt(val string, defaultVal int) int {
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		return defaultVal
	}
	if n > 100 {
		return 100
	}
	return n
}

func parseNonNegativeInt(val string, defaultVal int) int {
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil || n < 0 {
		return defaultVal
	}
	return n
}
