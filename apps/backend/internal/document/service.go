package document

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/pdf"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
)

// ExamAuthorizer is the exam module's ownership check.
type ExamAuthorizer interface {
	GetExamForCaller(ctx context.Context, id uuid.UUID, caller exam.Caller) (*exam.Exam, error)
}

// QuestionImporter adds reviewed questions to an exam inside a transaction.
type QuestionImporter interface {
	ImportQuestions(ctx context.Context, tx database.DBTX, examID uuid.UUID, items []question.ImportItem) ([]question.Question, error)
}

// Queue hands a document to the background workers.
type Queue interface {
	EnqueueExtract(documentID uuid.UUID, attempt int) error
}

// Extractor turns a paper into draft questions.
type Extractor interface {
	Name() string
	PromptVersion() string
	CanReadImages() bool
	Extract(ctx context.Context, in extract.Input, progress func(done, total int)) (*extract.Result, error)
}

// Options are the limits and timings a Service runs with.
type Options struct {
	MaxBytes     int64
	MaxPages     int
	ParseTimeout time.Duration
	// Lease is how long a worker owns a document without renewing it.
	Lease time.Duration
	// MaxConcurrent is how many documents this process works on at once.
	MaxConcurrent int
	// Active is false while no extraction provider is configured.
	Active bool
	// MaxLivePerExam bounds the documents an exam can hold.
	MaxLivePerExam int
	// Vision limits: pictures are the expensive part of a document.
	MaxVisionPages int
	// MaxImagesPerPage is how many pictures of one page are sent to a vision model.
	MaxImagesPerPage int
	MaxImageBytes    int64
	MaxVisionBudget  int64
}

func (o Options) withDefaults() Options {
	if o.MaxBytes <= 0 {
		o.MaxBytes = 20 << 20
	}
	if o.MaxPages <= 0 {
		o.MaxPages = 60
	}
	if o.ParseTimeout <= 0 {
		o.ParseTimeout = 30 * time.Second
	}
	if o.Lease <= 0 {
		o.Lease = 2 * time.Minute
	}
	if o.MaxConcurrent <= 0 {
		o.MaxConcurrent = 2
	}
	if o.MaxLivePerExam <= 0 {
		o.MaxLivePerExam = 10
	}
	if o.MaxImagesPerPage <= 0 {
		o.MaxImagesPerPage = 3
	}
	if o.MaxVisionPages <= 0 {
		o.MaxVisionPages = 25
	}
	if o.MaxImageBytes <= 0 {
		o.MaxImageBytes = 2 << 20
	}
	if o.MaxVisionBudget <= 0 {
		o.MaxVisionBudget = 24 << 20
	}
	return o
}

// Deps are what a Service needs.
type Deps struct {
	Repo      Repository
	Exams     ExamAuthorizer
	Questions QuestionImporter
	Tx        database.TxManager
	Store     audiostore.Store
	Extractor Extractor
	Queue     Queue
	Options   Options
	Logger    *zerolog.Logger
}

// RetryRequest says how to run a document again.
type RetryRequest struct {
	// DiscardDraft allows re-running a document that already has a draft to
	// review, throwing the draft away. ExpectedDraftVersion must then match, so a
	// draft someone else is editing is not discarded unseen.
	DiscardDraft         bool
	ExpectedDraftVersion int
}

