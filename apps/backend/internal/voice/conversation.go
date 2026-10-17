package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/ws"
	"github.com/Satyajeet-Das/ai-scribe/internal/recording"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
	"github.com/Satyajeet-Das/ai-scribe/internal/voice/intent"
)

const (
	// maxConsecutiveViolations is how many malformed messages in a row a
	// client may send before it is disconnected.
	maxConsecutiveViolations = 10
	// recentRequestIDs is how many request IDs are remembered for de-duplication.
	recentRequestIDs = 256
)

var (
	errBargeIn         = errors.New("barge-in")
	errClientCancelled = errors.New("cancelled by client")
	errSuperseded      = errors.New("superseded by a newer request")
	errTurnFailed      = errors.New("turn failed")
	errConnectionEnded = errors.New("connection ended")
)

// conversation is the per-connection controller. Inbound frames arrive on the
// connection's single read goroutine; each utterance or command then runs as
// a turn on its own goroutine so that a slow provider never blocks the socket.
type conversation struct {
	svc  *Service
	conn *ws.Conn
	log  zerolog.Logger

	id        string
	userID    uuid.UUID
	sessionID uuid.UUID
	resumed   bool
	startedAt time.Time

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Read-goroutine state: touched only from HandleText / HandleBinary.
	limiter       *rate.Limiter
	seen          map[string]struct{}
	seenOrder     []string
	violations    int
	droppedStream uuid.UUID

	// speechEpoch advances every time the student interrupts. Audio frames
	// remember the epoch they were produced in and are discarded, even if
	// already queued, once it has moved on.
	speechEpoch atomic.Uint64

	mu   sync.Mutex
	turn *turn
	// lastResponseID is the most recent spoken response.
	lastResponseID string
	// sessionDeadline is when the exam's time runs out; zero if untimed.
	sessionDeadline time.Time
	authExpiresAt   time.Time
	authWarned      bool
	wake            chan struct{}

	// dialogMu serializes interpretation and execution of intents, so two
	// turns can never act on the exam concurrently from one connection.
	dialogMu  sync.Mutex
	pending   *pendingConfirmation
	completed map[string]ActionResultPayload

	endOnce sync.Once
}

// turn is one unit of interaction: a spoken utterance or a typed command.
type turn struct {
	requestID string
	streamID  uuid.UUID
	isAudio   bool
	// mode is AudioModeCommand or AudioModeAnswer for audio turns.
	mode string
	// record is true when the audio of this turn is kept for retention. pcm and
	// sampleRate belong to the read goroutine until audio.end, and to the turn
	// goroutine after it.
	record     bool
	pcmMu      sync.Mutex
	pcm        []byte
	sampleRate int
	// saved is the stored recording of this turn, once there is one.
	saved *recording.Saved
	// correction is the student's corrected wording sent with a confirm. It is
	// set before the turn starts and only read by it.
	correction *string
	startedAt  time.Time

	ctx    context.Context
	cancel context.CancelCauseFunc

	// Audio capture state. capturing and nextSeq belong to the read goroutine.
	chunks       chan []byte
	capturing    bool
	nextSeq      uint32
	bytes        atomic.Int64
	captureTimer *time.Timer
	endedAt      time.Time

	failOnce  sync.Once
	responded atomic.Bool
	// silent suppresses the turn_cancelled notice when the client itself
	// asked for the cancellation and has already been acknowledged.
	silent  atomic.Bool
	outcome atomic.Value // string

	// speechCancel and responseID are guarded by conversation.mu.
	speechCancel context.CancelCauseFunc
	responseID   string
}

func (t *turn) streamLabel() string {
	if !t.isAudio {
		return ""
	}
	return t.streamID.String()
}

// addPCM keeps a copy of dictated audio. The caller has already checked the
// stream against MaxStreamBytes, so the buffer cannot grow past that.
func (t *turn) addPCM(b []byte) {
	t.pcmMu.Lock()
	t.pcm = append(t.pcm, b...)
	t.pcmMu.Unlock()
}

// takePCM hands over the dictated audio and forgets it, so the memory is
// released whichever way the turn ends.
func (t *turn) takePCM() []byte {
	t.pcmMu.Lock()
	defer t.pcmMu.Unlock()
	pcm := t.pcm
	t.pcm = nil
	return pcm
}

