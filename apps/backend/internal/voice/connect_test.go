package voice

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/ws"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
)

func TestConnect_AuthenticationAndAuthorization(t *testing.T) {
	h := newHarness(t)
	other := uuid.New()
	h.auth.issue("other-token", other, time.Time{})

	rejected := func(t *testing.T, o dialOptions, wantStatus int) {
		t.Helper()
		conn, resp, err := h.dialRaw(o)
		if conn != nil {
			_ = conn.CloseNow()
		}
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, wantStatus, resp.StatusCode)
	}

	t.Run("missing token is rejected before upgrade", func(t *testing.T) {
		rejected(t, dialOptions{}, http.StatusUnauthorized)
	})
	t.Run("invalid token is rejected", func(t *testing.T) {
		rejected(t, dialOptions{token: "forged"}, http.StatusUnauthorized)
	})
	t.Run("another student's session is forbidden", func(t *testing.T) {
		rejected(t, dialOptions{token: "other-token"}, http.StatusForbidden)
	})
	t.Run("unknown session is not found", func(t *testing.T) {
		rejected(t, dialOptions{token: h.token, sessionID: uuid.New()}, http.StatusNotFound)
	})
	t.Run("disallowed origin is forbidden", func(t *testing.T) {
		rejected(t, dialOptions{token: h.token, origin: "https://evil.example.com"}, http.StatusForbidden)
	})
	t.Run("session lookup outage is a 503, not an upgrade", func(t *testing.T) {
		h.exam.fail("GetSession", errors.New("database connection refused"))
		defer h.exam.fail("GetSession", nil)
		rejected(t, dialOptions{token: h.token}, http.StatusServiceUnavailable)
	})

	for _, status := range []session.Status{session.StatusSubmitted, session.StatusExpired, session.StatusPending} {
		t.Run("session in state "+string(status)+" is rejected", func(t *testing.T) {
			h.exam.setStatus(status)
			defer h.exam.setStatus(session.StatusInProgress)
			rejected(t, dialOptions{token: h.token}, http.StatusConflict)
		})
	}

	assert.Zero(t, h.svc.ActiveConnections(), "no rejected attempt may hold a connection slot")

	t.Run("bearer header is accepted", func(t *testing.T) {
		conn, _, err := h.dialRaw(dialOptions{token: h.token, origin: "https://exam.example.com"})
		require.NoError(t, err)
		defer conn.CloseNow()
		assert.Equal(t, Subprotocol, conn.Subprotocol())
	})
	t.Run("token in subprotocol is accepted and never echoed", func(t *testing.T) {
		conn, resp, err := h.dialRaw(dialOptions{token: h.token, subprotocol: true})
		require.NoError(t, err)
		defer conn.CloseNow()
		assert.Equal(t, Subprotocol, conn.Subprotocol())
		assert.NotContains(t, resp.Header.Get("Sec-WebSocket-Protocol"), h.token)
	})
}

func TestConnect_ReadyCarriesAuthoritativeState(t *testing.T) {
	h := newHarness(t)
	_, ready := h.connect()

	assert.Equal(t, ProtocolVersion, ready.Protocol)
	assert.Equal(t, h.sessionID, ready.SessionID)
	assert.False(t, ready.Resumed)
	assert.NotEmpty(t, ready.ConnectionID)
	assert.Equal(t, session.StatusInProgress, ready.Session.Status)
	require.NotNil(t, ready.Session.Question)
	assert.Equal(t, 1, ready.Session.Question.CurrentIndex)
	require.NotNil(t, ready.Session.RemainingSeconds)
	assert.Greater(t, *ready.Session.RemainingSeconds, int64(3500))
	assert.Equal(t, FrameHeaderSize, ready.Limits.BinaryHeaderBytes)
	assert.True(t, ready.Capabilities.STT)
	assert.True(t, ready.Capabilities.TTS)
}

