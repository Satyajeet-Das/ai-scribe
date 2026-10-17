package voice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/answer"
	"github.com/Satyajeet-Das/ai-scribe/internal/model"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/middleware"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech/speechtest"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/ws"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
	"github.com/Satyajeet-Das/ai-scribe/internal/voice/intent"
)

const waitTimeout = 5 * time.Second

// ---------------------------------------------------------------------------
// Fakes for the exam services and auth
// ---------------------------------------------------------------------------

type fakeAuth struct {
	mu     sync.Mutex
	tokens map[string]*platformauth.AuthenticatedIdentity
}

func (a *fakeAuth) Authenticate(_ context.Context, token string) (*platformauth.AuthenticatedIdentity, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	id, ok := a.tokens[token]
	if !ok {
		return nil, platformauth.ErrInvalidToken
	}
	cp := *id
	return &cp, nil
}

func (a *fakeAuth) issue(token string, userID uuid.UUID, expiresAt time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokens[token] = &platformauth.AuthenticatedIdentity{UserID: userID, Role: platformauth.RoleStudent, Provider: "fake", ExpiresAt: expiresAt}
}

// fakeExam is an in-memory stand-in for the session and answer services. Like
// the real ones, it is the authority: it rejects work on a session that is not
// in progress, and it is the only place answers are stored.
type fakeExam struct {
	mu sync.Mutex

	sess         session.Session
	durationMins int
	questions    []session.StudentQuestionResponse
	index        int
	answers      map[uuid.UUID]answer.Answer
	revisions    []*answer.RevisionInput

	// errs injects an error for a method name.
	errs map[string]error
	// submitGate, when non-nil, blocks SubmitAnswer until a value arrives.
	submitGate chan struct{}
	calls      map[string]int
}

func (f *fakeExam) fail(method string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		delete(f.errs, method)
		return
	}
	f.errs[method] = err
}

func (f *fakeExam) setStatus(s session.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sess.Status = s
}

func (f *fakeExam) count(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

// revisionsOf returns the provenance sent with each SubmitAnswer call.
func (f *fakeExam) revisionsOf() []*answer.RevisionInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*answer.RevisionInput(nil), f.revisions...)
}

func (f *fakeExam) saved(questionID uuid.UUID) (answer.Answer, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.answers[questionID]
	return a, ok
}

func (f *fakeExam) currentQuestionID() uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.questions[f.index].ID
}

// enter records the call and returns an injected error, if any. Caller holds mu.
func (f *fakeExam) enter(method string) error {
	f.calls[method]++
	return f.errs[method]
}

func (f *fakeExam) statusErr() error {
	switch f.sess.Status {
	case session.StatusInProgress:
		return nil
	case session.StatusSubmitted:
		return session.ErrSessionAlreadySubmitted
	case session.StatusExpired:
		return session.ErrSessionExpired
	default:
		return session.ErrSessionNotInProgress
	}
}

func (f *fakeExam) GetSession(_ context.Context, id, _ uuid.UUID) (*session.Session, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("GetSession"); err != nil {
		return nil, 0, err
	}
	if id != f.sess.ID {
		return nil, 0, session.ErrSessionNotFound
	}
	cp := f.sess
	return &cp, f.durationMins, nil
}

func (f *fakeExam) question() *session.StudentQuestionResponse {
	q := f.questions[f.index]
	q.CurrentIndex = f.index + 1
	q.TotalQuestions = len(f.questions)
	q.IsFirst = f.index == 0
	q.IsLast = f.index == len(f.questions)-1
	return &q
}

func (f *fakeExam) GetCurrentQuestion(_ context.Context, id, callerID uuid.UUID) (*session.StudentQuestionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("GetCurrentQuestion"); err != nil {
		return nil, err
	}
	if callerID != f.sess.StudentID {
		return nil, session.ErrUnauthorizedStudent
	}
	if err := f.statusErr(); err != nil {
		return nil, err
	}
	return f.question(), nil
}

func (f *fakeExam) NextQuestion(_ context.Context, id, callerID uuid.UUID) (*session.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("NextQuestion"); err != nil {
		return nil, err
	}
	if err := f.statusErr(); err != nil {
		return nil, err
	}
	if f.index >= len(f.questions)-1 {
		return nil, session.ErrNoNextQuestion
	}
	f.index++
	cp := f.sess
	return &cp, nil
}

func (f *fakeExam) PreviousQuestion(_ context.Context, id, callerID uuid.UUID) (*session.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("PreviousQuestion"); err != nil {
		return nil, err
	}
	if err := f.statusErr(); err != nil {
		return nil, err
	}
	if f.index <= 0 {
		return nil, session.ErrNoPreviousQuestion
	}
	f.index--
	cp := f.sess
	return &cp, nil
}