func (t *turn) setOutcome(o string) { t.outcome.CompareAndSwap(nil, o) }

func newConversation(s *Service, conn *ws.Conn, identity *platformauth.AuthenticatedIdentity, sess *session.Session, durationMins int, httpRequestID string) *conversation {
	id := uuid.NewString()
	c := &conversation{
		svc:       s,
		conn:      conn,
		id:        id,
		userID:    identity.UserID,
		sessionID: sess.ID,
		startedAt: s.now(),
		limiter:   rate.NewLimiter(rate.Limit(20), 40),
		seen:      make(map[string]struct{}, recentRequestIDs),
		wake:      make(chan struct{}, 1),
		completed: make(map[string]ActionResultPayload),
		log: s.Logger.With().
			Str("connection_id", id).
			Str("http_request_id", httpRequestID).
			Str("session_id", sess.ID.String()).
			Str("user_id", identity.UserID.String()).
			Logger(),
	}
	c.authExpiresAt = identity.ExpiresAt
	if durationMins > 0 {
		c.sessionDeadline = sess.StartedAt.Add(time.Duration(durationMins) * time.Minute)
	}
	return c
}

// run serves the connection and returns once every goroutine it started has
// exited.
func (c *conversation) run(parent context.Context) {
	c.ctx, c.cancel = context.WithCancel(parent)
	defer c.cancel()

	m := c.svc.Metrics
	m.Inc(MetricConnectionsOpened)
	m.Add(MetricConnectionsActive, 1)
	c.log.Info().Str("event", "voice.connected").Bool("resumed", c.resumed).Msg("voice connection opened")

	c.sendReady()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.watch()
	}()

	info := c.conn.Serve(c.ctx, c)

	// The socket is gone: stop whatever the student was doing. An answer save
	// that has already started is allowed to finish (see actionContext).
	c.cancel()
	c.mu.Lock()
	if c.turn != nil {
		c.turn.silent.Store(true)
		c.turn.cancel(errConnectionEnded)
	}
	c.mu.Unlock()
	c.wg.Wait()

	m.Add(MetricConnectionsActive, -1)
	m.Inc(MetricConnectionsClosed, info.Reason)
	m.Observe(MetricConnectionDuration, c.svc.now().Sub(c.startedAt))
	c.log.Info().
		Str("event", "voice.disconnected").
		Str("reason", info.Reason).
		Int("close_code", int(info.Code)).
		Bool("local", info.Local).
		Dur("duration", c.svc.now().Sub(c.startedAt)).
		Msg("voice connection closed")
}

// ---------------------------------------------------------------------------
// Outbound
// ---------------------------------------------------------------------------

func (c *conversation) encode(typ, replyTo string, payload any) []byte {
	env := Envelope{V: ProtocolVersion, Type: typ, ReplyTo: replyTo, TS: c.svc.now().UnixMilli()}
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			c.log.Error().Err(err).Str("type", typ).Msg("failed to encode voice payload")
			return nil
		}
		env.Payload = raw
	}
	data, err := json.Marshal(env)
	if err != nil {
		return nil
	}
	return data
}

// send queues a control message. Control messages are never dropped: a client
// that cannot keep up with them is disconnected instead.
func (c *conversation) send(typ, replyTo string, payload any) {
	data := c.encode(typ, replyTo, payload)
	if data == nil {
		return
	}
	if err := c.conn.Send(ws.Message{Data: data}); errors.Is(err, ws.ErrSendQueueFull) {
		c.svc.Metrics.Inc(MetricFailures, "slow_consumer")
		c.log.Warn().Str("event", "voice.slow_consumer").Msg("send queue full, closing connection")
		c.conn.Close(CloseSlowConsumer, "slow_consumer")
	}
}

// sendLossy queues an advisory message and drops it if the queue is full.
func (c *conversation) sendLossy(typ, replyTo string, payload any) {
	if data := c.encode(typ, replyTo, payload); data != nil {
		_ = c.conn.Send(ws.Message{Data: data})
	}
}

func (c *conversation) sendError(replyTo, code, message string, retryable bool, streamID string) {
	c.send(TypeError, replyTo, ErrorPayload{Code: code, Message: message, Retryable: retryable, StreamID: streamID})
}

func (c *conversation) ack(replyTo string) { c.send(TypeAck, replyTo, nil) }