// Service manages exam documents from upload to approval.
type Service interface {
	Upload(ctx context.Context, examID uuid.UUID, filename string, body io.Reader, caller Caller) (*Document, error)
	List(ctx context.Context, examID uuid.UUID, caller Caller) ([]Document, error)
	Get(ctx context.Context, examID, documentID uuid.UUID, caller Caller) (*Document, error)
	// OpenFile streams the original PDF to its owner.
	OpenFile(ctx context.Context, examID, documentID uuid.UUID, caller Caller) (io.ReadCloser, int64, *Document, error)
	Cancel(ctx context.Context, examID, documentID uuid.UUID, caller Caller) (*Document, error)
	Retry(ctx context.Context, examID, documentID uuid.UUID, req RetryRequest, caller Caller) (*Document, error)
	Delete(ctx context.Context, examID, documentID uuid.UUID, caller Caller) error

	GetDraft(ctx context.Context, examID, documentID uuid.UUID, caller Caller) (*Draft, error)
	AddQuestion(ctx context.Context, examID, documentID uuid.UUID, req AddQuestionRequest, caller Caller) (*QuestionResult, error)
	ReplaceQuestion(ctx context.Context, examID, documentID, questionID uuid.UUID, req ReplaceQuestionRequest, caller Caller) (*QuestionResult, error)
	DeleteQuestion(ctx context.Context, examID, documentID, questionID uuid.UUID, expectedVersion int, caller Caller) (*Draft, error)
	Reorder(ctx context.Context, examID, documentID uuid.UUID, req ReorderRequest, caller Caller) (*Draft, error)
	// Approve copies the reviewed draft into the exam. It is the only way
	// extracted content reaches the exam's questions.
	Approve(ctx context.Context, examID, documentID uuid.UUID, expectedVersion int, caller Caller) (*ApproveResult, error)

	// Process runs the extraction job for one attempt. final is true on the
	// job's last delivery, when a transient failure can no longer be retried.
	Process(ctx context.Context, documentID uuid.UUID, attempt int, final bool) error
}

type service struct {
	repo      Repository
	exams     ExamAuthorizer
	questions QuestionImporter
	tx        database.TxManager
	store     audiostore.Store
	extractor Extractor
	queue     Queue
	opts      Options
	gate      *pdf.Gate
	log       *zerolog.Logger
}

func NewService(d Deps) Service {
	opts := d.Options.withDefaults()
	return &service{
		repo: d.Repo, exams: d.Exams, questions: d.Questions, tx: d.Tx, store: d.Store,
		extractor: d.Extractor, queue: d.Queue, opts: opts,
		gate: pdf.NewGate(opts.MaxConcurrent), log: d.Logger,
	}
}

func (c Caller) canManage() bool {
	switch c.Role {
	case platformauth.RoleAdmin, platformauth.RoleTeacher:
		return true
	}
	return false
}

func (c Caller) examCaller() exam.Caller { return exam.Caller{ID: c.ID, Role: c.Role} }

// authorizeExam checks the role, then that the caller owns the exam or is an
// administrator. A missing exam and one the caller does not own look the same.
func (s *service) authorizeExam(ctx context.Context, examID uuid.UUID, caller Caller) (*exam.Exam, error) {
	if !caller.canManage() {
		return nil, ErrForbiddenRole
	}
	ex, err := s.exams.GetExamForCaller(ctx, examID, caller.examCaller())
	switch {
	case errors.Is(err, exam.ErrExamNotFound), errors.Is(err, exam.ErrUnauthorizedCreator), errors.Is(err, exam.ErrInvalidCallerRole):
		return nil, ErrNotFound
	case err != nil:
		return nil, err
	case ex == nil || ex.IsDeleted():
		return nil, ErrNotFound
	}
	return ex, nil
}

// load authorises the caller for the exam and returns one of its documents. A
// document of another exam is not found, whatever its id.
func (s *service) load(ctx context.Context, examID, documentID uuid.UUID, caller Caller) (*Document, error) {
	if _, err := s.authorizeExam(ctx, examID, caller); err != nil {
		return nil, err
	}
	d, err := s.repo.Get(ctx, documentID)
	if err != nil {
		return nil, err
	}
	if d.ExamID != examID {
		return nil, ErrNotFound
	}
	return d, nil
}

