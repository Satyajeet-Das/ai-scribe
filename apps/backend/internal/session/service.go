package session

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/assignment"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/model"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
)

type AssignmentReader interface {
	GetAssignment(ctx context.Context, id uuid.UUID) (*assignment.Assignment, error)
	GetActiveAssignment(ctx context.Context, examID, studentID uuid.UUID) (*assignment.Assignment, error)
}

type ExamReader interface {
	GetExam(ctx context.Context, id uuid.UUID) (*exam.Exam, error)
}

type QuestionReader interface {
	ListQuestionsByExam(ctx context.Context, examID uuid.UUID) ([]question.Question, error)
	GetQuestion(ctx context.Context, id uuid.UUID) (*question.Question, error)
}

type TaskEnqueuer interface {
	EnqueueExpireSessionTask(sessionID uuid.UUID, delay time.Duration) error
}

// EndNotifier is told when a session stops being in progress (submitted or
// expired), so that whatever is attached to it, such as a voice connection,
// can stop at once instead of noticing on its next check.
type EndNotifier interface {
	SessionEnded(sessionID uuid.UUID)
}

type Service interface {
	GetSession(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Session, int, error)
	GetCurrentQuestion(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*StudentQuestionResponse, error)
	StartSession(ctx context.Context, req StartSessionRequest, callerID uuid.UUID) (*Session, int, error)
	SubmitSession(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Session, error)
	NextQuestion(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Session, error)
	PreviousQuestion(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Session, error)
	ExpireSession(ctx context.Context, sessionID uuid.UUID) error
	LockAndValidate(ctx context.Context, id uuid.UUID, event Event, callerID uuid.UUID) (*Session, func(context.Context) error, error)
	UpdateActivity(ctx context.Context, id uuid.UUID) error
	ListSessions(ctx context.Context, studentID uuid.UUID, limit, offset int) ([]Session, int, error)
}

type sessionService struct {
	repo             Repository
	assignmentReader AssignmentReader
	examReader       ExamReader
	questionReader   QuestionReader
	cache            Cache
	txManager        database.TxManager
	enqueuer         TaskEnqueuer
	logger           *zerolog.Logger
	ended            EndNotifier
}

// NotifyOnEnd makes svc tell n about every session it submits or expires. It
// must be called before the service is used.
func NotifyOnEnd(svc Service, n EndNotifier) {
	if s, ok := svc.(*sessionService); ok {
		s.ended = n
	}
}

func (s *sessionService) notifyEnded(id uuid.UUID) {
	if s.ended != nil {
		s.ended.SessionEnded(id)
	}
}

func NewService(repo Repository, assignmentReader AssignmentReader, examReader ExamReader, questionReader QuestionReader, cache Cache, txManager database.TxManager, enqueuer TaskEnqueuer, logger *zerolog.Logger) Service {
	return &sessionService{
		repo:             repo,
		assignmentReader: assignmentReader,
		examReader:       examReader,
		questionReader:   questionReader,
		cache:            cache,
		txManager:        txManager,
		enqueuer:         enqueuer,
		logger:           logger,
	}
}

func (s *sessionService) GetSession(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Session, int, error) {
	sess, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	if sess == nil {
		return nil, 0, ErrSessionNotFound
	}

	ex, err := s.examReader.GetExam(ctx, sess.ExamID)
	if err != nil {
		return nil, 0, err
	}
	if ex == nil {
		return nil, 0, exam.ErrExamNotFound
	}

	// Auto-expire if time has elapsed
	if sess.Status == StatusInProgress && ex.DurationMins > 0 {
		maxAllowed := time.Duration(ex.DurationMins) * time.Minute
		if time.Since(sess.StartedAt) > maxAllowed {
			if err := s.repo.Expire(ctx, id); err == nil {
				s.notifyEnded(id)
			}
			sess.Status = StatusExpired
		}
	}

	return sess, ex.DurationMins, nil
}