// endSession tells the client why the connection is ending, then closes it.
func (c *conversation) endSession(reason string, code ws.StatusCode) {
	c.endOnce.Do(func() {
		c.log.Info().Str("event", "voice.session_closed").Str("reason", reason).Msg("ending voice connection")
		c.send(TypeSessionClosed, "", SessionClosedPayload{Reason: reason})
		c.conn.Close(code, reason)
	})
}

func (c *conversation) sendReady() {
	cfg := c.svc.Config
	caps := Capabilities{STT: c.svc.sttEnabled(), TTS: c.svc.ttsEnabled(), Recording: c.svc.recordingEnabled(), Intent: c.svc.Classifier.Name()}
	if caps.TTS {
		f := c.svc.TTS.Format()
		caps.TTSOut = &f
	}

	ctx, cancel := context.WithTimeout(c.ctx, cfg.ActionTimeout)
	defer cancel()
	state, err := c.sessionState(ctx)
	if err != nil {
		// The session was valid a moment ago; report what is known and let the
		// client re-sync.
		c.log.Warn().Err(err).Msg("could not load session state for ready message")
		state = SessionState{Status: session.StatusInProgress}
	}

	c.send(TypeReady, "", ReadyPayload{
		ConnectionID: c.id,
		Protocol:     ProtocolVersion,
		SessionID:    c.sessionID,
		Resumed:      c.resumed,
		Limits: Limits{
			MaxMessageBytes:   cfg.MaxMessageBytes,
			MaxChunkBytes:     cfg.Audio.MaxChunkBytes,
			MaxStreamBytes:    cfg.Audio.MaxStreamBytes,
			MaxStreamMillis:   cfg.Audio.MaxStreamDuration.Milliseconds(),
			AudioEncodings:    []string{speech.EncodingPCM16LE},
			AudioSampleRates:  allowedSampleRates,
			AudioChannels:     1,
			ConfirmTTLMillis:  cfg.ConfirmationTTL.Milliseconds(),
			BinaryHeaderBytes: FrameHeaderSize,
		},
		Capabilities: caps,
		Session:      state,
	})
}

// sessionState reads the authoritative exam state from the session service.
func (c *conversation) sessionState(ctx context.Context) (SessionState, error) {
	sess, durationMins, err := c.svc.Sessions.GetSession(ctx, c.sessionID, c.userID)
	if err != nil {
		return SessionState{}, err
	}
	if sess == nil {
		return SessionState{}, session.ErrSessionNotFound
	}
	if sess.StudentID != c.userID {
		return SessionState{}, session.ErrUnauthorizedStudent
	}
	resp := session.ToSessionResponse(sess, durationMins)
	state := SessionState{Status: sess.Status, RemainingSeconds: resp.RemainingSeconds}
	if sess.Status != session.StatusInProgress {
		return state, nil
	}
	q, err := c.svc.Sessions.GetCurrentQuestion(ctx, c.sessionID, c.userID)
	if err != nil {
		return state, err
	}
	state.Question = q
	return state, nil
}

// ---------------------------------------------------------------------------
// Watchdog: exam deadline, periodic re-validation, token expiry
// ---------------------------------------------------------------------------

func (c *conversation) watch() {
	nextCheck := c.svc.now().Add(c.svc.recheckInterval)
	for {
		now := c.svc.now()
		c.mu.Lock()
		deadline, authExp, warned := c.sessionDeadline, c.authExpiresAt, c.authWarned
		c.mu.Unlock()

		wakeAt := nextCheck
		if !deadline.IsZero() && deadline.After(now) && deadline.Before(wakeAt) {
			wakeAt = deadline
		}
		if !authExp.IsZero() {
			authClose := authExp.Add(c.svc.Config.AuthExpiryGrace)
			if authClose.Before(wakeAt) {
				wakeAt = authClose
			}
			if warnAt := authExp.Add(-c.svc.authWarnLead); !warned && warnAt.Before(wakeAt) {
				wakeAt = warnAt
			}
		}

		timer := time.NewTimer(wakeAt.Sub(now))
		select {
		case <-c.ctx.Done():
			timer.Stop()
			return
		case <-c.wake:
			timer.Stop()
			continue
		case <-timer.C:
		}

		now = c.svc.now()
		if !authExp.IsZero() {
			if !now.Before(authExp.Add(c.svc.Config.AuthExpiryGrace)) {
				c.svc.Metrics.Inc(MetricFailures, "auth_expired")
				c.endSession("auth_expired", CloseUnauthorized)
				return
			}
			if !warned && !now.Before(authExp.Add(-c.svc.authWarnLead)) {
				c.mu.Lock()
				c.authWarned = true
				c.mu.Unlock()
				c.send(TypeAuthExpiring, "", AuthExpiringPayload{ExpiresAt: authExp})
			}
		}

		pastDeadline := !deadline.IsZero() && !now.Before(deadline)
		if pastDeadline || !now.Before(nextCheck) {
			nextCheck = now.Add(c.svc.recheckInterval)
			if pastDeadline && nextCheck.Sub(now) > time.Second {
				// Clocks differ slightly; poll until the session service
				// agrees that time is up.
				nextCheck = now.Add(time.Second)
			}
			if !c.revalidate() {
				return
			}
		}
	}
}

