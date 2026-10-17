package grading

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
)

// Handler serves the marking endpoints.
type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler { return &Handler{service: service} }

// RegisterRoutes mounts the marking endpoints beside the response review, under
// the same exam and student. The role middleware is a first gate; the service
// re-checks role and ownership.
func (h *Handler) RegisterRoutes(g *echo.Group, authMiddleware echo.MiddlewareFunc, roleMiddleware echo.MiddlewareFunc) {
	marking := g.Group("/exams/:exam_id/responses/:student_id/marking")
	if authMiddleware != nil {
		marking.Use(authMiddleware)
	}
	if roleMiddleware != nil {
		marking.Use(roleMiddleware)
	}
	marking.GET("", h.Get)
	marking.PUT("/questions/:question_id", h.SaveMark)
	marking.POST("/finalise", h.Finalise)
	marking.POST("/reopen", h.Reopen)
}

// ---- Wire format ----

type paperDTO struct {
	Status         Status     `json:"status"`
	Version        int        `json:"version"`
	TotalMarks     float64    `json:"totalMarks"`
	MaxMarks       float64    `json:"maxMarks"`
	MarkedCount    int        `json:"markedCount"`
	TotalQuestions int        `json:"totalQuestions"`
	FinalisedAt    *time.Time `json:"finalisedAt,omitempty"`
}

type lineDTO struct {
	QuestionID      uuid.UUID  `json:"questionId"`
	QuestionNumber  int        `json:"questionNumber"`
	Type            string     `json:"type"`
	MaxMarks        float64    `json:"maxMarks"`
	Answered        bool       `json:"answered"`
	CorrectOptionID *uuid.UUID `json:"correctOptionId,omitempty"`
	SuggestedMarks  *float64   `json:"suggestedMarks"`
	TeacherMarks    *float64   `json:"teacherMarks"`
	EffectiveMarks  *float64   `json:"effectiveMarks"`
	MarkSource      MarkSource `json:"markSource"`
	Comment         *string    `json:"comment"`
	Version         int        `json:"version"`
	ExceedsMax      bool       `json:"exceedsMax"`
}

type markingDTO struct {
	SessionID      *uuid.UUID `json:"sessionId"`
	SessionStatus  string     `json:"sessionStatus,omitempty"`
	MarkingAllowed bool       `json:"markingAllowed"`
	Reason         string     `json:"reason,omitempty"`
	Paper          paperDTO   `json:"paper"`
	Questions      []lineDTO  `json:"questions"`
}

type saveMarkResponse struct {
	Question lineDTO  `json:"question"`
	Paper    paperDTO `json:"paper"`
}

func marksPtr(half *int) *float64 {
	if half == nil {
		return nil
	}
	m := Marks(*half)
	return &m
}

func toPaperDTO(p PaperSummary) paperDTO {
	return paperDTO{
		Status: p.Status, Version: p.Version, TotalMarks: Marks(p.TotalHalf), MaxMarks: Marks(p.MaxHalf),
		MarkedCount: p.Marked, TotalQuestions: p.Questions, FinalisedAt: p.FinalisedAt,
	}
}

func toLineDTO(l Line) lineDTO {
	return lineDTO{
		QuestionID: l.QuestionID, QuestionNumber: l.QuestionNumber, Type: l.Type,
		MaxMarks: Marks(l.MaxHalf), Answered: l.Answered, CorrectOptionID: l.CorrectOptionID,
		SuggestedMarks: marksPtr(l.SuggestedHalf), TeacherMarks: marksPtr(l.TeacherHalf),
		EffectiveMarks: marksPtr(l.EffectiveHalf), MarkSource: l.Source, Comment: l.Comment,
		Version: l.Version, ExceedsMax: l.ExceedsMax,
	}
}

func toMarkingDTO(m *Marking) markingDTO {
	out := markingDTO{
		SessionID: m.SessionID, MarkingAllowed: m.MarkingAllowed, Reason: m.Reason,
		Paper: toPaperDTO(m.Paper), Questions: make([]lineDTO, 0, len(m.Lines)),
	}
	if m.SessionID != nil {
		out.SessionStatus = string(m.SessionStatus)
	}
	for _, l := range m.Lines {
		out.Questions = append(out.Questions, toLineDTO(l))
	}
	return out
}

type saveMarkBody struct {
	SessionID uuid.UUID `json:"sessionId"`
	// Marks is a JSON number, kept as written so "7.25" or "1e1" can be refused
	// rather than rounded. null clears the teacher's mark.
	Marks           *json.Number `json:"marks"`
	Comment         *string      `json:"comment"`
	ExpectedVersion int          `json:"expectedVersion"`
}

type finaliseBody struct {
	SessionID            uuid.UUID `json:"sessionId"`
	ExpectedPaperVersion int       `json:"expectedPaperVersion"`
}

type reopenBody struct {
	SessionID uuid.UUID `json:"sessionId"`
	Reason    string    `json:"reason"`
}

// ---- Endpoints ----

func (h *Handler) Get(c echo.Context) error {
	examID, studentID, err := ids(c)
	if err != nil {
		return err
	}
	m, err := h.service.GetMarking(c.Request().Context(), examID, studentID, getCaller(c))
	if err != nil {
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusOK, toMarkingDTO(m))
}

