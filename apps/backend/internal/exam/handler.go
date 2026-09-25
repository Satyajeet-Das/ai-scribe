package exam

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

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
	exams := g.Group("/exams")
	if authMiddleware != nil {
		exams.Use(authMiddleware)
	}

	exams.GET("", h.ListExams)
	exams.POST("", h.CreateExam)
	exams.GET("/:id", h.GetExam)
	exams.PATCH("/:id", h.UpdateExam)
	exams.POST("/:id/publish", h.PublishExam)
	exams.POST("/:id/archive", h.ArchiveExam)
}

func (h *Handler) GetExam(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	exam, err := h.service.GetExam(c.Request().Context(), id)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, ToExamResponse(exam))
}

func (h *Handler) ListExams(c echo.Context) error {
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

	var statusFilter *Status
	if s := c.QueryParam("status"); s != "" {
		st := Status(s)
		statusFilter = &st
	}

	exams, total, err := h.service.ListExams(c.Request().Context(), limit, offset, statusFilter)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":   ToExamResponseList(exams),
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) CreateExam(c echo.Context) error {
	var req CreateExamRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	userIDStr, _ := c.Get("user_id").(string)
	createdBy, _ := uuid.Parse(userIDStr)

	exam, err := h.service.CreateExam(c.Request().Context(), req, createdBy)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusCreated, ToExamResponse(exam))
}

func (h *Handler) UpdateExam(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	var req UpdateExamRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	userIDStr, _ := c.Get("user_id").(string)
	callerID, _ := uuid.Parse(userIDStr)

	exam, err := h.service.UpdateExam(c.Request().Context(), id, req, callerID)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, ToExamResponse(exam))
}

func (h *Handler) PublishExam(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	userIDStr, _ := c.Get("user_id").(string)
	callerID, _ := uuid.Parse(userIDStr)

	exam, err := h.service.PublishExam(c.Request().Context(), id, callerID)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, ToExamResponse(exam))
}

func (h *Handler) ArchiveExam(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	userIDStr, _ := c.Get("user_id").(string)
	callerID, _ := uuid.Parse(userIDStr)

	exam, err := h.service.ArchiveExam(c.Request().Context(), id, callerID)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, ToExamResponse(exam))
}

func (h *Handler) mapError(err error) error {
	if errors.Is(err, ErrExamNotFound) {
		return httperrs.NewNotFoundError("Exam not found", true, nil)
	}
	if errors.Is(err, ErrUnauthorizedCreator) {
		return httperrs.NewForbiddenError("Forbidden: only the exam creator can perform this action", false)
	}
	if errors.Is(err, ErrExamAlreadyPublished) ||
		errors.Is(err, ErrExamAlreadyArchived) ||
		errors.Is(err, ErrExamNotDraft) ||
		errors.Is(err, ErrInvalidExamState) ||
		errors.Is(err, ErrExamCannotPublish) {
		return httperrs.NewBadRequestError(err.Error(), false, nil, nil, nil)
	}
	return httperrs.NewInternalServerError()
}