func (s *sessionService) GetCurrentQuestion(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*StudentQuestionResponse, error) {
	sess, _, err := s.GetSession(ctx, id, callerID)
	if err != nil {
		return nil, err
	}

	if callerID != uuid.Nil && sess.StudentID != callerID {
		return nil, ErrUnauthorizedStudent
	}

	if sess.Status != StatusInProgress {
		if sess.Status == StatusSubmitted {
			return nil, ErrSessionAlreadySubmitted
		}
		if sess.Status == StatusExpired {
			return nil, ErrSessionExpired
		}
		return nil, ErrSessionNotInProgress
	}

	questions, err := s.questionReader.ListQuestionsByExam(ctx, sess.ExamID)
	if err != nil {
		return nil, err
	}
	if len(questions) == 0 {
		return nil, ErrNoQuestionsInExam
	}

	currentQID := sess.CurrentQuestionID
	if currentQID == uuid.Nil {
		currentQID = questions[0].ID
		_ = s.repo.UpdateCurrentQuestion(ctx, sess.ID, currentQID)
		sess.CurrentQuestionID = currentQID
	}

	currentIndex := -1
	for i, q := range questions {
		if q.ID == currentQID {
			currentIndex = i
			break
		}
	}
	if currentIndex == -1 {
		currentIndex = 0
		currentQID = questions[0].ID
		_ = s.repo.UpdateCurrentQuestion(ctx, sess.ID, currentQID)
		sess.CurrentQuestionID = currentQID
	}

	qWithOpts, err := s.questionReader.GetQuestion(ctx, currentQID)
	if err != nil {
		return nil, err
	}
	if qWithOpts == nil {
		return nil, question.ErrQuestionNotFound
	}

	opts := make([]StudentOptionResponse, 0, len(qWithOpts.Options))
	for _, opt := range qWithOpts.Options {
		opts = append(opts, StudentOptionResponse{
			ID:           opt.ID,
			OptionKey:    opt.OptionKey,
			OptionText:   opt.OptionText,
			DisplayOrder: opt.DisplayOrder,
		})
	}

	return &StudentQuestionResponse{
		ID:             qWithOpts.ID,
		ExamID:         qWithOpts.ExamID,
		QuestionNumber: qWithOpts.QuestionNumber,
		Text:           qWithOpts.Text,
		Type:           qWithOpts.Type,
		Points:         qWithOpts.Points,
		Options:        opts,
		IsFirst:        currentIndex == 0,
		IsLast:         currentIndex == len(questions)-1,
		TotalQuestions: len(questions),
		CurrentIndex:   currentIndex + 1,
	}, nil
}