// revalidate re-reads the persisted session and ends the connection if the
// session is no longer the caller's or no longer in progress. It reports
// whether the connection may continue.
func (c *conversation) revalidate() bool {
	ctx, cancel := context.WithTimeout(c.ctx, c.svc.Config.ActionTimeout)
	defer cancel()

	sess, _, err := c.svc.Sessions.GetSession(ctx, c.sessionID, c.userID)
	if err != nil {
		f := classifyError(err)
		if f.terminal {
			c.endSession(f.closeReason, f.closeCode)
			return false
		}
		// A transient outage must not end the exam connection.
		c.log.Warn().Err(err).Str("event", "voice.revalidate_failed").Msg("session revalidation failed")
		return true
	}
	if sess == nil || sess.StudentID != c.userID {
		c.endSession("forbidden", CloseForbidden)
		return false
	}
	switch sess.Status {
	case session.StatusInProgress:
		return true
	case session.StatusSubmitted:
		c.endSession("submitted", CloseSessionEnded)
	case session.StatusExpired:
		c.endSession("expired", CloseSessionEnded)
	default:
		c.endSession("inactive", CloseSessionEnded)
	}
	return false
}

// ---------------------------------------------------------------------------
// Inbound: text
// ---------------------------------------------------------------------------

func (c *conversation) violation(replyTo, code, message string) {
	c.svc.Metrics.Inc(MetricProtocolErrors, code)
	c.sendError(replyTo, code, message, false, "")
	c.violations++
	if c.violations >= maxConsecutiveViolations {
		c.log.Warn().Str("event", "voice.protocol_violations").Msg("too many malformed messages, closing connection")
		c.conn.Close(ws.StatusPolicyViolation, "protocol_violations")
	}
}

func (c *conversation) rememberID(id string) bool {
	if _, dup := c.seen[id]; dup {
		return false
	}
	if len(c.seenOrder) >= recentRequestIDs {
		delete(c.seen, c.seenOrder[0])
		c.seenOrder = c.seenOrder[1:]
	}
	c.seen[id] = struct{}{}
	c.seenOrder = append(c.seenOrder, id)
	return true
}

func strictDecode(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("trailing data")
	}
	return nil
}

// payload decodes a required payload, reporting a violation if it is invalid.
func (c *conversation) payload(env *Envelope, v any) bool {
	if len(env.Payload) == 0 || strictDecode(env.Payload, v) != nil {
		c.violation(env.ID, CodeInvalidMessage, "invalid or missing payload for "+env.Type)
		return false
	}
	return true
}

// HandleText implements ws.Handler.
func (c *conversation) HandleText(_ context.Context, data []byte) {
	if !c.limiter.Allow() {
		c.violation("", CodeRateLimited, "too many messages")
		return
	}

	var env Envelope
	if err := strictDecode(data, &env); err != nil {
		c.violation("", CodeInvalidMessage, "message is not a valid envelope")
		return
	}
	if env.V != ProtocolVersion {
		c.violation(env.ID, CodeUnsupportedVersion, "unsupported protocol version")
		return
	}
	if !validID(env.ID) {
		c.violation("", CodeInvalidMessage, "message id is missing or invalid")
		return
	}
	if !c.rememberID(env.ID) {
		// A retransmitted or replayed request is reported, never re-executed.
		c.svc.Metrics.Inc(MetricProtocolErrors, CodeDuplicateRequest)
		c.sendError(env.ID, CodeDuplicateRequest, "request id was already used", false, "")
		return
	}

	switch env.Type {
	case TypePing:
		c.send(TypePong, env.ID, nil)
	case TypeSessionSync:
		c.handleSessionSync(&env)
	case TypeAuthRefresh:
		c.handleAuthRefresh(&env)
	case TypeAudioStart:
		c.handleAudioStart(&env)
	case TypeAudioEnd:
		c.handleAudioEnd(&env)
	case TypeAudioCancel:
		c.handleAudioCancel(&env)
	case TypeTTSCancel:
		c.handleTTSCancel(&env)
	case TypeCommand:
		c.handleCommand(&env)
	default:
		c.violation(env.ID, CodeUnknownType, "unknown message type")
		return
	}
	c.violations = 0
}

