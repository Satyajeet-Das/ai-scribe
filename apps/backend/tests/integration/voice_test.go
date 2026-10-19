package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/answer"
	"github.com/Satyajeet-Das/ai-scribe/internal/assignment"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	jwtadapter "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth/adapters/jwt"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/middleware"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/logger"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/redis"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech/speechtest"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
	"github.com/Satyajeet-Das/ai-scribe/internal/user"
	"github.com/Satyajeet-Das/ai-scribe/internal/voice"
	"github.com/Satyajeet-Das/ai-scribe/internal/voice/intent"
)

// voiceClient is a minimal protocol client for the integration tests.
type voiceClient struct {
	t    *testing.T
	conn *websocket.Conn
	n    int
}

func (c *voiceClient) command(p voice.CommandPayload) string {
	c.t.Helper()
	c.n++
	id := "it-" + uuid.NewString()[:8]
	raw, err := json.Marshal(p)
	require.NoError(c.t, err)
	data, err := json.Marshal(voice.Envelope{V: voice.ProtocolVersion, Type: voice.TypeCommand, ID: id, Payload: raw})
	require.NoError(c.t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(c.t, c.conn.Write(ctx, websocket.MessageText, data))
	return id
}

// until reads until a message of the given type that answers request id
// (any request when id is empty).
func (c *voiceClient) until(typ, id string) voice.Envelope {
	c.t.Helper()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_, data, err := c.conn.Read(ctx)
		cancel()
		require.NoError(c.t, err, "waiting for %s", typ)
		var env voice.Envelope
		if json.Unmarshal(data, &env) != nil {
			continue // binary audio
		}
		if env.Type == typ && (id == "" || env.ReplyTo == id) {
			return env
		}
	}
}

func (c *voiceClient) closeCode() websocket.StatusCode {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_, _, err := c.conn.Read(ctx)
		cancel()
		if err != nil {
			return websocket.CloseStatus(err)
		}
	}
}

func payloadOf[T any](t *testing.T, env voice.Envelope) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(env.Payload, &v))
	return v
}