func (s *sessionService) StartSession(ctx context.Context, req StartSessionRequest, callerID uuid.UUID) (*Session, int, error) {
	if err := req.Validate(); err != nil {
		return nil, 0, err
	}

	var asgn *assignment.Assignment
	if req.AssignmentID != nil && *req.AssignmentID != uuid.Nil {
		var err error
		asgn, err = s.assignmentReader.GetAssignment(ctx, *req.AssignmentID)
		if err != nil {
			return nil, 0, err
		}
	} else if req.ExamID != nil && *req.ExamID != uuid.Nil {
		var err error
		asgn, err = s.assignmentReader.GetActiveAssignment(ctx, *req.ExamID, callerID)
		if err != nil {
			return nil, 0, err
		}
	}

	if asgn == nil {
		return nil, 0, ErrInvalidAssignment
	}

	if callerID != uuid.Nil && asgn.StudentID != callerID {
		return nil, 0, ErrUnauthorizedStudent
	}

	if asgn.Status == assignment.StatusRevoked {
		return nil, 0, ErrAssignmentRevoked
	}

	ex, err := s.examReader.GetExam(ctx, asgn.ExamID)
	if err != nil {
		return nil, 0, err
	}
	if ex == nil {
		return nil, 0, exam.ErrExamNotFound
	}

	if ex.Status == exam.StatusArchived {
		return nil, 0, ErrExamArchived
	}
	if ex.Status != exam.StatusPublished {
		return nil, 0, ErrExamNotPublished
	}

	existingActive, err := s.repo.GetActiveByAssignmentID(ctx, asgn.ID)
	if err != nil {
		return nil, 0, err
	}
	if existingActive != nil {
		// Auto-expire if time has elapsed
		if ex.DurationMins > 0 && time.Since(existingActive.StartedAt) > time.Duration(ex.DurationMins)*time.Minute {
			_ = s.repo.Expire(ctx, existingActive.ID)
			return nil, 0, ErrSessionExpired
		}
		s.logger.Info().
			Str("session_id", existingActive.ID.String()).
			Str("assignment_id", asgn.ID.String()).
			Str("student_id", callerID.String()).
			Msg("active session already in progress, returning existing session to rejoin")
		return existingActive, ex.DurationMins, nil
	}

	now := time.Now().UTC()
	sess := &Session{
		Base: model.Base{
			BaseWithId:        model.BaseWithId{ID: uuid.New()},
			BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
			BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
		},
		AssignmentID:   asgn.ID,
		ExamID:         asgn.ExamID,
		StudentID:      asgn.StudentID,
		Status:         StatusPending,
		StartedAt:      now,
		LastActivityAt: now,
	}

	// Determine the first question
	questions, err := s.questionReader.ListQuestionsByExam(ctx, ex.ID)
	if err != nil {
		return nil, 0, err
	}
	if len(questions) > 0 {
		sess.CurrentQuestionID = questions[0].ID
	}

	nextState, err := Transition(sess.Status, EventBegin)
	if err != nil {
		s.logger.Warn().
			Err(err).
			Str("event", "session.transition_rejected").
			Str("previous_state", string(sess.Status)).
			Str("action", string(EventBegin)).
			Str("student_id", asgn.StudentID.String()).
			Msg("session transition rejected")
		return nil, 0, err
	}
	sess.Status = nextState

	if err := s.txManager.WithTx(ctx, func(tx database.DBTX) error {
		return s.repo.Create(ctx, sess)
	}); err != nil {
		var pgErr *pgconn.PgError
		if (errors.As(err, &pgErr) && pgErr.Code == "23505") || strings.Contains(err.Error(), "idx_unique_active_session") {
			existingActive, getErr := s.repo.GetActiveByAssignmentID(ctx, asgn.ID)
			if getErr == nil && existingActive != nil {
				s.logger.Info().
					Str("session_id", existingActive.ID.String()).
					Str("assignment_id", asgn.ID.String()).
					Str("student_id", callerID.String()).
					Msg("concurrent start detected, returning existing active session")
				return existingActive, ex.DurationMins, nil
			}
		}
		s.logger.Error().Err(err).
			Str("assignment_id", asgn.ID.String()).
			Str("student_id", asgn.StudentID.String()).
			Msg("failed to create exam session")
		return nil, 0, err
	}

	// Cache state in Redis
	ttl := time.Duration(ex.DurationMins) * time.Minute
	if ttl == 0 {
		ttl = 4 * time.Hour // Default fallback
	}
	if err := s.cache.SetState(ctx, sess.ID, sess, ttl); err != nil {
		s.logger.Warn().Err(err).Str("session_id", sess.ID.String()).Msg("failed to cache session state in redis")
	}

	s.logger.Info().
		Str("event", "session.started").
		Str("session_id", sess.ID.String()).
		Str("assignment_id", asgn.ID.String()).
		Str("student_id", asgn.StudentID.String()).
		Msg("exam session started successfully")

	return sess, ex.DurationMins, nil
}

func (s *sessionService) SubmitSession(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Session, error) {
	sess, unlock, err := s.LockAndValidate(ctx, id, EventSubmit, callerID)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = unlock(context.Background())
	}()

	now := time.Now().UTC()
	
	if err := s.txManager.WithTx(ctx, func(tx database.DBTX) error {
		return s.repo.Submit(ctx, id, now)
	}); err != nil {
		s.logger.Error().Err(err).Str("session_id", id.String()).Msg("failed to submit session")
		return nil, err
	}

	sess.Status = StatusSubmitted
	sess.SubmittedAt = &now
	sess.UpdatedAt = now

	// Clear cache on submit
	_ = s.cache.DeleteState(ctx, id)
	s.notifyEnded(id)

	s.logger.Info().
		Str("event", "session.submitted").
		Str("session_id", id.String()).
		Str("student_id", sess.StudentID.String()).
		Msg("exam session submitted successfully")

	return sess, nil
}