func (f *fakeExam) SubmitAnswer(ctx context.Context, sessionID, questionID uuid.UUID, req answer.SubmitAnswerRequest, callerID uuid.UUID) (*answer.Answer, error) {
	f.mu.Lock()
	gate := f.submitGate
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("SubmitAnswer"); err != nil {
		return nil, err
	}
	if callerID != f.sess.StudentID {
		return nil, session.ErrUnauthorizedStudent
	}
	if err := f.statusErr(); err != nil {
		return nil, err
	}
	f.revisions = append(f.revisions, req.Revision)
	a, ok := f.answers[questionID]
	if !ok {
		a = answer.Answer{Base: model.Base{BaseWithId: model.BaseWithId{ID: uuid.New()}}, SessionID: sessionID, QuestionID: questionID}
	}
	a.SelectedOptionID = req.SelectedOptionID
	a.TextAnswer = req.TextAnswer
	a.UpdatedAt = time.Now().UTC()
	f.answers[questionID] = a
	return &a, nil
}

func (f *fakeExam) GetAnswer(_ context.Context, sessionID, questionID, callerID uuid.UUID) (*answer.Answer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("GetAnswer"); err != nil {
		return nil, err
	}
	a, ok := f.answers[questionID]
	if !ok {
		return nil, answer.ErrAnswerNotFound
	}
	return &a, nil
}

// classifierFunc adapts a function to intent.Classifier and records inputs.
type fakeClassifier struct {
	mu     sync.Mutex
	fn     func(ctx context.Context, in intent.Input) (intent.Intent, error)
	inputs []intent.Input
}

func (c *fakeClassifier) Name() string { return "fake-classifier" }

func (c *fakeClassifier) Classify(ctx context.Context, in intent.Input) (intent.Intent, error) {
	c.mu.Lock()
	c.inputs = append(c.inputs, in)
	fn := c.fn
	c.mu.Unlock()
	if fn == nil {
		return intent.Rules{}.Classify(ctx, in)
	}
	return fn(ctx, in)
}

func (c *fakeClassifier) set(fn func(ctx context.Context, in intent.Input) (intent.Intent, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fn = fn
}

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

type harness struct {
	t      *testing.T
	svc    *Service
	server *httptest.Server
	auth   *fakeAuth
	exam   *fakeExam
	stt    *speechtest.STT
	tts    *speechtest.TTS
	cls    *fakeClassifier
	rec    *fakeRecorder

	student   uuid.UUID
	sessionID uuid.UUID
	token     string

	convMu sync.Mutex
	convs  []*conversation
}

// onlyConversation returns the most recent server-side conversation.
func (h *harness) onlyConversation(t *testing.T) *conversation {
	t.Helper()
	h.convMu.Lock()
	defer h.convMu.Unlock()
	require.NotEmpty(t, h.convs)
	return h.convs[len(h.convs)-1]
}

func wsFrame(id uuid.UUID, seq uint32, drop func() bool) ws.Message {
	return ws.Message{Binary: true, Data: encodeFrame(id, seq, make([]byte, 320)), Drop: drop}
}

// untilAudio reads until the audio frame with the given sequence number.
func (c *client) untilAudio(seq uint32) audioFrame {
	c.t.Helper()
	for {
		f, err := c.read()
		require.NoError(c.t, err)
		if f.audio != nil && f.audio.Seq == seq {
			return *f.audio
		}
	}
}

type harnessOption func(cfg *config.VoiceConfig, h *harness)

func withConfig(fn func(cfg *config.VoiceConfig)) harnessOption {
	return func(cfg *config.VoiceConfig, _ *harness) { fn(cfg) }
}

// withRecorder turns on retention of dictated answers, with a fake store.
func withRecorder() harnessOption {
	return func(_ *config.VoiceConfig, h *harness) { h.rec = &fakeRecorder{} }
}

func withoutTTS() harnessOption {
	return func(_ *config.VoiceConfig, h *harness) { h.tts = nil }
}

func newHarness(t *testing.T, opts ...harnessOption) *harness {
	t.Helper()
	baseline := runtime.NumGoroutine()

	student := uuid.New()
	sessionID := uuid.New()
	examID := uuid.New()

	mcq := func(n int, text string, opts ...string) session.StudentQuestionResponse {
		q := session.StudentQuestionResponse{ID: uuid.New(), ExamID: examID, QuestionNumber: n, Text: text, Type: question.TypeMCQ, Points: 1}
		for i, o := range opts {
			q.Options = append(q.Options, session.StudentOptionResponse{ID: uuid.New(), OptionKey: string(rune('A' + i)), OptionText: o, DisplayOrder: i})
		}
		return q
	}

	h := &harness{
		t:         t,
		student:   student,
		sessionID: sessionID,
		token:     "student-token",
		auth:      &fakeAuth{tokens: map[string]*platformauth.AuthenticatedIdentity{}},
		stt:       &speechtest.STT{},
		tts:       &speechtest.TTS{},
		cls:       &fakeClassifier{},
		exam: &fakeExam{
			sess: session.Session{
				Base:      model.Base{BaseWithId: model.BaseWithId{ID: sessionID}},
				ExamID:    examID,
				StudentID: student,
				Status:    session.StatusInProgress,
				StartedAt: time.Now().UTC(),
			},
			durationMins: 60,
			questions: []session.StudentQuestionResponse{
				mcq(1, "What is the capital of France?", "London", "Paris", "Rome"),
				mcq(2, "Which planet is known as the red planet?", "Venus", "Mars"),
				{ID: uuid.New(), ExamID: examID, QuestionNumber: 3, Text: "Describe photosynthesis.", Type: question.TypeEssay, Points: 5},
			},
			answers: map[uuid.UUID]answer.Answer{},
			errs:    map[string]error{},
			calls:   map[string]int{},
		},
	}
	h.auth.issue(h.token, student, time.Time{})

	cfg := config.DefaultVoiceConfig()
	cfg.STT.FinalTimeout = 500 * time.Millisecond
	cfg.Audio.EnqueueTimeout = 200 * time.Millisecond
	for _, o := range opts {
		o(&cfg, h)
	}

	logger := zerolog.Nop()
	deps := Deps{
		Config:         cfg,
		AllowedOrigins: []string{"https://exam.example.com"},
		Auth:           h.auth,
		Sessions:       h.exam,
		Answers:        h.exam,
		STT:            h.stt,
		Classifier:     h.cls,
		Metrics:        NewMetrics(nil),
		Logger:         &logger,
	}
	if h.tts != nil {
		deps.TTS = h.tts
	}
	if h.rec != nil {
		deps.Recorder = h.rec
	}
	h.svc = NewService(deps)
	h.svc.onConversation = func(c *conversation) {
		h.convMu.Lock()
		h.convs = append(h.convs, c)
		h.convMu.Unlock()
	}

	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = middleware.NewGlobalMiddlewares(&logger, nil).GlobalErrorHandler
	h.svc.RegisterRoutes(e.Group("/api/v1"))
	h.server = httptest.NewServer(e)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
		defer cancel()
		_ = h.svc.Shutdown(ctx)
		h.server.CloseClientConnections()
		h.server.Close()
		assertNoGoroutineLeak(t, baseline)
	})
	return h
}