func (c *conversation) handleSessionSync(env *Envelope) {
	ctx, cancel := context.WithTimeout(c.ctx, c.svc.Config.ActionTimeout)
	defer cancel()
	state, err := c.sessionState(ctx)
	if err != nil {
		c.domainError(env.ID, err, "")
		if state.Status == "" {
			return
		}
	}
	c.send(TypeSessionState, env.ID, state)
	if state.Status != session.StatusInProgress {
		c.endSession(closeReasonFor(state.Status), CloseSessionEnded)
	}
}

func (c *conversation) handleAuthRefresh(env *Envelope) {
	var p AuthRefreshPayload
	if !c.payload(env, &p) {
		return
	}
	ctx, cancel := context.WithTimeout(c.ctx, c.svc.Config.ActionTimeout)
	defer cancel()

	identity, err := c.svc.Auth.Authenticate(ctx, p.Token)
	if err != nil || identity == nil {
		c.sendError(env.ID, CodeUnauthorized, "token refresh rejected", false, "")
		return
	}
	if identity.UserID != c.userID {
		// A token for a different user must never be adopted by a live session.
		c.log.Warn().Str("event", "voice.auth_mismatch").Msg("auth.refresh presented a token for a different user")
		c.sendError(env.ID, CodeForbidden, "token does not belong to this connection's user", false, "")
		c.endSession("forbidden", CloseForbidden)
		return
	}

	c.mu.Lock()
	c.authExpiresAt = identity.ExpiresAt
	c.authWarned = false
	c.mu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
	c.ack(env.ID)
}

// currentTurn returns the live turn, treating a cancelled one as absent.
func (c *conversation) currentTurn() *turn {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.turn != nil && c.turn.ctx.Err() != nil {
		return nil
	}
	return c.turn
}

func (c *conversation) startTurn(requestID string, isAudio bool, streamID uuid.UUID) *turn {
	ctx, cancel := context.WithCancelCause(c.ctx)
	t := &turn{
		requestID: requestID,
		streamID:  streamID,
		isAudio:   isAudio,
		startedAt: c.svc.now(),
		ctx:       ctx,
		cancel:    cancel,
	}
	c.mu.Lock()
	c.turn = t
	c.mu.Unlock()
	return t
}

// interrupt cancels the turn in progress, if any, because the student has
// started something new. This is barge-in: active speech stops immediately and
// in-flight recognition or classification is abandoned as stale.
func (c *conversation) interrupt(cause error) {
	prev := c.currentTurn()
	if prev == nil {
		return
	}
	if prev.capturing {
		c.droppedStream = prev.streamID
	}
	label := "superseded"
	if errors.Is(cause, errBargeIn) {
		label = "barge_in"
	}
	c.svc.Metrics.Inc(MetricCancellations, label)
	prev.cancel(cause)
}

