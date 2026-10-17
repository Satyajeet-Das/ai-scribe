package voice

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/answer"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/middleware"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/ws"
	"github.com/Satyajeet-Das/ai-scribe/internal/recording"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
	"github.com/Satyajeet-Das/ai-scribe/internal/voice/intent"
)

// SessionService is the slice of the exam-session service the voice module
// uses. Every exam transition goes through it, and therefore through the FSM.
type SessionService interface {
	GetSession(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*session.Session, int, error)
	GetCurrentQuestion(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*session.StudentQuestionResponse, error)
	NextQuestion(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*session.Session, error)
	PreviousQuestion(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*session.Session, error)
}

// AnswerService is the slice of the answer service the voice module uses.
type AnswerService interface {
	SubmitAnswer(ctx context.Context, sessionID, questionID uuid.UUID, req answer.SubmitAnswerRequest, callerID uuid.UUID) (*answer.Answer, error)
	GetAnswer(ctx context.Context, sessionID, questionID uuid.UUID, callerID uuid.UUID) (*answer.Answer, error)
}

// Recorder keeps the audio of dictated answers. It is optional: when nil, no
// audio is retained.
type Recorder interface {
	Save(ctx context.Context, in recording.SaveInput) (*recording.Saved, error)
}

// Deps are the collaborators of the voice module. All are interfaces (or
// plain data), so every provider can be replaced or mocked.
type Deps struct {
	Config         config.VoiceConfig
	AllowedOrigins []string
	Auth           platformauth.AuthProvider
	Sessions       SessionService
	Answers        AnswerService
	STT            speech.STTProvider
	TTS            speech.TTSProvider
	Classifier     intent.Classifier
	// Recorder, when set, retains the audio of answer-mode utterances.
	Recorder Recorder
	Metrics  *Metrics
	Logger   *zerolog.Logger
}

// Service owns every voice connection of this process.
type Service struct {
	Deps
	registry *ws.Registry

	baseCtx    context.Context
	baseCancel context.CancelFunc

	// recheckInterval is how often an idle connection re-reads its session.
	recheckInterval time.Duration
	// authWarnLead is how long before token expiry auth.expiring is sent.
	authWarnLead time.Duration
	now          func() time.Time

	recentMu sync.Mutex
	recent   map[uuid.UUID]time.Time

	// onConversation, when set, observes each new conversation. Tests only.
	onConversation func(*conversation)
}

func NewService(d Deps) *Service {
	if d.Metrics == nil {
		d.Metrics = NewMetrics(nil)
	}
	if d.STT == nil {
		d.STT = speech.DisabledSTT()
	}
	if d.TTS == nil {
		d.TTS = speech.DisabledTTS()
	}
	if d.Classifier == nil {
		d.Classifier = intent.Rules{}
	}
	d.AllowedOrigins = ws.SplitOrigins(d.AllowedOrigins)

	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		Deps:            d,
		registry:        ws.NewRegistry(d.Config.MaxConnections, d.Config.MaxConnectionsPerUser),
		baseCtx:         ctx,
		baseCancel:      cancel,
		recheckInterval: time.Minute,
		authWarnLead:    time.Minute,
		now:             time.Now,
		recent:          make(map[uuid.UUID]time.Time),
	}
}

// RegisterRoutes mounts the WebSocket endpoint. It is deliberately not placed
// behind the bearer-header middleware: browsers cannot set that header on a
// WebSocket, so the handler authenticates the upgrade itself.
func (s *Service) RegisterRoutes(g *echo.Group) {
	g.GET("/sessions/:id/voice", s.Connect)
}

// Shutdown stops accepting connections, closes the live ones gracefully and
// waits for their goroutines. If ctx expires first, remaining work is
// cancelled and the context error is returned.
func (s *Service) Shutdown(ctx context.Context) error {
	err := s.registry.Shutdown(ctx, ws.StatusGoingAway, "server_shutdown")
	s.baseCancel()
	if err != nil {
		// Cancelling the base context aborts whatever is still running; give
		// it a brief moment so the process does not exit mid-teardown.
		grace, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.registry.Shutdown(grace, ws.StatusGoingAway, "server_shutdown")
	}
	snapshot := s.Metrics.Snapshot()
	ev := s.Logger.Info().Str("event", "voice.shutdown")
	for _, k := range SortedKeys(snapshot) {
		ev = ev.Float64(k, snapshot[k])
	}
	ev.Msg("voice service stopped")
	return err
}

// ActiveConnections reports the number of live voice connections.
func (s *Service) ActiveConnections() int { return s.registry.Count() }

func (s *Service) ttsEnabled() bool       { return s.TTS.Name() != "disabled" }
func (s *Service) recordingEnabled() bool { return s.Recorder != nil }
func (s *Service) sttEnabled() bool       { return s.STT.Name() != "disabled" }

