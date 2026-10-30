package document

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
)

// uploadOverhead is how much a multipart upload may exceed the file itself:
// boundaries, headers and the small form fields around it.
const uploadOverhead = 1 << 20

// Handler serves the exam document endpoints.
type Handler struct {
	service  Service
	maxBytes int64
}

func NewHandler(service Service, maxBytes int64) *Handler {
	return &Handler{service: service, maxBytes: maxBytes}
}

// RegisterRoutes mounts the endpoints under an exam. The role middleware is a
// first gate; the service re-checks the role and that the caller owns the exam.
// uploadLimit, when not nil, is applied to uploads only.
func (h *Handler) RegisterRoutes(g *echo.Group, authMiddleware, roleMiddleware, uploadLimit echo.MiddlewareFunc) {
	docs := g.Group("/exams/:exam_id/documents")
	if authMiddleware != nil {
		docs.Use(authMiddleware)
	}
	if roleMiddleware != nil {
		docs.Use(roleMiddleware)
	}

	if uploadLimit != nil {
		docs.POST("", h.Upload, uploadLimit)
	} else {
		docs.POST("", h.Upload)
	}
	docs.GET("", h.List)
	docs.GET("/:document_id", h.Get)
	docs.GET("/:document_id/file", h.File)
	docs.DELETE("/:document_id", h.Delete)
	docs.POST("/:document_id/cancel", h.Cancel)
	docs.POST("/:document_id/retry", h.Retry)
	docs.POST("/:document_id/approve", h.Approve)

	docs.GET("/:document_id/draft", h.GetDraft)
	docs.POST("/:document_id/draft/questions", h.AddQuestion)
	docs.PUT("/:document_id/draft/questions/:question_id", h.ReplaceQuestion)
	docs.DELETE("/:document_id/draft/questions/:question_id", h.DeleteQuestion)
	docs.PUT("/:document_id/draft/reorder", h.Reorder)
}

// ---- documents ----

// Upload accepts a PDF as multipart/form-data (field "file"). The body is read
// as a stream and never written to a temporary file; a request over the size
// limit is cut off, not read to the end.
func (h *Handler) Upload(c echo.Context) error {
	examID, err := uuid.Parse(c.Param("exam_id"))
	if err != nil {
		return badID("exam")
	}
	mt, _, err := mime.ParseMediaType(c.Request().Header.Get(echo.HeaderContentType))
	if err != nil || mt != "multipart/form-data" {
		return apiError(http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Send the file as multipart/form-data in a field named \"file\".", nil)
	}

	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, h.maxBytes+uploadOverhead)
	mr, err := c.Request().MultipartReader()
	if err != nil {
		return h.readError(err)
	}
	var part *multipart.Part
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return h.readError(err)
		}
		if p.FormName() == "file" && p.FileName() != "" {
			part = p
			break
		}
		_ = p.Close()
	}
	if part == nil {
		return validation("file", "is required: send the PDF in a multipart field named \"file\"")
	}
	defer part.Close()

	d, err := h.service.Upload(c.Request().Context(), examID, part.FileName(), part, getCaller(c))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return h.readError(err)
		}
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusAccepted, toDocumentResponse(d, false))
}

// readError maps a failure to read the upload body.
func (h *Handler) readError(err error) error {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return apiError(http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE",
			fmt.Sprintf("The upload is larger than the %d MB limit.", h.maxBytes>>20), nil)
	}
	return apiError(http.StatusBadRequest, "BAD_REQUEST", "The upload could not be read as multipart/form-data.", nil)
}

func (h *Handler) List(c echo.Context) error {
	examID, err := uuid.Parse(c.Param("exam_id"))
	if err != nil {
		return badID("exam")
	}
	docs, err := h.service.List(c.Request().Context(), examID, getCaller(c))
	if err != nil {
		return h.mapError(err)
	}
	out := DocumentListResponse{Documents: make([]DocumentResponse, 0, len(docs)), Total: len(docs)}
	for i := range docs {
		out.Documents = append(out.Documents, toDocumentResponse(&docs[i], false))
	}
	setNoStore(c)
	return c.JSON(http.StatusOK, out)
}