func (s *sessionService) NextQuestion(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Session, error) {
	sess, unlock, err := s.LockAndValidate(ctx, id, EventNavigateNext, callerID)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = unlock(context.Background())
	}()

	questions, err := s.questionReader.ListQuestionsByExam(ctx, sess.ExamID)
	if err != nil {
		return nil, err
	}

	currentIndex := -1
	for i, q := range questions {
		if q.ID == sess.CurrentQuestionID {
			currentIndex = i
			break
		}
	}

	if currentIndex == -1 || currentIndex >= len(questions)-1 {
		return nil, ErrNoNextQuestion
	}

	nextQuestionID := questions[currentIndex+1].ID
	now := time.Now().UTC()

	if err := s.txManager.WithTx(ctx, func(tx database.DBTX) error {
		return s.repo.UpdateCurrentQuestion(ctx, sess.ID, nextQuestionID)
	}); err != nil {
		s.logger.Error().Err(err).Str("session_id", id.String()).Msg("failed to update current question in database")
		return nil, err
	}

	sess.CurrentQuestionID = nextQuestionID
	sess.LastActivityAt = now

	// Update cache with new runtime state
	ex, err := s.examReader.GetExam(ctx, sess.ExamID)
	if err != nil {
		return nil, err
	}
	
	ttl := time.Duration(ex.DurationMins) * time.Minute
	if ttl == 0 {
		ttl = 4 * time.Hour
	}
	_ = s.cache.SetState(ctx, sess.ID, sess, ttl)

	s.logger.Info().Str("event", "session.navigation").Str("session_id", sess.ID.String()).Str("direction", "next").Msg("navigated to next question")
	return sess, nil
}

func (s *sessionService) PreviousQuestion(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*Session, error) {
	sess, unlock, err := s.LockAndValidate(ctx, id, EventNavigatePrevious, callerID)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = unlock(context.Background())
	}()

	questions, err := s.questionReader.ListQuestionsByExam(ctx, sess.ExamID)
	if err != nil {
		return nil, err
	}

	currentIndex := -1
	for i, q := range questions {
		if q.ID == sess.CurrentQuestionID {
			currentIndex = i
			break
		}
	}

	if currentIndex <= 0 {
		return nil, ErrNoPreviousQuestion
	}

	prevQuestionID := questions[currentIndex-1].ID
	now := time.Now().UTC()

	if err := s.txManager.WithTx(ctx, func(tx database.DBTX) error {
		return s.repo.UpdateCurrentQuestion(ctx, sess.ID, prevQuestionID)
	}); err != nil {
		s.logger.Error().Err(err).Str("session_id", id.String()).Msg("failed to update current question in database")
		return nil, err
	}

	sess.CurrentQuestionID = prevQuestionID
	sess.LastActivityAt = now

	// Update cache with new runtime state
	ex, err := s.examReader.GetExam(ctx, sess.ExamID)
	if err != nil {
		return nil, err
	}
	
	ttl := time.Duration(ex.DurationMins) * time.Minute
	if ttl == 0 {
		ttl = 4 * time.Hour
	}
	_ = s.cache.SetState(ctx, sess.ID, sess, ttl)

	s.logger.Info().Str("event", "session.navigation").Str("session_id", sess.ID.String()).Str("direction", "previous").Msg("navigated to previous question")
	return sess, nil
}

