package question

import (
	"errors"
	"net/http"
	"strings"

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
	// Nested under exams: /api/v1/exams/:exam_id/questions
	examQuestions := g.Group("/exams/:exam_id/questions")
	if authMiddleware != nil {
		examQuestions.Use(authMiddleware)
	}
	examQuestions.POST("", h.CreateQuestion)
	examQuestions.GET("", h.ListQuestionsByExam)

	// Under questions: /api/v1/questions
	questions := g.Group("/questions")
	if authMiddleware != nil {
		questions.Use(authMiddleware)
	}
	questions.GET("/:id", h.GetQuestion)
	questions.PATCH("/:id", h.UpdateQuestion)
	questions.DELETE("/:id", h.DeleteQuestion)
	questions.POST("/:id/options", h.CreateOption)
	questions.GET("/:id/options", h.ListOptions)
}

func (h *Handler) CreateQuestion(c echo.Context) error {
	examIDParam := c.Param("exam_id")
	examID, err := uuid.Parse(examIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	var req CreateQuestionRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	userIDStr, _ := c.Get("user_id").(string)
	callerID, _ := uuid.Parse(userIDStr)

	q, err := h.service.CreateQuestion(c.Request().Context(), examID, req, callerID)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusCreated, ToTeacherQuestionResponse(q))
}

func (h *Handler) ListQuestionsByExam(c echo.Context) error {
	examIDParam := c.Param("exam_id")
	examID, err := uuid.Parse(examIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	questions, err := h.service.ListQuestionsByExam(c.Request().Context(), examID)
	if err != nil {
		return h.mapError(err)
	}

	role, _ := c.Get("user_role").(string)
	if isTeacherOrAdmin(role) {
		return c.JSON(http.StatusOK, ToTeacherQuestionResponseList(questions))
	}
	return c.JSON(http.StatusOK, ToStudentQuestionResponseList(questions))
}

func (h *Handler) GetQuestion(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid question ID format", false, nil, nil, nil)
	}

	q, err := h.service.GetQuestion(c.Request().Context(), id)
	if err != nil {
		return h.mapError(err)
	}

	role, _ := c.Get("user_role").(string)
	if isTeacherOrAdmin(role) {
		return c.JSON(http.StatusOK, ToTeacherQuestionResponse(q))
	}
	return c.JSON(http.StatusOK, ToStudentQuestionResponse(q))
}

func (h *Handler) UpdateQuestion(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid question ID format", false, nil, nil, nil)
	}

	var req UpdateQuestionRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	userIDStr, _ := c.Get("user_id").(string)
	callerID, _ := uuid.Parse(userIDStr)

	q, err := h.service.UpdateQuestion(c.Request().Context(), id, req, callerID)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, ToTeacherQuestionResponse(q))
}

func (h *Handler) DeleteQuestion(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid question ID format", false, nil, nil, nil)
	}

	userIDStr, _ := c.Get("user_id").(string)
	callerID, _ := uuid.Parse(userIDStr)

	if err := h.service.DeleteQuestion(c.Request().Context(), id, callerID); err != nil {
		return h.mapError(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) CreateOption(c echo.Context) error {
	questionIDParam := c.Param("id")
	questionID, err := uuid.Parse(questionIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid question ID format", false, nil, nil, nil)
	}

	var req CreateOptionRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	userIDStr, _ := c.Get("user_id").(string)
	callerID, _ := uuid.Parse(userIDStr)

	opt, err := h.service.CreateOption(c.Request().Context(), questionID, req, callerID)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusCreated, ToTeacherOptionResponse(opt))
}

func (h *Handler) ListOptions(c echo.Context) error {
	questionIDParam := c.Param("id")
	questionID, err := uuid.Parse(questionIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid question ID format", false, nil, nil, nil)
	}

	options, err := h.service.ListOptions(c.Request().Context(), questionID)
	if err != nil {
		return h.mapError(err)
	}

	role, _ := c.Get("user_role").(string)
	if isTeacherOrAdmin(role) {
		res := make([]TeacherQuestionOptionResponse, len(options))
		for i := range options {
			res[i] = ToTeacherOptionResponse(&options[i])
		}
		return c.JSON(http.StatusOK, res)
	}

	res := make([]StudentQuestionOptionResponse, len(options))
	for i := range options {
		res[i] = ToStudentOptionResponse(&options[i])
	}
	return c.JSON(http.StatusOK, res)
}

func (h *Handler) mapError(err error) error {
	if errors.Is(err, ErrQuestionNotFound) || errors.Is(err, exam.ErrExamNotFound) || errors.Is(err, ErrOptionNotFound) {
		return httperrs.NewNotFoundError(err.Error(), true, nil)
	}
	if errors.Is(err, ErrUnauthorizedCreator) {
		return httperrs.NewForbiddenError("Forbidden: only the exam creator can perform this action", false)
	}
	if errors.Is(err, ErrExamNotDraft) ||
		errors.Is(err, ErrExamArchived) ||
		errors.Is(err, ErrInvalidQuestionType) ||
		errors.Is(err, ErrOptionKeyDuplicate) ||
		errors.Is(err, ErrOptionsOnlyForMCQ) {
		return httperrs.NewBadRequestError(err.Error(), false, nil, nil, nil)
	}
	return httperrs.NewInternalServerError()
}

func isTeacherOrAdmin(role string) bool {
	return strings.EqualFold(role, "TEACHER") || strings.EqualFold(role, "ADMIN") || strings.EqualFold(role, "educator")
}
