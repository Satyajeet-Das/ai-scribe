package exam

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

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

func (h *Handler) RegisterRoutes(g *echo.Group, authMiddleware echo.MiddlewareFunc, roleMiddleware ...echo.MiddlewareFunc) {
	exams := g.Group("/exams")
	if authMiddleware != nil {
		exams.Use(authMiddleware)
	}
	for _, m := range roleMiddleware {
		if m != nil {
			exams.Use(m)
		}
	}

	exams.GET("", h.ListExams)
	exams.POST("", h.CreateExam)
	exams.GET("/:id", h.GetExam)
	exams.PUT("/:id", h.UpdateExam)
	exams.PATCH("/:id", h.UpdateExam)
	exams.DELETE("/:id", h.DeleteExam)
	exams.POST("/:id/publish", h.PublishExam)
	exams.POST("/:id/unpublish", h.UnpublishExam)
	exams.POST("/:id/archive", h.ArchiveExam)
}

func (h *Handler) getCaller(c echo.Context) (Caller, error) {
	userID, err := platformauth.GetCallerUserID(c)
	if err != nil {
		return Caller{}, httperrs.NewUnauthorizedError("Authentication required", false)
	}
	role := platformauth.GetCallerRole(c)
	return Caller{
		ID:   userID,
		Role: role,
	}, nil
}

func (h *Handler) GetExam(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	caller, err := h.getCaller(c)
	if err != nil {
		return err
	}

	exam, err := h.service.GetExamForCaller(c.Request().Context(), id, caller)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, ToExamResponse(exam))
}

func (h *Handler) ListExams(c echo.Context) error {
	caller, err := h.getCaller(c)
	if err != nil {
		return err
	}

	params := ListExamsParams{
		Limit:  20,
		Offset: 0,
	}

	if l := c.QueryParam("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			params.Limit = val
		}
	}
	if o := c.QueryParam("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			params.Offset = val
		}
	}

	if s := c.QueryParam("status"); s != "" {
		st := Status(s)
		if st.IsValid() {
			params.Status = &st
		}
	}

	if sub := c.QueryParam("subject"); sub != "" {
		params.Subject = sub
	}

	search := c.QueryParam("search")
	if search == "" {
		search = c.QueryParam("q")
	}
	params.Search = search

	if cb := c.QueryParam("createdBy"); cb != "" {
		if parsedCB, err := uuid.Parse(cb); err == nil {
			params.CreatedBy = &parsedCB
		}
	}

	exams, total, err := h.service.ListExams(c.Request().Context(), params, caller)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, ToExamListResponse(exams, total, params.Limit, params.Offset))
}

func (h *Handler) CreateExam(c echo.Context) error {
	var req CreateExamRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	caller, err := h.getCaller(c)
	if err != nil {
		return err
	}

	exam, err := h.service.CreateExam(c.Request().Context(), req, caller)
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

	caller, err := h.getCaller(c)
	if err != nil {
		return err
	}

	exam, err := h.service.UpdateExam(c.Request().Context(), id, req, caller)
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

	caller, err := h.getCaller(c)
	if err != nil {
		return err
	}

	exam, err := h.service.PublishExam(c.Request().Context(), id, caller)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, ToExamResponse(exam))
}

func (h *Handler) UnpublishExam(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	caller, err := h.getCaller(c)
	if err != nil {
		return err
	}

	exam, err := h.service.UnpublishExam(c.Request().Context(), id, caller)
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

	caller, err := h.getCaller(c)
	if err != nil {
		return err
	}

	exam, err := h.service.ArchiveExam(c.Request().Context(), id, caller)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, ToExamResponse(exam))
}

func (h *Handler) DeleteExam(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	caller, err := h.getCaller(c)
	if err != nil {
		return err
	}

	if err := h.service.DeleteExam(c.Request().Context(), id, caller); err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Exam deleted successfully",
	})
}

func (h *Handler) mapError(err error) error {
	if errors.Is(err, ErrExamNotFound) {
		return httperrs.NewNotFoundError("Exam not found", true, nil)
	}
	if errors.Is(err, ErrUnauthorizedCreator) || errors.Is(err, ErrInvalidCallerRole) {
		return httperrs.NewForbiddenError("Forbidden: insufficient permissions to manage this exam", false)
	}
	if errors.Is(err, ErrExamAlreadyPublished) ||
		errors.Is(err, ErrExamAlreadyArchived) ||
		errors.Is(err, ErrExamNotDraft) ||
		errors.Is(err, ErrExamNotPublished) ||
		errors.Is(err, ErrInvalidExamState) ||
		errors.Is(err, ErrExamCannotPublish) ||
		errors.Is(err, ErrPublishedStructuralChange) ||
		errors.Is(err, ErrCannotUnpublishActiveExam) ||
		errors.Is(err, ErrCannotDeleteActiveExam) {
		return httperrs.NewBadRequestError(err.Error(), false, nil, nil, nil)
	}
	return httperrs.NewInternalServerError()
}
