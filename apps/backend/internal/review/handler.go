package review

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
)

type Handler struct {
	service       Service
	playback      PlaybackService
	playbackLimit echo.MiddlewareFunc
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// WithPlayback adds the endpoint that plays a recording's original audio.
func (h *Handler) WithPlayback(p PlaybackService) *Handler {
	h.playback = p
	return h
}

// WithPlaybackLimit puts mw (a rate limit) in front of the audio route only.
func (h *Handler) WithPlaybackLimit(mw echo.MiddlewareFunc) *Handler {
	h.playbackLimit = mw
	return h
}

// RegisterRoutes mounts the read-only review endpoints. Both are GET only, so
// no route here can change an answer, a session or an exam. The role
// middleware is a first gate; the service re-checks role and ownership.
func (h *Handler) RegisterRoutes(g *echo.Group, authMiddleware echo.MiddlewareFunc, roleMiddleware echo.MiddlewareFunc) {
	responses := g.Group("/exams/:exam_id/responses")
	if authMiddleware != nil {
		responses.Use(authMiddleware)
	}
	if roleMiddleware != nil {
		responses.Use(roleMiddleware)
	}
	responses.GET("", h.ListStudents)
	responses.GET("/:student_id", h.GetStudentResponses)
	responses.GET("/:student_id/questions/:question_id/revisions", h.GetRevisions)
	if h.playback != nil {
		var mw []echo.MiddlewareFunc
		if h.playbackLimit != nil {
			mw = append(mw, h.playbackLimit)
		}
		responses.GET("/:student_id/recordings/:recording_id/audio", h.GetRecordingAudio, mw...)
	}
}

// GetRecordingAudio plays a recording's original audio. Access is checked and
// recorded in the service before a single byte is sent. The response is never
// cached and is served with range support so the player can seek.
func (h *Handler) GetRecordingAudio(c echo.Context) error {
	examID, err := uuid.Parse(c.Param("exam_id"))
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}
	studentID, err := uuid.Parse(c.Param("student_id"))
	if err != nil {
		return httperrs.NewBadRequestError("Invalid student ID format", false, nil, nil, nil)
	}
	recordingID, err := uuid.Parse(c.Param("recording_id"))
	if err != nil {
		return httperrs.NewBadRequestError("Invalid recording ID format", false, nil, nil, nil)
	}

	meta := AccessMeta{RequestID: c.Response().Header().Get(echo.HeaderXRequestID), IP: c.RealIP()}
	audio, err := h.playback.OpenRecording(c.Request().Context(), examID, studentID, recordingID, getCaller(c), meta)
	if err != nil {
		return h.mapError(err)
	}
	defer audio.Body.Close()

	data, err := io.ReadAll(audio.Body) // already in memory, see OpenRecording
	if err != nil {
		return h.mapError(ErrAudioUnavailable)
	}

	setNoStore(c)
	hdr := c.Response().Header()
	hdr.Set("Content-Type", audio.ContentType)
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("Content-Disposition", `inline; filename="response`+audioExtension(audio.ContentType)+`"`)
	http.ServeContent(c.Response(), c.Request(), "", time.Time{}, bytes.NewReader(data))
	return nil
}

func (h *Handler) ListStudents(c echo.Context) error {
	examID, err := uuid.Parse(c.Param("exam_id"))
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}

	q := ListQuery{
		Search: c.QueryParam("search"),
		Status: c.QueryParam("status"),
		Sort:   c.QueryParam("sort"),
		Order:  c.QueryParam("order"),
	}
	if q.Search == "" {
		q.Search = c.QueryParam("q")
	}
	if q.Limit, err = intParam(c, "limit"); err != nil {
		return httperrs.NewBadRequestError("limit must be a whole number", false, nil, nil, nil)
	}
	if q.Offset, err = intParam(c, "offset"); err != nil {
		return httperrs.NewBadRequestError("offset must be a whole number", false, nil, nil, nil)
	}

	list, err := h.service.ListStudents(c.Request().Context(), examID, q, getCaller(c))
	if err != nil {
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusOK, ToStudentListResponse(list))
}