// assertNoGoroutineLeak waits for the goroutine count to return to its
// baseline. Every connection, turn, pump and timer must be gone.
func assertNoGoroutineLeak(t *testing.T, baseline int) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for {
		n := runtime.NumGoroutine()
		if n <= baseline {
			return
		}
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<16)
			buf = buf[:runtime.Stack(buf, true)]
			t.Errorf("goroutine leak: %d running, baseline %d\n%s", n, baseline, buf)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (h *harness) url(sessionID uuid.UUID) string {
	return "ws" + strings.TrimPrefix(h.server.URL, "http") + "/api/v1/sessions/" + sessionID.String() + "/voice"
}

type dialOptions struct {
	token       string
	subprotocol bool
	origin      string
	sessionID   uuid.UUID
}

// dialRaw attempts the upgrade and returns the HTTP response on failure.
func (h *harness) dialRaw(o dialOptions) (*websocket.Conn, *http.Response, error) {
	if o.sessionID == uuid.Nil {
		o.sessionID = h.sessionID
	}
	header := http.Header{}
	opts := &websocket.DialOptions{HTTPHeader: header, Subprotocols: []string{Subprotocol}}
	if o.origin != "" {
		header.Set("Origin", o.origin)
	}
	if o.token != "" {
		if o.subprotocol {
			opts.Subprotocols = append(opts.Subprotocols, AuthSubprotocolPrefix+o.token)
		} else {
			header.Set("Authorization", "Bearer "+o.token)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	return websocket.Dial(ctx, h.url(o.sessionID), opts)
}

// connect dials as the session's student and consumes the ready message.
func (h *harness) connect() (*client, ReadyPayload) {
	h.t.Helper()
	conn, _, err := h.dialRaw(dialOptions{token: h.token})
	require.NoError(h.t, err)
	conn.SetReadLimit(1 << 20)
	c := &client{t: h.t, conn: conn}
	h.t.Cleanup(func() { _ = conn.CloseNow() })
	ready := decode[ReadyPayload](h.t, c.until(TypeReady))
	return c, ready
}

// ---------------------------------------------------------------------------
// Test client
// ---------------------------------------------------------------------------

type frame struct {
	env   *Envelope
	audio *audioFrame
}

type client struct {
	t    *testing.T
	conn *websocket.Conn
	seq  int
	// seen is every frame read so far, in order.
	seen []frame
}

func (c *client) nextID() string {
	c.seq++
	return "req-" + uuid.NewString()[:8] + "-" + string(rune('a'+c.seq%26))
}

func (c *client) send(typ string, payload any) string {
	c.t.Helper()
	id := c.nextID()
	c.sendWithID(id, typ, payload)
	return id
}

func (c *client) sendWithID(id, typ string, payload any) {
	c.t.Helper()
	env := Envelope{V: ProtocolVersion, Type: typ, ID: id}
	if payload != nil {
		raw, err := json.Marshal(payload)
		require.NoError(c.t, err)
		env.Payload = raw
	}
	data, err := json.Marshal(env)
	require.NoError(c.t, err)
	c.sendText(string(data))
}

func (c *client) sendText(text string) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	require.NoError(c.t, c.conn.Write(ctx, websocket.MessageText, []byte(text)))
}

func (c *client) sendBinary(data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	return c.conn.Write(ctx, websocket.MessageBinary, data)
}

func (c *client) chunk(stream uuid.UUID, seq uint32, size int) {
	c.t.Helper()
	require.NoError(c.t, c.sendBinary(encodeFrame(stream, seq, make([]byte, size))))
}

// read returns the next frame or the error that ended the connection.
func (c *client) read() (frame, error) {
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	typ, data, err := c.conn.Read(ctx)
	if err != nil {
		return frame{}, err
	}
	var f frame
	if typ == websocket.MessageBinary {
		af, err := decodeFrame(data)
		require.NoError(c.t, err)
		f.audio = &af
	} else {
		var env Envelope
		require.NoError(c.t, json.Unmarshal(data, &env))
		require.Equal(c.t, ProtocolVersion, env.V)
		f.env = &env
	}
	c.seen = append(c.seen, f)
	return f, nil
}

// until reads frames until a JSON message of the given type arrives.
func (c *client) until(typ string) Envelope {
	c.t.Helper()
	for {
		f, err := c.read()
		require.NoError(c.t, err, "waiting for %q", typ)
		if f.env != nil && f.env.Type == typ {
			return *f.env
		}
	}
}

// untilError reads until an error message and returns its payload.
func (c *client) untilError() ErrorPayload {
	c.t.Helper()
	return decode[ErrorPayload](c.t, c.until(TypeError))
}

// closeCode reads until the connection ends and returns the close code.
func (c *client) closeCode() websocket.StatusCode {
	c.t.Helper()
	for {
		_, err := c.read()
		if err != nil {
			var ce websocket.CloseError
			if errors.As(err, &ce) {
				return ce.Code
			}
			return websocket.CloseStatus(err)
		}
	}
}

// types lists the JSON message types seen since index from.
func (c *client) types(from int) []string {
	var out []string
	for _, f := range c.seen[from:] {
		if f.env != nil {
			out = append(out, f.env.Type)
		}
	}
	return out
}

func decode[T any](t *testing.T, env Envelope) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(env.Payload, &v), "payload of %q", env.Type)
	return v
}