func (c *conversation) handleAudioStart(env *Envelope) {
	var p AudioStartPayload
	if !c.payload(env, &p) {
		return
	}
	streamID, err := uuid.Parse(p.StreamID)
	if err != nil || streamID == uuid.Nil {
		c.violation(env.ID, CodeInvalidMessage, "streamId must be a UUID")
		return
	}
	if p.Encoding != speech.EncodingPCM16LE || p.Channels != 1 || !validSampleRate(p.SampleRate) {
		c.sendError(env.ID, CodeInvalidAudioFormat, "audio must be mono pcm_s16le at a supported sample rate", false, p.StreamID)
		return
	}
	if !validAudioMode(p.Mode) {
		c.sendError(env.ID, CodeInvalidAudioMode, "mode must be \"command\" or \"answer\"", false, p.StreamID)
		return
	}
	if t := c.currentTurn(); t != nil && t.capturing {
		c.sendError(env.ID, CodeStreamActive, "another audio stream is still open; end or cancel it first", false, p.StreamID)
		return
	}
	if !c.svc.sttEnabled() {
		c.sendError(env.ID, CodeSTTUnavailable, "speech recognition is not configured", false, p.StreamID)
		return
	}

	// Every stream is tied to a session that is re-read from storage here, so
	// audio is never accepted for a session that has ended.
	if !c.revalidateFor(env.ID, p.StreamID) {
		return
	}

	c.speechEpoch.Add(1)
	c.interrupt(errBargeIn)

	t := c.startTurn(env.ID, true, streamID)
	t.mode = p.Mode
	if t.mode == "" {
		t.mode = AudioModeCommand
	}
	// Only dictated answers are kept. Commands are never retained.
	t.record = t.mode == AudioModeAnswer && c.svc.recordingEnabled()
	t.sampleRate = p.SampleRate
	t.capturing = true
	t.chunks = make(chan []byte, c.svc.Config.Audio.BufferChunks)
	t.captureTimer = time.AfterFunc(c.svc.Config.Audio.MaxStreamDuration, func() {
		c.failTurn(t, CodeStreamTooLong, "audio stream exceeded the maximum duration", false)
	})

	c.svc.Metrics.Inc(MetricStreamsStarted)
	c.log.Debug().Str("event", "voice.stream_started").Str("stream_id", p.StreamID).Str("request_id", env.ID).Msg("audio stream started")

	format := speech.AudioFormat{Encoding: p.Encoding, SampleRate: p.SampleRate, Channels: p.Channels}
	c.wg.Add(1)
	go c.runAudioTurn(t, format)

	c.ack(env.ID)
}

// revalidateFor re-reads the session before accepting new work. On failure it
// has already answered the request.
func (c *conversation) revalidateFor(requestID, streamID string) bool {
	ctx, cancel := context.WithTimeout(c.ctx, c.svc.Config.ActionTimeout)
	defer cancel()
	sess, _, err := c.svc.Sessions.GetSession(ctx, c.sessionID, c.userID)
	if err == nil {
		switch {
		case sess == nil:
			err = session.ErrSessionNotFound
		case sess.StudentID != c.userID:
			err = session.ErrUnauthorizedStudent
		case sess.Status == session.StatusSubmitted:
			err = session.ErrSessionAlreadySubmitted
		case sess.Status == session.StatusExpired:
			err = session.ErrSessionExpired
		case sess.Status != session.StatusInProgress:
			err = session.ErrSessionNotInProgress
		}
	}
	if err != nil {
		c.domainError(requestID, err, streamID)
		return false
	}
	return true
}

// domainError reports a failure from the exam services and, when the failure
// means the session is over, ends the connection.
func (c *conversation) domainError(replyTo string, err error, streamID string) failure {
	f := classifyError(err)
	c.svc.Metrics.Inc(MetricFailures, f.code)
	if f.unexpected {
		c.log.Error().Err(err).Str("event", "voice.dependency_failure").Str("code", f.code).Msg("exam service call failed")
	}
	c.sendError(replyTo, f.code, f.message, f.retryable, streamID)
	if f.terminal {
		c.endSession(f.closeReason, f.closeCode)
	}
	return f
}

func (c *conversation) handleAudioEnd(env *Envelope) {
	var p AudioEndPayload
	if !c.payload(env, &p) {
		return
	}
	t := c.currentTurn()
	if t == nil || !t.capturing || t.streamID.String() != p.StreamID {
		c.sendError(env.ID, CodeUnknownStream, "no open audio stream with that id", false, p.StreamID)
		return
	}
	if p.Chunks != nil && *p.Chunks != t.nextSeq {
		c.droppedStream = t.streamID
		c.failTurn(t, CodeChunkCountMismatch, "received chunk count does not match the client's count", true)
		return
	}
	if t.nextSeq == 0 {
		c.droppedStream = t.streamID
		c.failTurn(t, CodeEmptyStream, "audio stream contained no audio", false)
		return
	}

	t.capturing = false
	t.captureTimer.Stop()
	t.endedAt = c.svc.now()
	close(t.chunks)
	c.ack(env.ID)
}