func (h *Handler) Get(c echo.Context) error {
	examID, docID, err := ids(c)
	if err != nil {
		return err
	}
	d, err := h.service.Get(c.Request().Context(), examID, docID, getCaller(c))
	if err != nil {
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusOK, toDocumentResponse(d, true))
}

// File streams the original PDF to the exam's owner. It is never public: it is
// served through this check, as a download, with no caching.
func (h *Handler) File(c echo.Context) error {
	examID, docID, err := ids(c)
	if err != nil {
		return err
	}
	rc, size, d, err := h.service.OpenFile(c.Request().Context(), examID, docID, getCaller(c))
	if err != nil {
		return h.mapError(err)
	}
	defer rc.Close()

	hd := c.Response().Header()
	hd.Set(echo.HeaderContentDisposition, mime.FormatMediaType("attachment", map[string]string{"filename": d.OriginalFilename}))
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set(echo.HeaderContentLength, strconv.FormatInt(size, 10))
	setNoStore(c)
	return c.Stream(http.StatusOK, "application/pdf", rc)
}

func (h *Handler) Delete(c echo.Context) error {
	examID, docID, err := ids(c)
	if err != nil {
		return err
	}
	if err := h.service.Delete(c.Request().Context(), examID, docID, getCaller(c)); err != nil {
		return h.mapError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) Cancel(c echo.Context) error {
	examID, docID, err := ids(c)
	if err != nil {
		return err
	}
	d, err := h.service.Cancel(c.Request().Context(), examID, docID, getCaller(c))
	if err != nil {
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusOK, toDocumentResponse(d, true))
}

func (h *Handler) Retry(c echo.Context) error {
	examID, docID, err := ids(c)
	if err != nil {
		return err
	}
	var body RetryDocumentRequest
	if c.Request().ContentLength != 0 {
		if err := c.Bind(&body); err != nil {
			return badBody()
		}
	}
	d, err := h.service.Retry(c.Request().Context(), examID, docID,
		RetryRequest{DiscardDraft: body.DiscardDraft, ExpectedDraftVersion: body.ExpectedDraftVersion}, getCaller(c))
	if err != nil {
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusAccepted, toDocumentResponse(d, true))
}

// ---- draft ----

func (h *Handler) GetDraft(c echo.Context) error {
	examID, docID, err := ids(c)
	if err != nil {
		return err
	}
	d, err := h.service.GetDraft(c.Request().Context(), examID, docID, getCaller(c))
	if err != nil {
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusOK, toDraftResponse(d))
}

func (h *Handler) AddQuestion(c echo.Context) error {
	examID, docID, err := ids(c)
	if err != nil {
		return err
	}
	var body AddDraftQuestionRequest
	if err := c.Bind(&body); err != nil {
		return badBody()
	}
	if body.ExpectedDraftVersion == nil {
		return validation("expectedDraftVersion", "is required")
	}
	pos := 0
	if body.Position != nil {
		pos = *body.Position
	}
	res, err := h.service.AddQuestion(c.Request().Context(), examID, docID,
		AddQuestionRequest{ExpectedDraftVersion: *body.ExpectedDraftVersion, Position: pos, Question: body.Question.toInput()}, getCaller(c))
	if err != nil {
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusCreated, DraftQuestionResultResponse{Question: toDraftQuestionResponse(res.Question), DraftVersion: res.DraftVersion})
}

func (h *Handler) ReplaceQuestion(c echo.Context) error {
	examID, docID, err := ids(c)
	if err != nil {
		return err
	}
	qid, err := uuid.Parse(c.Param("question_id"))
	if err != nil {
		return badID("question")
	}
	var body ReplaceDraftQuestionRequest
	if err := c.Bind(&body); err != nil {
		return badBody()
	}
	if body.ExpectedDraftVersion == nil {
		return validation("expectedDraftVersion", "is required")
	}
	res, err := h.service.ReplaceQuestion(c.Request().Context(), examID, docID, qid,
		ReplaceQuestionRequest{ExpectedDraftVersion: *body.ExpectedDraftVersion, Question: body.Question.toInput()}, getCaller(c))
	if err != nil {
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusOK, DraftQuestionResultResponse{Question: toDraftQuestionResponse(res.Question), DraftVersion: res.DraftVersion})
}

func (h *Handler) DeleteQuestion(c echo.Context) error {
	examID, docID, err := ids(c)
	if err != nil {
		return err
	}
	qid, err := uuid.Parse(c.Param("question_id"))
	if err != nil {
		return badID("question")
	}
	raw := c.QueryParam("expectedDraftVersion")
	if raw == "" {
		return validation("expectedDraftVersion", "is required (query parameter)")
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return validation("expectedDraftVersion", "must be a whole number")
	}
	d, err := h.service.DeleteQuestion(c.Request().Context(), examID, docID, qid, v, getCaller(c))
	if err != nil {
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusOK, toDraftResponse(d))
}

func (h *Handler) Reorder(c echo.Context) error {
	examID, docID, err := ids(c)
	if err != nil {
		return err
	}
	var body ReorderDraftRequest
	if err := c.Bind(&body); err != nil {
		return badBody()
	}
	if body.ExpectedDraftVersion == nil {
		return validation("expectedDraftVersion", "is required")
	}
	d, err := h.service.Reorder(c.Request().Context(), examID, docID,
		ReorderRequest{ExpectedDraftVersion: *body.ExpectedDraftVersion, QuestionIDs: body.QuestionIDs}, getCaller(c))
	if err != nil {
		return h.mapError(err)
	}
	setNoStore(c)
	return c.JSON(http.StatusOK, toDraftResponse(d))
}

func (h *Handler) Approve(c echo.Context) error {
	examID, docID, err := ids(c)
	if err != nil {
		return err
	}
	var body ApproveDraftRequest
	if err := c.Bind(&body); err != nil {
		return badBody()
	}
	if body.ExpectedDraftVersion == nil {
		return validation("expectedDraftVersion", "is required")
	}
	res, err := h.service.Approve(c.Request().Context(), examID, docID, *body.ExpectedDraftVersion, getCaller(c))
	if err != nil {
		return h.mapError(err)
	}
	out := ApproveResponse{Document: toDocumentResponse(res.Document, false), Questions: make([]ApprovedQuestionResponse, 0, len(res.Questions))}
	for _, q := range res.Questions {
		out.Questions = append(out.Questions, ApprovedQuestionResponse{ID: q.ID, QuestionNumber: q.QuestionNumber, Type: string(q.Type), Points: q.Points, Text: q.Text})
	}
	setNoStore(c)
	return c.JSON(http.StatusOK, out)
}

// ---- errors ----

func (h *Handler) mapError(err error) error {
	var (
		ve  *ValidationError
		inc *IncompleteError
		pdf *InvalidPDFError
		dup *DuplicateError
	)
	switch {
	case errors.As(err, &ve):
		return validation(ve.Field, ve.Message)
	case errors.As(err, &inc):
		fields := make([]httperrs.FieldError, 0, len(inc.Problems))
		for _, p := range inc.Problems {
			f := p.Field
			if p.Position > 0 {
				f = fmt.Sprintf("questions[%d].%s", p.Position, p.Field)
			}
			fields = append(fields, httperrs.FieldError{Field: f, Error: p.Message})
		}
		return apiError(http.StatusConflict, "DRAFT_INCOMPLETE", "The draft is not ready to approve. Fix the listed problems and try again.", fields)
	case errors.As(err, &pdf):
		return apiError(http.StatusUnprocessableEntity, "INVALID_PDF", invalidPDFMessage(pdf.Reason),
			[]httperrs.FieldError{{Field: "file", Error: pdf.Reason}})
	case errors.As(err, &dup):
		return apiError(http.StatusConflict, "DOCUMENT_ALREADY_UPLOADED", "This file was already uploaded to this exam.",
			[]httperrs.FieldError{{Field: "existingDocumentId", Error: dup.ExistingID.String()}})
	case errors.Is(err, ErrNotFound):
		return httperrs.NewNotFoundError("Not found", true, nil)
	case errors.Is(err, ErrForbiddenRole):
		return httperrs.NewForbiddenError(ErrForbiddenRole.Error(), true)
	case errors.Is(err, ErrExamNotDraft):
		return apiError(http.StatusConflict, "EXAM_NOT_DRAFT", "Questions can only be added to a draft exam.", nil)
	case errors.Is(err, ErrExtractionUnavailable):
		return apiError(http.StatusServiceUnavailable, "EXTRACTION_UNAVAILABLE", "Reading exam documents is not set up on this server.", nil)
	case errors.Is(err, ErrFileTooLarge):
		return apiError(http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", fmt.Sprintf("The file is larger than the %d MB limit.", h.maxBytes>>20), nil)
	case errors.Is(err, ErrUnsupportedType):
		return apiError(http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Only PDF files can be uploaded.", nil)
	case errors.Is(err, ErrTooManyDocuments):
		return apiError(http.StatusConflict, "TOO_MANY_DOCUMENTS", "This exam already has the most documents it can hold. Delete one first.", nil)
	case errors.Is(err, ErrInvalidState):
		return apiError(http.StatusConflict, "INVALID_STATE", "This cannot be done while the document is in its current state.", nil)
	case errors.Is(err, ErrStaleVersion):
		return apiError(http.StatusConflict, "STALE_VERSION", "The draft was changed elsewhere. Load the latest and try again.", nil)
	case errors.Is(err, ErrAlreadyApproved):
		return apiError(http.StatusConflict, "ALREADY_APPROVED", "This draft was already approved into the exam.", nil)
	case errors.Is(err, ErrConcurrentChange):
		return apiError(http.StatusConflict, "CONCURRENT_CHANGE", "The exam's questions changed at the same moment. Try again.", nil)
	case errors.Is(err, ErrDraftTooLarge):
		return apiError(http.StatusConflict, "DRAFT_TOO_LARGE", fmt.Sprintf("A draft can hold at most %d questions.", maxDraftQuestions), nil)
	case errors.Is(err, ErrQueueUnavailable):
		return apiError(http.StatusServiceUnavailable, "QUEUE_UNAVAILABLE", "Processing could not be started right now. Please try again.", nil)
	case errors.Is(err, ErrStorageUnavailable):
		return apiError(http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "The file could not be stored or read right now. Please try again.", nil)
	case errors.Is(err, ErrBusy):
		return apiError(http.StatusServiceUnavailable, "BUSY", "The server is busy reading other documents. Please try again shortly.", nil)
	}
	return httperrs.NewInternalServerError()
}

func invalidPDFMessage(reason string) string {
	switch reason {
	case ReasonEncrypted:
		return "This PDF is password protected. Remove the password and upload it again."
	case ReasonTooManyPages:
		return "This PDF has more pages than can be read at once."
	case ReasonNoPages:
		return "This PDF has no pages."
	case ReasonNotPDF:
		return "This is not a PDF file."
	}
	return "This PDF is damaged or could not be read. Check that the whole file was uploaded."
}

func apiError(status int, code, msg string, fields []httperrs.FieldError) error {
	return &httperrs.HTTPError{Code: code, Message: msg, Status: status, Override: true, Errors: fields}
}

func validation(field, msg string) error {
	code := "VALIDATION_ERROR"
	return httperrs.NewBadRequestError(field+" "+msg, true, &code, []httperrs.FieldError{{Field: field, Error: msg}}, nil)
}

func badID(what string) error {
	return httperrs.NewBadRequestError("Invalid "+what+" ID format", false, nil, nil, nil)
}

func badBody() error {
	return httperrs.NewBadRequestError("Invalid request body", false, nil, nil, nil)
}

func ids(c echo.Context) (examID, docID uuid.UUID, err error) {
	examID, err = uuid.Parse(c.Param("exam_id"))
	if err != nil {
		return uuid.Nil, uuid.Nil, badID("exam")
	}
	docID, err = uuid.Parse(c.Param("document_id"))
	if err != nil {
		return uuid.Nil, uuid.Nil, badID("document")
	}
	return examID, docID, nil
}

// setNoStore keeps exam content out of shared and browser caches.
func setNoStore(c echo.Context) {
	h := c.Response().Header()
	h.Set("Cache-Control", "no-store")
	h.Set("Pragma", "no-cache")
}

func getCaller(c echo.Context) Caller {
	userIDStr, _ := c.Get("user_id").(string)
	id, _ := uuid.Parse(userIDStr)
	roleStr, _ := c.Get("user_role").(string)
	return Caller{ID: id, Role: platformauth.Role(strings.ToUpper(roleStr))}
}