// speak sends one complete utterance: start, n chunks, end.
func (c *client) speak(chunks int) (uuid.UUID, string) {
	c.t.Helper()
	return c.speakMode(chunks, "")
}

// speakMode is speak with an explicit audio mode ("" leaves it unset).
func (c *client) speakMode(chunks int, mode string) (uuid.UUID, string) {
	c.t.Helper()
	stream := uuid.New()
	id := c.send(TypeAudioStart, AudioStartPayload{StreamID: stream.String(), Encoding: "pcm_s16le", SampleRate: 16000, Channels: 1, Mode: mode})
	for i := 0; i < chunks; i++ {
		c.chunk(stream, uint32(i), 640)
	}
	n := uint32(chunks)
	c.send(TypeAudioEnd, AudioEndPayload{StreamID: stream.String(), Chunks: &n})
	return stream, id
}

// say scripts the recognizer's transcript, then speaks one utterance.
func (h *harness) say(c *client, transcript string) (uuid.UUID, string) {
	h.t.Helper()
	h.stt.Set(func(s *speechtest.STT) { s.Finals = []string{transcript} })
	return c.speak(2)
}

// sayAs is say with an explicit audio mode.
func (h *harness) sayAs(c *client, mode, transcript string) (uuid.UUID, string) {
	h.t.Helper()
	h.stt.Set(func(s *speechtest.STT) { s.Finals = []string{transcript} })
	return c.speakMode(2, mode)
}

func eventually(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	require.Eventually(t, cond, waitTimeout, 10*time.Millisecond, msg)
}