func (s *sessionService) LockAndValidate(ctx context.Context, id uuid.UUID, event Event, callerID uuid.UUID) (*Session, func(context.Context) error, error) {
	// 1. Acquire distributed lock
	unlock, err := s.cache.AcquireLock(ctx, id, 10*time.Second)
	if err != nil {
		return nil, nil, err
	}

	// 2. Fetch current state (try cache first, fallback to DB)
	sess, err := s.cache.GetState(ctx, id)
	if err != nil {
		if errors.Is(err, ErrCacheMiss) {
			sess, err = s.repo.GetByID(ctx, id)
			if err != nil {
				_ = unlock(ctx)
				return nil, nil, err
			}
			if sess == nil {
				_ = unlock(ctx)
				return nil, nil, ErrSessionNotFound
			}
			// Optional: we could re-cache it here
		} else {
			_ = unlock(ctx)
			return nil, nil, err
		}
	}

	// Authorization
	if callerID != uuid.Nil && sess.StudentID != callerID {
		_ = unlock(ctx)
		return nil, nil, ErrUnauthorizedStudent
	}

	// Passive Expiration Check and Emergency Unpublish Check
	if sess.Status == StatusInProgress {
		ex, err := s.examReader.GetExam(ctx, sess.ExamID)
		if err != nil {
			_ = unlock(ctx)
			return nil, nil, err
		}
		if ex != nil {
			if ex.Status != exam.StatusPublished && event != EventSystemExpire {
				_ = unlock(ctx)
				return nil, nil, ErrExamNotPublished
			}
			if ex.DurationMins > 0 {
				maxAllowed := time.Duration(ex.DurationMins) * time.Minute
				if time.Since(sess.StartedAt) > maxAllowed {
					if err := s.txManager.WithTx(ctx, func(tx database.DBTX) error {
						return s.repo.Expire(ctx, id)
					}); err == nil {
						sess.Status = StatusExpired
						_ = s.cache.DeleteState(ctx, id)
						s.notifyEnded(id)
					}
				}
			}
		}
	}

	// 3. FSM Guard
	_, err = Transition(sess.Status, event)
	if err != nil {
		_ = unlock(ctx)
		
		logEvent := s.logger.Warn().
			Err(err).
			Str("event", "session.transition_rejected").
			Str("session_id", id.String()).
			Str("previous_state", string(sess.Status)).
			Str("action", string(event))
			
		if callerID != uuid.Nil {
			logEvent.Str("student_id", callerID.String())
		}
		
		logEvent.Msg("session transition rejected")
		
		return nil, nil, err
	}

	return sess, unlock, nil
}

func (s *sessionService) ExpireSession(ctx context.Context, id uuid.UUID) error {
	// Attempt to lock and transition via SYSTEM_EXPIRE
	sess, unlock, err := s.LockAndValidate(ctx, id, EventSystemExpire, uuid.Nil)
	if err != nil {
		// If it's already expired or submitted, that's fine. It's a no-op.
		if errors.Is(err, ErrSessionAlreadySubmitted) || errors.Is(err, ErrSessionExpired) {
			s.logger.Debug().Str("session_id", id.String()).Msg("session already completed, ignoring expiry task")
			return nil
		}
		return err
	}
	defer func() {
		_ = unlock(context.Background())
	}()

	// Perform expiration
	if err := s.txManager.WithTx(ctx, func(tx database.DBTX) error {
		return s.repo.Expire(ctx, id)
	}); err != nil {
		s.logger.Error().Err(err).Str("session_id", id.String()).Msg("failed to expire session in database")
		return err
	}

	// Update in-memory struct and clear cache
	sess.Status = StatusExpired
	_ = s.cache.DeleteState(ctx, id)
	s.notifyEnded(id)

	s.logger.Info().
		Str("event", "session.expired").
		Str("session_id", id.String()).
		Msg("exam session forcefully expired")

	return nil
}

func (s *sessionService) UpdateActivity(ctx context.Context, id uuid.UUID) error {
	unlock, err := s.cache.AcquireLock(ctx, id, 10*time.Second)
	if err != nil {
		return err
	}
	defer func() {
		_ = unlock(context.Background())
	}()

	sess, err := s.cache.GetState(ctx, id)
	if err != nil {
		if errors.Is(err, ErrCacheMiss) {
			return nil // Nothing to update in cache
		}
		return err
	}

	sess.LastActivityAt = time.Now().UTC()

	ex, err := s.examReader.GetExam(ctx, sess.ExamID)
	if err != nil {
		return err
	}
	
	ttl := time.Duration(ex.DurationMins) * time.Minute
	if ttl == 0 {
		ttl = 4 * time.Hour
	}
	
	return s.cache.SetState(ctx, sess.ID, sess, ttl)
}

func (s *sessionService) ListSessions(ctx context.Context, studentID uuid.UUID, limit, offset int) ([]Session, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListByStudentID(ctx, studentID, limit, offset)
}
