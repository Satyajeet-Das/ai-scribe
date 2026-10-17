package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/audit"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
)

const (
	defaultLimit    = 20
	maxLimit        = 100
	maxSearchLength = 100
)

// Caller is the authenticated identity, taken from the verified token and
// never from the request body or query.
type Caller struct {
	ID   uuid.UUID
	Role platformauth.Role
}

func (c Caller) canReview() bool {
	return c.Role == platformauth.RoleAdmin || c.Role == platformauth.RoleTeacher
}

func (c Caller) examCaller() exam.Caller {
	return exam.Caller{ID: c.ID, Role: c.Role}
}

// Policy states what a review may show. It is explicit so that neither choice
// happens by accident.
type Policy struct {
	// AllowInProgress lets an authorized teacher read the answers a student
	// has saved so far in a session that has not ended. They are read-only and
	// flagged as not final.
	AllowInProgress bool
	// ExposeCorrectAnswers includes each option's is-correct flag in this review.
	// Marking has its own endpoint and gives the teacher the correct option there
	// (internal/grading), so this stays off unless explicitly enabled.
	ExposeCorrectAnswers bool
}

// DefaultPolicy allows viewing in-progress responses and hides answer keys.
func DefaultPolicy() Policy {
	return Policy{AllowInProgress: true, ExposeCorrectAnswers: false}
}

// ExamAuthorizer is the exam module's ownership check.
type ExamAuthorizer interface {
	GetExamForCaller(ctx context.Context, id uuid.UUID, caller exam.Caller) (*exam.Exam, error)
}

// ListQuery is the unvalidated roster request.
type ListQuery struct {
	Search string
	Status string
	Sort   string
	Order  string
	Limit  int
	Offset int
}

// Normalize validates the query and fills defaults.
func (q ListQuery) Normalize(examID uuid.UUID) (ListParams, error) {
	p := ListParams{ExamID: examID, Sort: SortName, Order: OrderAsc, Limit: q.Limit, Offset: q.Offset}

	if p.Limit == 0 {
		p.Limit = defaultLimit
	}
	if p.Limit < 0 || p.Limit > maxLimit {
		return p, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidQuery, maxLimit)
	}
	if p.Offset < 0 {
		return p, fmt.Errorf("%w: offset must not be negative", ErrInvalidQuery)
	}

	p.Search = strings.TrimSpace(q.Search)
	if utf8.RuneCountInString(p.Search) > maxSearchLength {
		return p, fmt.Errorf("%w: search is too long", ErrInvalidQuery)
	}

	if s := strings.TrimSpace(q.Status); s != "" {
		st := Status(strings.ToUpper(s))
		if !st.Valid() {
			return p, fmt.Errorf("%w: status must be one of NOT_STARTED, IN_PROGRESS, SUBMITTED, EXPIRED", ErrInvalidQuery)
		}
		p.Status = &st
	}

	switch f := SortField(q.Sort); f {
	case "":
	case SortName, SortRollNo, SortSubmittedAt:
		p.Sort = f
	default:
		return p, fmt.Errorf("%w: sort must be one of name, rollNo, submittedAt", ErrInvalidQuery)
	}
	switch o := SortOrder(strings.ToLower(q.Order)); o {
	case "":
	case OrderAsc, OrderDesc:
		p.Order = o
	default:
		return p, fmt.Errorf("%w: order must be asc or desc", ErrInvalidQuery)
	}
	return p, nil
}

// ExamInfo is the exam metadata shown with a review.
type ExamInfo struct {
	ID             uuid.UUID
	Title          string
	Subject        string
	Status         string
	DurationMins   int
	TotalQuestions int
}

// StudentList is the roster result.
type StudentList struct {
	Exam   ExamInfo
	Counts StatusCounts
	Rows   []StudentRow
	Total  int
	Limit  int
	Offset int
}

// Unavailable reasons explain why no answers were returned.
const (
	ReasonNotStarted        = "NOT_STARTED"
	ReasonInProgressBlocked = "IN_PROGRESS_REVIEW_DISABLED"
)

// Counts summarises one student's answers.
type Counts struct {
	Total      int
	Answered   int
	Unanswered int
}

// StudentResponses is one student's review.
type StudentResponses struct {
	Exam              ExamInfo
	StudentID         uuid.UUID
	RollNo            string
	Name              string
	Status            Status
	Session           *SessionInfo
	ResponseAvailable bool
	UnavailableReason string
	Counts            *Counts
	Questions         []QuestionRecord
	// ShowCorrect tells the DTO layer whether option correctness may be sent.
	ShowCorrect bool
}