// cleanFilename keeps the name a teacher sees. It is a label only: files are
// stored under a random key and the name is never used in a path.
func cleanFilename(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == utf8.RuneError {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > 255 {
		name = string(r[:255])
	}
	if name == "" || name == "." || name == ".." {
		return "exam.pdf"
	}
	return name
}

func (s *service) Upload(ctx context.Context, examID uuid.UUID, filename string, body io.Reader, caller Caller) (*Document, error) {
	ex, err := s.authorizeExam(ctx, examID, caller)
	if err != nil {
		return nil, err
	}
	if ex.Status != exam.StatusDraft {
		return nil, ErrExamNotDraft
	}
	if !s.opts.Active {
		return nil, ErrExtractionUnavailable
	}
	live, err := s.repo.CountLive(ctx, examID)
	if err != nil {
		return nil, err
	}
	if live >= s.opts.MaxLivePerExam {
		return nil, ErrTooManyDocuments
	}

	// Read at most one byte over the limit: enough to know it is too big, without
	// ever holding more than the limit.
	data, err := io.ReadAll(io.LimitReader(body, s.opts.MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > s.opts.MaxBytes {
		return nil, ErrFileTooLarge
	}

	info, err := s.inspect(ctx, data)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(data)
	now := time.Now().UTC()
	d := &Document{
		ID: uuid.New(), ExamID: examID, UploadedBy: caller.ID,
		OriginalFilename: cleanFilename(filename), StorageKey: audiostore.NewKeyFor(audiostore.KindPDF),
		SizeBytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), PageCount: info.PageCount, ProgressTotal: info.PageCount,
		Status: StatusQueued, Attempt: 1, Provider: s.extractor.Name(), PromptVersion: s.extractor.PromptVersion(),
		CreatedAt: now, UpdatedAt: now,
	}

	if err := s.store.Put(ctx, d.StorageKey, bytes.NewReader(data), d.SizeBytes); err != nil {
		s.log.Error().Err(err).Str("store", s.store.Name()).Msg("storing an uploaded document failed")
		return nil, ErrStorageUnavailable
	}
	// From here on, anything that fails removes the file again, so a refused
	// upload leaves nothing behind. The cleanup must outlive a cancelled request.
	cleanup := context.WithoutCancel(ctx)
	removeFile := func() {
		if derr := s.store.Delete(cleanup, d.StorageKey); derr != nil {
			s.log.Error().Err(derr).Str("document_id", d.ID.String()).Msg("could not remove the file of a refused upload")
		}
	}

	if err := s.repo.Insert(ctx, d); err != nil {
		removeFile()
		return nil, err
	}
	if err := s.queue.EnqueueExtract(d.ID, d.Attempt); err != nil {
		s.log.Error().Err(err).Str("document_id", d.ID.String()).Msg("queueing an uploaded document failed")
		if derr := s.repo.Delete(cleanup, d.ID); derr != nil {
			s.log.Error().Err(derr).Str("document_id", d.ID.String()).Msg("could not remove the record of a document that could not be queued")
		}
		removeFile()
		return nil, ErrQueueUnavailable
	}

	s.log.Info().
		Str("event", "document.uploaded").
		Str("document_id", d.ID.String()).
		Str("exam_id", examID.String()).
		Int("pages", d.PageCount).
		Int64("bytes", d.SizeBytes).
		Msg("exam document uploaded and queued")
	return d, nil
}

// inspect validates a file under the parser limit and maps what is wrong with
// it to an error a teacher can act on.
func (s *service) inspect(ctx context.Context, data []byte) (pdf.Info, error) {
	if err := pdf.CheckSignature(data, s.opts.MaxBytes); err != nil {
		return pdf.Info{}, mapPDFError(err)
	}
	gctx, cancel := context.WithTimeout(ctx, s.opts.ParseTimeout)
	defer cancel()
	release, err := s.gate.Acquire(gctx)
	if err != nil {
		return pdf.Info{}, ErrBusy
	}
	defer release()

	info, err := pdf.Inspect(ctx, data, pdf.Limits{MaxBytes: s.opts.MaxBytes, MaxPages: s.opts.MaxPages, Timeout: s.opts.ParseTimeout})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return pdf.Info{}, &InvalidPDFError{Reason: ReasonMalformed}
		}
		return pdf.Info{}, mapPDFError(err)
	}
	return info, nil
}

func mapPDFError(err error) error {
	switch {
	case errors.Is(err, pdf.ErrTooLarge):
		return ErrFileTooLarge
	case errors.Is(err, pdf.ErrNotPDF):
		return ErrUnsupportedType
	case errors.Is(err, pdf.ErrEncrypted):
		return &InvalidPDFError{Reason: ReasonEncrypted}
	case errors.Is(err, pdf.ErrTooManyPages):
		return &InvalidPDFError{Reason: ReasonTooManyPages}
	case errors.Is(err, pdf.ErrNoPages):
		return &InvalidPDFError{Reason: ReasonNoPages}
	case errors.Is(err, pdf.ErrBusy):
		return ErrBusy
	}
	return &InvalidPDFError{Reason: ReasonMalformed}
}