// TestVoice_EndToEndAgainstRealServices drives the voice endpoint with the
// real session and answer services, PostgreSQL, Redis and JWT authentication.
// Only the speech providers are faked.
func TestVoice_EndToEndAgainstRealServices(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testDB, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	logSvc := logger.NewLoggerService(testDB.Config.Observability)
	l := logger.NewLoggerWithService(testDB.Config.Observability, logSvc)

	redisClient := redis.New(testDB.Config, &l, logSvc)
	defer redisClient.Close()

	userRepo := user.NewRepository(testDB.Pool)
	examRepo := exam.NewRepository(testDB.Pool)
	assignmentRepo := assignment.NewRepository(testDB.Pool)
	questionRepo := question.NewRepository(testDB.Pool)
	sessionRepo := session.NewRepository(testDB.Pool)
	answerRepo := answer.NewRepository(testDB.Pool)

	examSvc := exam.NewService(examRepo, &l)
	assignmentSvc := assignment.NewService(assignmentRepo, examSvc, userRepo, &l)
	txMgr := &dbTxManager{pool: testDB.Pool}
	questionSvc := question.NewService(questionRepo, examSvc, txMgr, &l)
	sessionSvc := session.NewService(sessionRepo, assignmentSvc, examSvc, questionSvc, session.NewRedisCache(redisClient), txMgr, &mockTaskEnqueuer{}, &l)
	answerSvc := answer.NewService(answerRepo, sessionSvc, questionSvc, &l)

	// --- seed -------------------------------------------------------------
	newUser := func(email, role string) *user.User {
		clerkID := uuid.NewString()
		u := &user.User{ClerkID: &clerkID, Email: email, PasswordHash: "hash", Role: role, FirstName: "T", LastName: "U", IsActive: true}
		require.NoError(t, userRepo.Create(ctx, u))
		return u
	}
	student := newUser("voice_student@test.com", "STUDENT")
	intruder := newUser("voice_intruder@test.com", "STUDENT")
	teacher := newUser("voice_teacher@test.com", "TEACHER")

	ex := &exam.Exam{CreatedBy: teacher.ID, Title: "Voice Exam", Subject: "Geography", DurationMins: 60, Status: exam.StatusPublished}
	require.NoError(t, examRepo.Create(ctx, ex))

	q1 := &question.Question{ExamID: ex.ID, QuestionNumber: 1, Type: question.TypeMCQ, Text: "Capital of France?", Points: 1}
	require.NoError(t, questionRepo.Create(ctx, q1))
	options := map[string]*question.QuestionOption{}
	for i, o := range []struct {
		key, text string
		correct   bool
	}{{"A", "London", false}, {"B", "Paris", true}, {"C", "Rome", false}} {
		opt := &question.QuestionOption{QuestionID: q1.ID, OptionKey: o.key, OptionText: o.text, DisplayOrder: i, IsCorrect: o.correct}
		require.NoError(t, questionRepo.CreateOption(ctx, opt))
		options[o.key] = opt
	}
	q2 := &question.Question{ExamID: ex.ID, QuestionNumber: 2, Type: question.TypeEssay, Text: "Describe a river.", Points: 5}
	require.NoError(t, questionRepo.Create(ctx, q2))

	asgn := &assignment.Assignment{ExamID: ex.ID, StudentID: student.ID, Status: assignment.StatusAssigned}
	require.NoError(t, assignmentRepo.Create(ctx, asgn))
	sess, _, err := sessionSvc.StartSession(ctx, session.StartSessionRequest{AssignmentID: &asgn.ID}, student.ID)
	require.NoError(t, err)

	// --- voice service behind a real router ---------------------------------
	jwtCfg := jwtadapter.DefaultConfig("integration-secret")
	authProvider := jwtadapter.NewAdapter(jwtCfg, userRepo, redisClient, &l)
	token := func(u *user.User) string {
		tok, _, _, err := jwtadapter.GenerateAccessToken(jwtCfg, u.ID, platformauth.RoleStudent, u.Email)
		require.NoError(t, err)
		return tok
	}

	stt := &speechtest.STT{}
	voiceCfg := config.DefaultVoiceConfig()
	voiceCfg.MaxConnectionsPerUser = 10
	svc := voice.NewService(voice.Deps{
		Config:     voiceCfg,
		Auth:       authProvider,
		Sessions:   sessionSvc,
		Answers:    answerSvc,
		STT:        stt,
		TTS:        &speechtest.TTS{},
		Classifier: intent.Rules{},
		Logger:     &l,
	})
	session.NotifyOnEnd(sessionSvc, svc)
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = middleware.NewGlobalMiddlewares(&l, nil).GlobalErrorHandler
	svc.RegisterRoutes(e.Group("/api/v1"))
	srv := httptest.NewServer(e)
	defer srv.Close()
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = svc.Shutdown(sctx)
	}()

	dial := func(tok string) (*voiceClient, *http.Response, error) {
		dctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/sessions/" + sess.ID.String() + "/voice"
		conn, resp, err := websocket.Dial(dctx, url, &websocket.DialOptions{
			Subprotocols: []string{voice.Subprotocol, voice.AuthSubprotocolPrefix + tok},
		})
		if err != nil {
			return nil, resp, err
		}
		conn.SetReadLimit(1 << 20)
		t.Cleanup(func() { _ = conn.CloseNow() })
		return &voiceClient{t: t, conn: conn}, resp, nil
	}

	countAnswers := func() (rows int, selected *uuid.UUID) {
		require.NoError(t, testDB.Pool.QueryRow(ctx, `SELECT count(*) FROM answers WHERE session_id = $1 AND question_id = $2`, sess.ID, q1.ID).Scan(&rows))
		if rows == 1 {
			require.NoError(t, testDB.Pool.QueryRow(ctx, `SELECT selected_option_id FROM answers WHERE session_id = $1 AND question_id = $2`, sess.ID, q1.ID).Scan(&selected))
		}
		return rows, selected
	}

	t.Run("a real JWT for another student is refused", func(t *testing.T) {
		_, resp, err := dial(token(intruder))
		require.Error(t, err)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
	t.Run("a forged JWT is refused", func(t *testing.T) {
		forged, _, _, err := jwtadapter.GenerateAccessToken(jwtadapter.DefaultConfig("other-secret"), student.ID, platformauth.RoleStudent, student.Email)
		require.NoError(t, err)
		_, resp, err := dial(forged)
		require.Error(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	c, _, err := dial(token(student))
	require.NoError(t, err)
	ready := payloadOf[voice.ReadyPayload](t, c.until(voice.TypeReady, ""))

	t.Run("ready exposes the question without the answer key", func(t *testing.T) {
		require.NotNil(t, ready.Session.Question)
		assert.Equal(t, q1.ID, ready.Session.Question.ID)
		raw, err := json.Marshal(ready)
		require.NoError(t, err)
		lower := strings.ToLower(string(raw))
		assert.NotContains(t, lower, "iscorrect")
		assert.NotContains(t, lower, "is_correct")
	})

	t.Run("selection persists only after confirmation", func(t *testing.T) {
		id := c.command(voice.CommandPayload{Intent: "select_option", OptionKey: "A"})
		confirm := payloadOf[voice.ConfirmRequestPayload](t, c.until(voice.TypeConfirmRequest, id))
		c.until(voice.TypeResponse, id)
		rows, _ := countAnswers()
		assert.Zero(t, rows, "nothing is written before the student confirms")

		id = c.command(voice.CommandPayload{Intent: "confirm", ConfirmationID: confirm.ConfirmationID})
		result := payloadOf[voice.ActionResultPayload](t, c.until(voice.TypeActionResult, id))
		require.True(t, result.Saved)
		rows, selected := countAnswers()
		require.Equal(t, 1, rows)
		require.NotNil(t, selected)
		assert.Equal(t, options["A"].ID, *selected)

		// The same confirmation again: reported as a duplicate, no new write.
		var before time.Time
		require.NoError(t, testDB.Pool.QueryRow(ctx, `SELECT updated_at FROM answers WHERE session_id = $1 AND question_id = $2`, sess.ID, q1.ID).Scan(&before))
		id = c.command(voice.CommandPayload{Intent: "confirm", ConfirmationID: confirm.ConfirmationID})
		dup := payloadOf[voice.ActionResultPayload](t, c.until(voice.TypeActionResult, id))
		assert.True(t, dup.Duplicate)
		var after time.Time
		require.NoError(t, testDB.Pool.QueryRow(ctx, `SELECT updated_at FROM answers WHERE session_id = $1 AND question_id = $2`, sess.ID, q1.ID).Scan(&after))
		assert.True(t, before.Equal(after), "a duplicate confirm must not touch the row")
	})

	t.Run("a spoken answer follows the same path", func(t *testing.T) {
		stt.Set(func(s *speechtest.STT) { s.Finals = []string{"option C"} })
		stream := uuid.New()
		send := func(typ string, p any) string {
			raw, _ := json.Marshal(p)
			id := "it-" + uuid.NewString()[:8]
			data, _ := json.Marshal(voice.Envelope{V: voice.ProtocolVersion, Type: typ, ID: id, Payload: raw})
			require.NoError(t, c.conn.Write(ctx, websocket.MessageText, data))
			return id
		}
		startID := send(voice.TypeAudioStart, voice.AudioStartPayload{StreamID: stream.String(), Encoding: "pcm_s16le", SampleRate: 16000, Channels: 1})
		frame := make([]byte, voice.FrameHeaderSize+640)
		frame[0] = 1
		copy(frame[1:17], stream[:])
		require.NoError(t, c.conn.Write(ctx, websocket.MessageBinary, frame))
		one := uint32(1)
		send(voice.TypeAudioEnd, voice.AudioEndPayload{StreamID: stream.String(), Chunks: &one})

		confirm := payloadOf[voice.ConfirmRequestPayload](t, c.until(voice.TypeConfirmRequest, startID))
		assert.Equal(t, "C", confirm.OptionKey)
		_, selected := countAnswers()
		assert.Equal(t, options["A"].ID, *selected, "the earlier answer stands until the new one is confirmed")

		id := c.command(voice.CommandPayload{Intent: "confirm"})
		require.True(t, payloadOf[voice.ActionResultPayload](t, c.until(voice.TypeActionResult, id)).Saved)
		rows, selected := countAnswers()
		assert.Equal(t, 1, rows)
		assert.Equal(t, options["C"].ID, *selected)
	})

	t.Run("voice and REST writers racing leave exactly one consistent row", func(t *testing.T) {
		id := c.command(voice.CommandPayload{Intent: "select_option", OptionKey: "B"})
		c.until(voice.TypeResponse, id)

		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				key := []string{"A", "C"}[i%2]
				optID := options[key].ID
				// The same service the REST handler calls.
				_, _ = answerSvc.SubmitAnswer(context.Background(), sess.ID, q1.ID, answer.SubmitAnswerRequest{SelectedOptionID: &optID}, student.ID)
			}(i)
		}
		confirmID := c.command(voice.CommandPayload{Intent: "confirm"})
		result := payloadOf[voice.ActionResultPayload](t, c.until(voice.TypeActionResult, confirmID))
		wg.Wait()

		rows, selected := countAnswers()
		require.Equal(t, 1, rows, "the unique (session, question) row is never duplicated")
		require.NotNil(t, selected)
		valid := *selected == options["A"].ID || *selected == options["B"].ID || *selected == options["C"].ID
		assert.True(t, valid)
		if !result.Saved {
			// Lock contention is allowed to fail the write; it must then be
			// reported as not saved and retryable, never as saved.
			assert.Equal(t, voice.StatusFailed, result.Status)
			assert.True(t, result.Retryable)
		}
	})

	t.Run("navigation is persisted by the session service", func(t *testing.T) {
		id := c.command(voice.CommandPayload{Intent: "next_question"})
		result := payloadOf[voice.ActionResultPayload](t, c.until(voice.TypeActionResult, id))
		require.Equal(t, voice.StatusOK, result.Status)
		var current uuid.UUID
		require.NoError(t, testDB.Pool.QueryRow(ctx, `SELECT current_question_id FROM sessions WHERE id = $1`, sess.ID).Scan(&current))
		assert.Equal(t, q2.ID, current)

		id = c.command(voice.CommandPayload{Intent: "next_question"})
		result = payloadOf[voice.ActionResultPayload](t, c.until(voice.TypeActionResult, id))
		assert.Equal(t, voice.StatusRejected, result.Status)
		assert.Equal(t, voice.CodeNoNextQuestion, result.Code)
	})

	t.Run("reconnect resumes from the persisted position", func(t *testing.T) {
		fresh, _, err := dial(token(student))
		require.NoError(t, err)
		r := payloadOf[voice.ReadyPayload](t, fresh.until(voice.TypeReady, ""))
		assert.True(t, r.Resumed)
		assert.Equal(t, q2.ID, r.Session.Question.ID)
		assert.Equal(t, websocket.StatusCode(voice.CloseReplaced), c.closeCode())
		c = fresh
	})

	t.Run("submitting closes the voice connection at once, and reconnects are refused", func(t *testing.T) {
		start := time.Now()
		_, err := sessionSvc.SubmitSession(ctx, sess.ID, student.ID)
		require.NoError(t, err)

		closed := payloadOf[voice.SessionClosedPayload](t, c.until(voice.TypeSessionClosed, ""))
		assert.Equal(t, "submitted", closed.Reason)
		assert.Equal(t, websocket.StatusCode(voice.CloseSessionEnded), c.closeCode())
		assert.Less(t, time.Since(start), 5*time.Second, "not left to the periodic re-check")

		var current uuid.UUID
		require.NoError(t, testDB.Pool.QueryRow(ctx, `SELECT current_question_id FROM sessions WHERE id = $1`, sess.ID).Scan(&current))
		assert.Equal(t, q2.ID, current, "a submitted session does not move")

		_, resp, err := dial(token(student))
		require.Error(t, err)
		assert.Equal(t, http.StatusConflict, resp.StatusCode)
	})
}