type Service interface {
	ListStudents(ctx context.Context, examID uuid.UUID, q ListQuery, caller Caller) (*StudentList, error)
	GetStudentResponses(ctx context.Context, examID, studentID uuid.UUID, caller Caller) (*StudentResponses, error)
}

type service struct {
	repo   Repository
	exams  ExamAuthorizer
	policy Policy
	logger *zerolog.Logger

	// store and audit are set by EnablePlayback.
	store audiostore.Store
	audit audit.Recorder
}

func NewService(repo Repository, exams ExamAuthorizer, policy Policy, logger *zerolog.Logger) Service {
	return &service{repo: repo, exams: exams, policy: policy, logger: logger}
}

// authorize checks the role and that the caller owns the exam (or is an
// administrator). Missing and not-owned exams are indistinguishable.
func (s *service) authorize(ctx context.Context, examID uuid.UUID, caller Caller) (*exam.Exam, error) {
	if !caller.canReview() || caller.ID == uuid.Nil {
		s.denied(examID, caller, "role")
		return nil, ErrForbiddenRole
	}
	ex, err := s.exams.GetExamForCaller(ctx, examID, caller.examCaller())
	if err != nil {
		if errors.Is(err, exam.ErrExamNotFound) || errors.Is(err, exam.ErrUnauthorizedCreator) {
			if errors.Is(err, exam.ErrUnauthorizedCreator) {
				s.denied(examID, caller, "ownership")
			}
			return nil, ErrExamNotFound
		}
		return nil, err
	}
	return ex, nil
}

func (s *service) denied(examID uuid.UUID, caller Caller, reason string) {
	if s.logger == nil {
		return
	}
	s.logger.Warn().
		Str("event", "review.denied").
		Str("reason", reason).
		Str("exam_id", examID.String()).
		Str("user_id", caller.ID.String()).
		Msg("response review denied")
}

func (s *service) ListStudents(ctx context.Context, examID uuid.UUID, q ListQuery, caller Caller) (*StudentList, error) {
	ex, err := s.authorize(ctx, examID, caller)
	if err != nil {
		return nil, err
	}
	params, err := q.Normalize(examID)
	if err != nil {
		return nil, err
	}
	page, err := s.repo.ListStudents(ctx, params)
	if err != nil {
		return nil, err
	}
	return &StudentList{
		Exam:   examInfo(ex, page.TotalQuestions),
		Counts: page.Counts,
		Rows:   page.Rows,
		Total:  page.Total,
		Limit:  params.Limit,
		Offset: params.Offset,
	}, nil
}

func (s *service) GetStudentResponses(ctx context.Context, examID, studentID uuid.UUID, caller Caller) (*StudentResponses, error) {
	ex, err := s.authorize(ctx, examID, caller)
	if err != nil {
		return nil, err
	}
	if studentID == uuid.Nil {
		return nil, fmt.Errorf("%w: invalid student id", ErrInvalidQuery)
	}
	data, err := s.repo.GetResponses(ctx, examID, studentID, s.policy.AllowInProgress)
	if err != nil {
		return nil, err
	}

	out := &StudentResponses{
		Exam:        examInfo(ex, data.TotalQuestions),
		StudentID:   data.StudentID,
		RollNo:      data.RollNo,
		Name:        data.Name,
		Status:      StatusNotStarted,
		Session:     data.Session,
		ShowCorrect: s.policy.ExposeCorrectAnswers,
	}
	if data.Session != nil {
		out.Status = data.Session.Status
	}

	switch {
	case data.Loaded:
		out.ResponseAvailable = true
		out.Questions = data.Questions
		c := Counts{Total: len(data.Questions)}
		for i := range data.Questions {
			if IsAnswered(data.Questions[i].Answer) {
				c.Answered++
			}
		}
		c.Unanswered = c.Total - c.Answered
		out.Counts = &c
	case data.Session == nil:
		out.UnavailableReason = ReasonNotStarted
	default:
		out.UnavailableReason = ReasonInProgressBlocked
	}
	return out, nil
}

func examInfo(ex *exam.Exam, totalQuestions int) ExamInfo {
	return ExamInfo{
		ID:             ex.ID,
		Title:          ex.Title,
		Subject:        ex.Subject,
		Status:         string(ex.Status),
		DurationMins:   ex.DurationMins,
		TotalQuestions: totalQuestions,
	}
}