// Connect authenticates and authorizes the upgrade, then serves the
// connection until it ends.
func (s *Service) Connect(c echo.Context) error {
	r := c.Request()
	log := middleware.GetLogger(c)

	if !s.Config.Enabled {
		return httperrs.NewNotFoundError("Voice interaction is not enabled", false, nil)
	}

	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return httperrs.NewBadRequestError("Invalid session ID format", false, nil, nil, nil)
	}

	if !ws.CheckOrigin(r, s.AllowedOrigins) {
		s.reject("origin")
		log.Warn().Str("event", "voice.rejected").Str("reason", "origin").Msg("voice connection rejected")
		return httperrs.NewForbiddenError("Forbidden: origin not allowed", false)
	}

	token := bearerToken(r)
	if token == "" {
		s.reject("unauthenticated")
		return httperrs.NewUnauthorizedError("Unauthorized: missing access token", false)
	}
	identity, err := s.Auth.Authenticate(r.Context(), token)
	if err != nil || identity == nil || identity.UserID == uuid.Nil {
		s.reject("unauthenticated")
		log.Warn().Err(err).Str("event", "voice.rejected").Str("reason", "unauthenticated").Msg("voice connection rejected")
		return httperrs.NewUnauthorizedError("Unauthorized", false)
	}

	// Authorize against the persisted session. This runs on every connect,
	// which is what makes a reconnect safe: nothing is trusted from a previous
	// connection.
	sess, durationMins, err := s.Sessions.GetSession(r.Context(), sessionID, identity.UserID)
	if err != nil {
		s.reject("session_lookup")
		if errors.Is(err, session.ErrSessionNotFound) {
			return httperrs.NewNotFoundError("session not found", true, nil)
		}
		log.Error().Err(err).Str("event", "voice.rejected").Str("session_id", sessionID.String()).Msg("session lookup failed")
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Service temporarily unavailable")
	}
	if sess == nil {
		s.reject("session_lookup")
		return httperrs.NewNotFoundError("session not found", true, nil)
	}
	if sess.StudentID != identity.UserID {
		s.reject("forbidden")
		log.Warn().Str("event", "voice.rejected").Str("reason", "not_owner").
			Str("session_id", sessionID.String()).Str("user_id", identity.UserID.String()).
			Msg("voice connection rejected")
		return httperrs.NewForbiddenError("Forbidden: only the assigned student can access this session", false)
	}
	if sess.Status != session.StatusInProgress {
		s.reject("session_inactive")
		return httperrs.NewConflictError(inactiveMessage(sess.Status), true)
	}

	lease, err := s.registry.Acquire(identity.UserID.String())
	if err != nil {
		switch {
		case errors.Is(err, ws.ErrTooManyOwnerConnections):
			s.reject("user_limit")
			return echo.NewHTTPError(http.StatusTooManyRequests, "Too many voice connections for this user")
		case errors.Is(err, ws.ErrShuttingDown):
			s.reject("shutting_down")
		default:
			s.reject("capacity")
		}
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Voice service is at capacity, please retry")
	}
	defer lease.Release()

	// A WebSocket lives for the whole exam; keep it out of request-latency APM.
	if txn := newrelic.FromContext(r.Context()); txn != nil {
		txn.Ignore()
	}

	conn, err := ws.Accept(c.Response(), r, ws.Options{
		Subprotocols:    []string{Subprotocol},
		MaxMessageBytes: s.Config.MaxMessageBytes,
		SendQueueSize:   s.Config.SendQueueSize,
		WriteTimeout:    s.Config.WriteTimeout,
		PingInterval:    s.Config.PingInterval,
		PongTimeout:     s.Config.PongTimeout,
	})
	if err != nil {
		// Accept has already written the HTTP error.
		s.reject("upgrade")
		return nil
	}

	conv := newConversation(s, conn, identity, sess, durationMins, middleware.GetRequestID(c))

	replaced, err := lease.Bind(sessionID.String(), conn)
	if err != nil {
		conn.Close(ws.StatusGoingAway, "server_shutdown")
	}
	if replaced != nil {
		// The newest connection wins; the displaced one is told why.
		replaced.Close(CloseReplaced, "replaced")
	}
	conv.resumed = s.markSeen(sessionID) || replaced != nil
	if conv.resumed {
		s.Metrics.Inc(MetricReconnects)
	}

	if s.onConversation != nil {
		s.onConversation(conv)
	}

	// The request context dies with the handler; connection work hangs off
	// the service context so that Shutdown can cancel it.
	conv.run(s.baseCtx)
	return nil
}

func (s *Service) reject(reason string) {
	s.Metrics.Inc(MetricConnectionsRejected, reason)
}

// markSeen records that a session has had a voice connection and reports
// whether it already had one, i.e. whether this is a reconnect.
func (s *Service) markSeen(sessionID uuid.UUID) bool {
	s.recentMu.Lock()
	defer s.recentMu.Unlock()
	now := s.now()
	if len(s.recent) > 4096 {
		for id, t := range s.recent {
			if now.Sub(t) > 12*time.Hour {
				delete(s.recent, id)
			}
		}
	}
	_, seen := s.recent[sessionID]
	s.recent[sessionID] = now
	return seen
}

// bearerToken extracts the access token from the Authorization header or,
// for browsers, from the "scribe.auth.<token>" subprotocol.
func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		parts := strings.SplitN(h, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
		return ""
	}
	for _, header := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, p := range strings.Split(header, ",") {
			if p = strings.TrimSpace(p); strings.HasPrefix(p, AuthSubprotocolPrefix) {
				return strings.TrimPrefix(p, AuthSubprotocolPrefix)
			}
		}
	}
	return ""
}

func inactiveMessage(status session.Status) string {
	switch status {
	case session.StatusSubmitted:
		return "session has already been submitted"
	case session.StatusExpired:
		return "session time has expired"
	default:
		return "session is not currently in progress"
	}
}