func (s *service) List(ctx context.Context, examID uuid.UUID, caller Caller) ([]Document, error) {
	if _, err := s.authorizeExam(ctx, examID, caller); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, examID)
}

func (s *service) Get(ctx context.Context, examID, documentID uuid.UUID, caller Caller) (*Document, error) {
	return s.load(ctx, examID, documentID, caller)
}

func (s *service) OpenFile(ctx context.Context, examID, documentID uuid.UUID, caller Caller) (io.ReadCloser, int64, *Document, error) {
	d, err := s.load(ctx, examID, documentID, caller)
	if err != nil {
		return nil, 0, nil, err
	}
	rc, size, err := s.store.Open(ctx, d.StorageKey)
	if err != nil {
		if errors.Is(err, audiostore.ErrNotFound) {
			return nil, 0, nil, ErrNotFound
		}
		return nil, 0, nil, ErrStorageUnavailable
	}
	return rc, size, d, nil
}

func (s *service) Cancel(ctx context.Context, examID, documentID uuid.UUID, caller Caller) (*Document, error) {
	d, err := s.load(ctx, examID, documentID, caller)
	if err != nil {
		return nil, err
	}
	if d.Status == StatusCancelled {
		return d, nil // cancelling twice changes nothing
	}
	ok, err := s.repo.Cancel(ctx, documentID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidState
	}
	s.log.Info().Str("event", "document.cancelled").Str("document_id", documentID.String()).Msg("exam document processing cancelled")
	return s.repo.Get(ctx, documentID)
}

func (s *service) Retry(ctx context.Context, examID, documentID uuid.UUID, req RetryRequest, caller Caller) (*Document, error) {
	if _, err := s.load(ctx, examID, documentID, caller); err != nil {
		return nil, err
	}
	if !s.opts.Active {
		return nil, ErrExtractionUnavailable
	}

	var attempt int
	err := s.tx.WithTx(ctx, func(tx database.DBTX) error {
		d, err := s.repo.GetForUpdate(ctx, documentID, tx)
		if err != nil {
			return err
		}
		switch d.Status {
		case StatusFailed, StatusCancelled:
		case StatusReviewRequired:
			if !req.DiscardDraft {
				return ErrInvalidState
			}
			if req.ExpectedDraftVersion != d.DraftVersion {
				return ErrStaleVersion
			}
		default:
			return ErrInvalidState
		}
		attempt, err = s.repo.Restart(ctx, documentID, tx)
		return err
	})
	if err != nil {
		return nil, err
	}

	if err := s.queue.EnqueueExtract(documentID, attempt); err != nil {
		s.log.Error().Err(err).Str("document_id", documentID.String()).Msg("queueing a retry failed")
		if _, ferr := s.repo.Fail(context.WithoutCancel(ctx), documentID, attempt, extract.CodeQueueDown, failureMessage(extract.CodeQueueDown)); ferr != nil {
			s.log.Error().Err(ferr).Str("document_id", documentID.String()).Msg("could not record the queue failure")
		}
		return nil, ErrQueueUnavailable
	}
	s.log.Info().Str("event", "document.retried").Str("document_id", documentID.String()).Int("attempt", attempt).Msg("exam document queued again")
	return s.repo.Get(ctx, documentID)
}

func (s *service) Delete(ctx context.Context, examID, documentID uuid.UUID, caller Caller) error {
	d, err := s.load(ctx, examID, documentID, caller)
	if err != nil {
		return err
	}
	// Questions already approved into the exam stay: their link to the document
	// is cleared by the database, not their content.
	if err := s.repo.Delete(ctx, documentID); err != nil {
		return err
	}
	if err := s.store.Delete(context.WithoutCancel(ctx), d.StorageKey); err != nil {
		s.log.Error().Err(err).Str("document_id", documentID.String()).Msg("the record was deleted but its file could not be removed")
	}
	s.log.Info().Str("event", "document.deleted").Str("document_id", documentID.String()).Msg("exam document deleted")
	return nil
}