func (h *Handler) SaveMark(c echo.Context) error {
	examID, studentID, err := ids(c)
	if err != nil {
		return err
	}
	questionID, err := uuid.Parse(c.Param("question_id"))
	if err != nil {
		return httperrs.NewBadRequestError("Invalid question ID format", false, nil, nil, nil)
	}
	var body saveMarkBody
	if err := c.Bind(&body); err != nil {
		return httperrs.NewBadRequestError("Invalid request body", false, nil, nil, nil)
	}
	if body.SessionID == uuid.Nil {
		return validation("sessionId", "is required")
	}
	req := SaveMarkRequest{SessionID: body.SessionID, Comment: body.Comment, ExpectedVersion: body.ExpectedVersion}
	if body.Marks != nil {
		half, err := ParseHalfMarks(body.Marks.String())
		if err != nil {
			return validation("marks", validationText(err))
		}
		req.MarksHalf = &half
	}

	res, err := h.service.SaveMark(c.Request().Context(), examID, studentID, questionID, req, getCaller(c), meta(c))
	if err != nil {
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusOK, saveMarkResponse{Question: toLineDTO(res.Line), Paper: toPaperDTO(res.Paper)})
}

func (h *Handler) Finalise(c echo.Context) error {
	examID, studentID, err := ids(c)
	if err != nil {
		return err
	}
	var body finaliseBody
	if err := c.Bind(&body); err != nil {
		return httperrs.NewBadRequestError("Invalid request body", false, nil, nil, nil)
	}
	if body.SessionID == uuid.Nil {
		return validation("sessionId", "is required")
	}
	m, err := h.service.Finalise(c.Request().Context(), examID, studentID,
		FinaliseRequest{SessionID: body.SessionID, ExpectedPaperVersion: body.ExpectedPaperVersion}, getCaller(c), meta(c))
	if err != nil {
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusOK, toMarkingDTO(m))
}

func (h *Handler) Reopen(c echo.Context) error {
	examID, studentID, err := ids(c)
	if err != nil {
		return err
	}
	var body reopenBody
	if err := c.Bind(&body); err != nil {
		return httperrs.NewBadRequestError("Invalid request body", false, nil, nil, nil)
	}
	if body.SessionID == uuid.Nil {
		return validation("sessionId", "is required")
	}
	m, err := h.service.Reopen(c.Request().Context(), examID, studentID,
		ReopenRequest{SessionID: body.SessionID, Reason: body.Reason}, getCaller(c), meta(c))
	if err != nil {
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusOK, toMarkingDTO(m))
}

// ---- Helpers ----

func ids(c echo.Context) (examID, studentID uuid.UUID, err error) {
	examID, err = uuid.Parse(c.Param("exam_id"))
	if err != nil {
		return uuid.Nil, uuid.Nil, httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}
	studentID, err = uuid.Parse(c.Param("student_id"))
	if err != nil {
		return uuid.Nil, uuid.Nil, httperrs.NewBadRequestError("Invalid student ID format", false, nil, nil, nil)
	}
	return examID, studentID, nil
}

func meta(c echo.Context) AccessMeta {
	return AccessMeta{RequestID: c.Response().Header().Get(echo.HeaderXRequestID), IP: c.RealIP()}
}

func validation(field, msg string) error {
	code := "VALIDATION_ERROR"
	return httperrs.NewBadRequestError(field+" "+msg, true, &code, []httperrs.FieldError{{Field: field, Error: msg}}, nil)
}

// setNoStore keeps student answers and marks out of shared and browser caches.
func setNoStore(c echo.Context) {
	h := c.Response().Header()
	h.Set("Cache-Control", "no-store")
	h.Set("Pragma", "no-cache")
}

func conflict(code, msg string, fields []httperrs.FieldError) error {
	return &httperrs.HTTPError{Code: code, Message: msg, Status: http.StatusConflict, Override: true, Errors: fields}
}

func (h *Handler) mapError(err error) error {
	var ve *ValidationError
	var inc *IncompleteError
	switch {
	case errors.As(err, &ve):
		return validation(ve.Field, ve.Message)
	case errors.Is(err, ErrExamNotFound):
		return httperrs.NewNotFoundError("Not found", true, nil)
	case errors.Is(err, ErrForbiddenRole):
		return httperrs.NewForbiddenError(ErrForbiddenRole.Error(), true)
	case errors.Is(err, ErrStaleVersion):
		return conflict("STALE_VERSION", "This was changed elsewhere. Load the latest and try again.", nil)
	case errors.Is(err, ErrPaperFinalised):
		return conflict("PAPER_FINALISED", "This paper is finalised. Reopen it to change marks.", nil)
	case errors.Is(err, ErrSessionNotEnded):
		return conflict("SESSION_NOT_ENDED", "The student has not finished this exam yet.", nil)
	case errors.Is(err, ErrSessionChanged):
		return conflict("SESSION_CHANGED", "A different attempt is now shown for this student. Reload the paper.", nil)
	case errors.As(err, &inc):
		fields := make([]httperrs.FieldError, 0, len(inc.Questions))
		nums := make([]string, 0, len(inc.Questions))
		for _, n := range inc.Questions {
			fields = append(fields, httperrs.FieldError{Field: "questionNumber", Error: fmt.Sprint(n)})
			nums = append(nums, fmt.Sprint(n))
		}
		return conflict("PAPER_INCOMPLETE", "Every question needs a mark before finalising. Missing: "+strings.Join(nums, ", "), fields)
	case errors.Is(err, ErrAuditUnavailable):
		// The real reason is deliberately not disclosed.
		return &httperrs.HTTPError{Code: "AUDIT_UNAVAILABLE", Message: "The change could not be saved right now. Please try again.", Status: http.StatusServiceUnavailable, Override: true}
	}
	return httperrs.NewInternalServerError()
}

func getCaller(c echo.Context) Caller {
	userIDStr, _ := c.Get("user_id").(string)
	id, _ := uuid.Parse(userIDStr)
	roleStr, _ := c.Get("user_role").(string)
	return Caller{ID: id, Role: platformauth.Role(strings.ToUpper(roleStr))}
}