func (c *conversation) handleAudioCancel(env *Envelope) {
	var p AudioCancelPayload
	if !c.payload(env, &p) {
		return
	}
	t := c.currentTurn()
	if t == nil || !t.isAudio || t.streamID.String() != p.StreamID {
		c.sendError(env.ID, CodeUnknownStream, "no audio stream with that id", false, p.StreamID)
		return
	}
	if t.capturing {
		c.droppedStream = t.streamID
	}
	t.silent.Store(true)
	t.setOutcome("cancelled")
	t.cancel(errClientCancelled)
	c.svc.Metrics.Inc(MetricCancellations, "client")
	c.ack(env.ID)
}

func (c *conversation) handleTTSCancel(env *Envelope) {
	var p TTSCancelPayload
	if len(env.Payload) > 0 && strictDecode(env.Payload, &p) != nil {
		c.violation(env.ID, CodeInvalidMessage, "invalid payload for "+env.Type)
		return
	}
	c.mu.Lock()
	if p.ResponseID == "" || p.ResponseID == c.lastResponseID {
		// Silences audio that is already queued, whether or not synthesis
		// is still running.
		c.speechEpoch.Add(1)
	}
	if t := c.turn; t != nil && t.speechCancel != nil && (p.ResponseID == "" || p.ResponseID == t.responseID) {
		t.speechCancel(errClientCancelled)
		c.svc.Metrics.Inc(MetricCancellations, "tts_client")
	}
	c.mu.Unlock()
	// Cancelling speech that already finished is not an error.
	c.ack(env.ID)
}

func (c *conversation) handleCommand(env *Envelope) {
	var p CommandPayload
	if !c.payload(env, &p) {
		return
	}
	name := intent.Name(p.Intent)
	// Free-text answers cannot be typed through the command channel, and
	// "unknown" is an outcome, not a request.
	if !name.Valid() || name == intent.Unknown || name == intent.DictateAnswer {
		c.sendError(env.ID, CodeUnsupportedIntent, "intent is not a permitted command", false, "")
		return
	}
	if p.ConfirmationID != "" && !validID(p.ConfirmationID) {
		c.violation(env.ID, CodeInvalidMessage, "confirmationId is invalid")
		return
	}
	if p.Text != nil {
		// A correction rides on a confirmation of a specific staged answer, and
		// is bounded like any other free text.
		switch {
		case name != intent.Confirm || p.ConfirmationID == "":
			c.sendError(env.ID, CodeUnsupportedIntent, "text is only valid when confirming a staged answer", false, "")
			return
		case !utf8.ValidString(*p.Text) || strings.ContainsRune(*p.Text, 0):
			// PostgreSQL cannot store NUL characters or invalid UTF-8; refuse it
			// here, with a reason, rather than fail later as a "save error".
			c.sendError(env.ID, CodeInvalidMessage, "the corrected answer contains characters that cannot be saved", false, "")
			return
		case strings.TrimSpace(*p.Text) == "":
			c.sendError(env.ID, CodeInvalidMessage, "an answer cannot be empty", false, "")
			return
		case utf8.RuneCountInString(*p.Text) > c.svc.Config.Intent.MaxTranscriptChars:
			c.sendError(env.ID, CodeInvalidMessage, "the corrected answer is too long", false, "")
			return
		}
	}
	// The option key is checked against the current question inside the turn.
	it := intent.Intent{Name: name, OptionKey: p.OptionKey, Confidence: 1}

	c.speechEpoch.Add(1)
	c.interrupt(errSuperseded)
	t := c.startTurn(env.ID, false, uuid.Nil)
	t.correction = p.Text
	c.wg.Add(1)
	go c.runCommandTurn(t, it, p.ConfirmationID)
	c.ack(env.ID)
}

// ---------------------------------------------------------------------------
// Inbound: binary audio
// ---------------------------------------------------------------------------

