package question

import (
	"errors"
	"net/http"
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
	// Nested under exams: /api/v1/exams/:exam_id/questions
	examQuestions := g.Group("/exams/:exam_id/questions")
	if authMiddleware != nil {
		examQuestions.Use(authMiddleware)
	}
	examQuestions.POST("", h.CreateQuestion)
	examQuestions.GET("", h.ListQuestionsByExam)
	examQuestions.PUT("/reorder", h.ReorderQuestions)
	examQuestions.POST("/reorder", h.ReorderQuestions)

	// Under questions: /api/v1/questions
	questions := g.Group("/questions")
	if authMiddleware != nil {
		questions.Use(authMiddleware)
	}
	questions.GET("/:id", h.GetQuestion)
	questions.PUT("/:id", h.UpdateQuestion)
	questions.PATCH("/:id", h.UpdateQuestion)
	questions.DELETE("/:id", h.DeleteQuestion)
	questions.POST("/:id/options", h.CreateOption)
	questions.GET("/:id/options", h.ListOptions)
	questions.PUT("/:id/options/:option_id", h.UpdateOption)
	questions.PATCH("/:id/options/:option_id", h.UpdateOption)
	questions.DELETE("/:id/options/:option_id", h.DeleteOption)
	questions.POST("/:id/options/:option_id/correct", h.SetCorrectOption)
	questions.PUT("/:id/options/:option_id/correct", h.SetCorrectOption)
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

	caller := getCaller(c)
	q, err := h.service.CreateQuestion(c.Request().Context(), examID, req, caller)
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

	caller := getCaller(c)
	questions, err := h.service.ListQuestionsByExamForCaller(c.Request().Context(), examID, caller)
	if err != nil {
		return h.mapError(err)
	}

	if isTeacherOrAdmin(caller) {
		return c.JSON(http.StatusOK, ToTeacherQuestionListResponse(questions))
	}
	return c.JSON(http.StatusOK, ToStudentQuestionListResponse(questions))
}

func (h *Handler) GetQuestion(c echo.Context) error {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid question ID format", false, nil, nil, nil)
	}

	caller := getCaller(c)
	q, err := h.service.GetQuestionForCaller(c.Request().Context(), id, caller)
	if err != nil {
		return h.mapError(err)
	}

	if isTeacherOrAdmin(caller) {
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

	caller := getCaller(c)
	q, err := h.service.UpdateQuestion(c.Request().Context(), id, req, caller)
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

	caller := getCaller(c)
	if err := h.service.DeleteQuestion(c.Request().Context(), id, caller); err != nil {
		return h.mapError(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) ReorderQuestions(c echo.Context) error {
	examIDParam := c.Param("exam_id")
	examID, err := uuid.Parse(examIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	var req ReorderQuestionsRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	caller := getCaller(c)
	questions, err := h.service.ReorderQuestions(c.Request().Context(), examID, req, caller)
	if err != nil {
		return h.mapError(err)
	}

	if isTeacherOrAdmin(caller) {
		return c.JSON(http.StatusOK, ToTeacherQuestionListResponse(questions))
	}
	return c.JSON(http.StatusOK, ToStudentQuestionListResponse(questions))
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

	caller := getCaller(c)
	opt, err := h.service.CreateOption(c.Request().Context(), questionID, req, caller)
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

	caller := getCaller(c)
	options, err := h.service.ListOptionsForCaller(c.Request().Context(), questionID, caller)
	if err != nil {
		return h.mapError(err)
	}

	if isTeacherOrAdmin(caller) {
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

func (h *Handler) UpdateOption(c echo.Context) error {
	questionIDParam := c.Param("id")
	questionID, err := uuid.Parse(questionIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid question ID format", false, nil, nil, nil)
	}

	optionIDParam := c.Param("option_id")
	optionID, err := uuid.Parse(optionIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid option ID format", false, nil, nil, nil)
	}

	var req UpdateOptionRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	caller := getCaller(c)
	opt, err := h.service.UpdateOption(c.Request().Context(), questionID, optionID, req, caller)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, ToTeacherOptionResponse(opt))
}

func (h *Handler) DeleteOption(c echo.Context) error {
	questionIDParam := c.Param("id")
	questionID, err := uuid.Parse(questionIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid question ID format", false, nil, nil, nil)
	}

	optionIDParam := c.Param("option_id")
	optionID, err := uuid.Parse(optionIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid option ID format", false, nil, nil, nil)
	}

	caller := getCaller(c)
	if err := h.service.DeleteOption(c.Request().Context(), questionID, optionID, caller); err != nil {
		return h.mapError(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) SetCorrectOption(c echo.Context) error {
	questionIDParam := c.Param("id")
	questionID, err := uuid.Parse(questionIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid question ID format", false, nil, nil, nil)
	}

	optionIDParam := c.Param("option_id")
	optionID, err := uuid.Parse(optionIDParam)
	if err != nil {
		return httperrs.NewBadRequestError("Invalid option ID format", false, nil, nil, nil)
	}

	caller := getCaller(c)
	opt, err := h.service.SetCorrectOption(c.Request().Context(), questionID, optionID, caller)
	if err != nil {
		return h.mapError(err)
	}

	return c.JSON(http.StatusOK, ToTeacherOptionResponse(opt))
}

func (h *Handler) mapError(err error) error {
	if errors.Is(err, ErrQuestionNotFound) ||
		errors.Is(err, exam.ErrExamNotFound) ||
		errors.Is(err, ErrOptionNotFound) {
		return httperrs.NewNotFoundError(err.Error(), true, nil)
	}
	if errors.Is(err, ErrUnauthorizedCreator) {
		return httperrs.NewForbiddenError("Forbidden: only the exam creator or admin can perform this action", false)
	}
	if errors.Is(err, ErrExamNotDraft) ||
		errors.Is(err, ErrExamArchived) ||
		errors.Is(err, ErrInvalidQuestionType) ||
		errors.Is(err, ErrOptionKeyDuplicate) ||
		errors.Is(err, ErrOptionsOnlyForMCQ) ||
		errors.Is(err, ErrQuestionDoesNotBelongToExam) ||
		errors.Is(err, ErrOptionDoesNotBelongToQuestion) ||
		errors.Is(err, ErrInvalidQuestionOrder) ||
		errors.Is(err, ErrDuplicateQuestionNumbers) ||
		errors.Is(err, ErrAtLeastOneCorrectOption) ||
		errors.Is(err, ErrMultipleCorrectOptions) {
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

func isTeacherOrAdmin(caller Caller) bool {
	return caller.IsTeacher() || caller.IsAdmin()
}