func TestConnect_DisabledReturnsNotFound(t *testing.T) {
	h := newHarness(t, withConfig(func(cfg *config.VoiceConfig) { cfg.Enabled = false }))
	_, resp, err := h.dialRaw(dialOptions{token: h.token})
	require.Error(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestConnect_PerUserLimit(t *testing.T) {
	h := newHarness(t, withConfig(func(cfg *config.VoiceConfig) { cfg.MaxConnectionsPerUser = 1 }))
	h.connect()

	// A second connection for the same session is a reconnect and replaces
	// the first, but while the first is still closing the user is at the limit.
	_, resp, err := h.dialRaw(dialOptions{token: h.token})
	require.Error(t, err)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.EqualValues(t, 1, h.svc.Metrics.Value(MetricConnectionsRejected, "user_limit"))
}

func TestProtocol_MalformedMessages(t *testing.T) {
	h := newHarness(t)
	c, _ := h.connect()

	t.Run("not JSON", func(t *testing.T) {
		c.sendText("{not json")
		assert.Equal(t, CodeInvalidMessage, c.untilError().Code)
	})
	t.Run("unknown envelope field", func(t *testing.T) {
		c.sendText(`{"v":1,"type":"ping","id":"a1","admin":true}`)
		assert.Equal(t, CodeInvalidMessage, c.untilError().Code)
	})
	t.Run("unsupported version", func(t *testing.T) {
		c.sendText(`{"v":2,"type":"ping","id":"a2"}`)
		assert.Equal(t, CodeUnsupportedVersion, c.untilError().Code)
	})
	t.Run("missing id", func(t *testing.T) {
		c.sendText(`{"v":1,"type":"ping"}`)
		assert.Equal(t, CodeInvalidMessage, c.untilError().Code)
	})
	t.Run("unknown type", func(t *testing.T) {
		c.sendText(`{"v":1,"type":"exam.grade","id":"a3"}`)
		env := c.until(TypeError)
		assert.Equal(t, "a3", env.ReplyTo)
		assert.Equal(t, CodeUnknownType, decode[ErrorPayload](t, env).Code)
	})
	t.Run("unknown payload field", func(t *testing.T) {
		c.sendText(`{"v":1,"type":"command","id":"a4","payload":{"intent":"next_question","questionId":"x"}}`)
		assert.Equal(t, CodeInvalidMessage, c.untilError().Code)
	})
	t.Run("command outside the permitted set", func(t *testing.T) {
		c.send(TypeCommand, CommandPayload{Intent: "submit_exam"})
		assert.Equal(t, CodeUnsupportedIntent, c.untilError().Code)
	})
	t.Run("bad binary header", func(t *testing.T) {
		require.NoError(t, c.sendBinary([]byte{9, 9, 9}))
		assert.Equal(t, CodeInvalidFrame, c.untilError().Code)
	})
	t.Run("valid message still works afterwards", func(t *testing.T) {
		id := c.send(TypePing, nil)
		assert.Equal(t, id, c.until(TypePong).ReplyTo)
	})

	assert.Zero(t, h.exam.count("NextQuestion"), "no malformed message may reach the exam services")
}

func TestProtocol_DuplicateRequestIsNotProcessedTwice(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()

	c.sendWithID("dup-1", TypeCommand, CommandPayload{Intent: "next_question"})
	c.until(TypeResponse)
	c.sendWithID("dup-1", TypeCommand, CommandPayload{Intent: "next_question"})

	env := c.until(TypeError)
	assert.Equal(t, "dup-1", env.ReplyTo)
	assert.Equal(t, CodeDuplicateRequest, decode[ErrorPayload](t, env).Code)
	assert.Equal(t, 1, h.exam.count("NextQuestion"))
}

func TestProtocol_OversizedMessageClosesConnection(t *testing.T) {
	h := newHarness(t)
	c, _ := h.connect()

	big := `{"v":1,"type":"ping","id":"big","payload":"` + strings.Repeat("x", int(h.svc.Config.MaxMessageBytes)) + `"}`
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	_ = c.conn.Write(ctx, websocket.MessageText, []byte(big))

	assert.Equal(t, websocket.StatusMessageTooBig, c.closeCode())
	eventually(t, func() bool { return h.svc.ActiveConnections() == 0 }, "connection slot released")
}

func TestProtocol_RepeatedViolationsCloseConnection(t *testing.T) {
	h := newHarness(t)
	c, _ := h.connect()

	for i := 0; i < maxConsecutiveViolations; i++ {
		c.sendText("garbage")
	}
	assert.Equal(t, ws.StatusPolicyViolation, c.closeCode())
}

func TestSessionSync_ReturnsStateFromService(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, _ := h.connect()

	c.send(TypeCommand, CommandPayload{Intent: "next_question"})
	c.until(TypeResponse)

	id := c.send(TypeSessionSync, nil)
	env := c.until(TypeSessionState)
	for env.ReplyTo != id {
		env = c.until(TypeSessionState)
	}
	state := decode[SessionState](t, env)
	require.NotNil(t, state.Question)
	assert.Equal(t, 2, state.Question.CurrentIndex)
}