// HandleBinary implements ws.Handler.
func (c *conversation) HandleBinary(_ context.Context, data []byte) {
	f, err := decodeFrame(data)
	if err != nil {
		c.violation("", CodeInvalidFrame, "binary frame header is invalid")
		return
	}

	t := c.currentTurn()
	if t == nil || !t.capturing || t.streamID != f.ID {
		if f.ID == c.droppedStream {
			// Late chunks of a stream that already failed or was cancelled.
			c.svc.Metrics.Inc(MetricProtocolErrors, "late_chunk")
			return
		}
		c.droppedStream = f.ID
		c.svc.Metrics.Inc(MetricProtocolErrors, CodeUnknownStream)
		c.sendError("", CodeUnknownStream, "audio chunk for a stream that is not open", false, f.ID.String())
		return
	}

	cfg := c.svc.Config.Audio
	reject := func(code, message string) {
		c.droppedStream = t.streamID
		c.failTurn(t, code, message, false)
	}
	switch {
	case len(f.Audio) == 0 || len(f.Audio)%2 != 0:
		reject(CodeInvalidFrame, "audio chunk must be a non-empty whole number of 16-bit samples")
		return
	case len(f.Audio) > cfg.MaxChunkBytes:
		reject(CodeChunkTooLarge, "audio chunk exceeds the maximum size")
		return
	case f.Seq != t.nextSeq:
		reject(CodeChunkOutOfOrder, "audio chunk is out of sequence")
		return
	case t.bytes.Load()+int64(len(f.Audio)) > cfg.MaxStreamBytes:
		reject(CodeStreamTooLarge, "audio stream exceeds the maximum size")
		return
	}
	t.nextSeq++
	t.bytes.Add(int64(len(f.Audio)))
	if t.record {
		t.addPCM(f.Audio)
	}

	// The buffer between the socket and the recognizer is bounded. When it is
	// full the read goroutine waits here, which stops reading the socket and
	// pushes back on the sender; if the recognizer stays stuck, the stream is
	// failed rather than buffered without limit.
	select {
	case t.chunks <- f.Audio:
		return
	default:
	}
	timer := time.NewTimer(cfg.EnqueueTimeout)
	defer timer.Stop()
	select {
	case t.chunks <- f.Audio:
	case <-t.ctx.Done():
	case <-timer.C:
		reject(CodeBackpressure, "audio is arriving faster than it can be processed")
	}
}

// ---------------------------------------------------------------------------
// Turn bookkeeping
// ---------------------------------------------------------------------------

// failTurn ends a turn with an error message to the client. Only the first
// failure of a turn is reported.
func (c *conversation) failTurn(t *turn, code, message string, retryable bool) {
	t.failOnce.Do(func() {
		if t.ctx.Err() != nil {
			return
		}
		t.responded.Store(true)
		t.setOutcome(code)
		c.svc.Metrics.Inc(MetricFailures, code)
		c.log.Warn().Str("event", "voice.turn_failed").Str("code", code).
			Str("request_id", t.requestID).Str("stream_id", t.streamLabel()).Msg("voice turn failed")
		c.sendError(t.requestID, code, message, retryable, t.streamLabel())
		t.cancel(errTurnFailed)
	})
}

// finishTurn runs when a turn's goroutine exits.
func (c *conversation) finishTurn(t *turn) {
	defer c.wg.Done()

	if t.captureTimer != nil {
		t.captureTimer.Stop()
	}
	cancelled := t.ctx.Err() != nil
	if cancelled {
		t.setOutcome("cancelled")
	}
	t.takePCM() // a turn that ends early (cancelled, failed, no speech) must not keep audio
	if cancelled && !t.responded.Load() && !t.silent.Load() {
		// The turn was abandoned before anything was said or done; tell the
		// client so it never waits on a result that will not come.
		c.sendError(t.requestID, CodeTurnCancelled, "the request was cancelled before it completed", true, t.streamLabel())
	}
	t.setOutcome("completed")
	t.cancel(nil)

	c.mu.Lock()
	if c.turn == t {
		c.turn = nil
	}
	c.mu.Unlock()

	if t.isAudio {
		outcome, _ := t.outcome.Load().(string)
		c.svc.Metrics.Inc(MetricStreamsEnded, outcome)
		c.svc.Metrics.Observe(MetricStreamDuration, c.svc.now().Sub(t.startedAt))
		c.svc.Metrics.Add(MetricStreamBytes, t.bytes.Load())
	}
}

func closeReasonFor(status session.Status) string {
	switch status {
	case session.StatusSubmitted:
		return "submitted"
	case session.StatusExpired:
		return "expired"
	default:
		return "inactive"
	}
}
