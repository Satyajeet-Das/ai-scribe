package assignment

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
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
	assignments.POST("", h.CreateAssignment)
	assignments.GET("/:id", h.GetAssignment)
	assignments.POST("/:id/revoke", h.RevokeAssignment)
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

func (h *Handler) ListAssignments(c echo.Context) error {
	limit := 20
	offset := 0

	if l := c.QueryParam("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}
	if o := c.QueryParam("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

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

	assignments, total, err := h.service.ListAssignments(c.Request().Context(), limit, offset, examID, studentID, statusFilter)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":   ToAssignmentResponseList(assignments),
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) CreateAssignment(c echo.Context) error {
	var req CreateAssignmentRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	userIDStr, _ := c.Get("user_id").(string)
	callerID, _ := uuid.Parse(userIDStr)

	a, err := h.service.CreateAssignment(c.Request().Context(), req, callerID)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusCreated, ToAssignmentResponse(a))
}

func (h *Handler) RevokeAssignment(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid assignment ID format", false, nil, nil, nil)
	}

	userIDStr, _ := c.Get("user_id").(string)
	callerID, _ := uuid.Parse(userIDStr)

	if err := h.service.RevokeAssignment(c.Request().Context(), id, callerID); err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Assignment successfully revoked",
	})
}

func (h *Handler) mapError(err error) error {
	if errors.Is(err, ErrAssignmentNotFound) || errors.Is(err, exam.ErrExamNotFound) {
		return httperrs.NewNotFoundError(err.Error(), true, nil)
	}
	if errors.Is(err, ErrUnauthorized) {
		return httperrs.NewForbiddenError("Forbidden: only the exam creator can manage assignments", false)
	}
	if errors.Is(err, ErrExamNotPublished) ||
		errors.Is(err, ErrDuplicateAssignment) ||
		errors.Is(err, ErrAssignmentRevoked) ||
		errors.Is(err, ErrAssignmentAlreadyRevoked) ||
		errors.Is(err, ErrInvalidAssignmentState) {
		return httperrs.NewBadRequestError(err.Error(), false, nil, nil, nil)
	}
	return httperrs.NewInternalServerError()
}