func (h *Handler) GetStudentResponses(c echo.Context) error {
	examID, err := uuid.Parse(c.Param("exam_id"))
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}
	studentID, err := uuid.Parse(c.Param("student_id"))
	if err != nil {
		return httperrs.NewBadRequestError("Invalid student ID format", false, nil, nil, nil)
	}

	res, err := h.service.GetStudentResponses(c.Request().Context(), examID, studentID, getCaller(c))
	if err != nil {
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusOK, ToStudentResponsesResponse(res))
}

// GetRevisions is the history of one answer, for checking how it came to be.
func (h *Handler) GetRevisions(c echo.Context) error {
	examID, err := uuid.Parse(c.Param("exam_id"))
	if err != nil {
		return httperrs.NewBadRequestError("Invalid exam ID format", false, nil, nil, nil)
	}
	studentID, err := uuid.Parse(c.Param("student_id"))
	if err != nil {
		return httperrs.NewBadRequestError("Invalid student ID format", false, nil, nil, nil)
	}
	questionID, err := uuid.Parse(c.Param("question_id"))
	if err != nil {
		return httperrs.NewBadRequestError("Invalid question ID format", false, nil, nil, nil)
	}

	res, err := h.service.GetRevisions(c.Request().Context(), examID, studentID, questionID, getCaller(c))
	if err != nil {
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusOK, ToRevisionHistoryResponse(res))
}

// setNoStore keeps student answers out of shared and browser caches.
func setNoStore(c echo.Context) {
	h := c.Response().Header()
	h.Set("Cache-Control", "no-store")
	h.Set("Pragma", "no-cache")
}

// audioExtension is the file extension for a recording's content type, for a
// download name that names nothing about the student.
func audioExtension(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])) {
	case "audio/wav", "audio/wave", "audio/x-wav":
		return ".wav"
	case "audio/webm":
		return ".webm"
	case "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	default:
		return ""
	}
}

func intParam(c echo.Context, name string) (int, error) {
	v := strings.TrimSpace(c.QueryParam(name))
	if v == "" {
		return 0, nil
	}
	return strconv.Atoi(v)
}

func (h *Handler) mapError(err error) error {
	switch {
	case errors.Is(err, ErrExamNotFound), errors.Is(err, ErrStudentNotAssigned), errors.Is(err, ErrQuestionNotFound):
		return httperrs.NewNotFoundError(err.Error(), true, nil)
	case errors.Is(err, ErrRecordingNotFound), errors.Is(err, ErrPlaybackDisabled):
		code := "RECORDING_NOT_FOUND"
		return httperrs.NewNotFoundError("Recording not found", true, &code)
	case errors.Is(err, ErrRecordingRemoved):
		return &httperrs.HTTPError{Code: "RECORDING_REMOVED", Message: "This recording has been removed. The transcript is still available.", Status: http.StatusGone, Override: true}
	case errors.Is(err, ErrAudioUnavailable), errors.Is(err, ErrAuditUnavailable):
		// For an audit failure the real reason is deliberately not disclosed.
		return &httperrs.HTTPError{Code: "AUDIO_UNAVAILABLE", Message: "The recording cannot be played right now. Please try again.", Status: http.StatusServiceUnavailable, Override: true}
	case errors.Is(err, ErrForbiddenRole):
		return httperrs.NewForbiddenError(ErrForbiddenRole.Error(), true)
	case errors.Is(err, ErrInvalidQuery):
		return httperrs.NewBadRequestError(err.Error(), true, nil, nil, nil)
	}
	return httperrs.NewInternalServerError()
}

func getCaller(c echo.Context) Caller {
	userIDStr, _ := c.Get("user_id").(string)
	id, _ := uuid.Parse(userIDStr)
	roleStr, _ := c.Get("user_role").(string)
	return Caller{ID: id, Role: platformauth.Role(strings.ToUpper(roleStr))}
}
