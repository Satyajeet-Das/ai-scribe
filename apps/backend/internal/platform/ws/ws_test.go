package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recorder struct {
	mu     sync.Mutex
	texts  []string
	binary int
	block  chan struct{}
}

func (r *recorder) HandleText(_ context.Context, data []byte) {
	if r.block != nil {
		<-r.block
	}
	r.mu.Lock()
	r.texts = append(r.texts, string(data))
	r.mu.Unlock()
}

func (r *recorder) HandleBinary(context.Context, []byte) {
	r.mu.Lock()
	r.binary++
	r.mu.Unlock()
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.texts)
}

// serve starts a server whose handler upgrades and serves one connection.
func serve(t *testing.T, opts Options, h Handler) (url string, conns <-chan *Conn, infos <-chan CloseInfo) {
	t.Helper()
	connCh := make(chan *Conn, 4)
	infoCh := make(chan CloseInfo, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := Accept(w, r, opts)
		if err != nil {
			return
		}
		connCh <- c
		infoCh <- c.Serve(context.Background(), h)
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), connCh, infoCh
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

func wait[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
		panic("unreachable")
	}
}

func TestConn_DeliversFramesAndFlushesBeforeClosing(t *testing.T) {
	rec := &recorder{}
	url, conns, infos := serve(t, Options{}, rec)
	client := dial(t, url)
	server := wait(t, conns)

	ctx := context.Background()
	require.NoError(t, client.Write(ctx, websocket.MessageText, []byte("hello")))
	require.NoError(t, client.Write(ctx, websocket.MessageBinary, []byte{1, 2}))
	require.Eventually(t, func() bool { return rec.count() == 1 }, 5*time.Second, 5*time.Millisecond)

	// Frames queued before Close are still delivered, then the code is sent.
	require.NoError(t, server.Send(Message{Data: []byte("one")}))
	require.NoError(t, server.Send(Message{Data: []byte("dropped"), Drop: func() bool { return true }}))
	require.NoError(t, server.Send(Message{Data: []byte("two")}))
	server.Close(4410, "session_ended")
	server.Close(StatusInternalError, "ignored: the first close wins")

	for _, want := range []string{"one", "two"} {
		_, data, err := client.Read(ctx)
		require.NoError(t, err)
		assert.Equal(t, want, string(data))
	}
	_, _, err := client.Read(ctx)
	assert.Equal(t, websocket.StatusCode(4410), websocket.CloseStatus(err))

	info := wait(t, infos)
	assert.True(t, info.Local)
	assert.Equal(t, "session_ended", info.Reason)
	assert.ErrorIs(t, server.Send(Message{Data: []byte("late")}), ErrClosed)
	assert.ErrorIs(t, server.SendContext(ctx, Message{Data: []byte("late")}), ErrClosed)
	<-server.Done()
}

func TestConn_ReadLimit(t *testing.T) {
	url, _, infos := serve(t, Options{MaxMessageBytes: 128}, &recorder{})
	client := dial(t, url)

	_ = client.Write(context.Background(), websocket.MessageText, []byte(strings.Repeat("x", 4096)))
	_, _, err := client.Read(context.Background())
	assert.Equal(t, websocket.StatusMessageTooBig, websocket.CloseStatus(err))
	wait(t, infos)
}

func TestConn_SendQueueIsBounded(t *testing.T) {
	// The handler blocks the read side; the client never reads. The queue
	// must fill up rather than grow.
	url, conns, _ := serve(t, Options{SendQueueSize: 2, WriteTimeout: 200 * time.Millisecond}, &recorder{})
	dial(t, url)
	server := wait(t, conns)

	payload := []byte(strings.Repeat("x", 32*1024))
	full := false
	for i := 0; i < 4096 && !full; i++ {
		full = server.Send(Message{Data: payload}) == ErrSendQueueFull
	}
	assert.True(t, full, "Send must report a full queue instead of buffering without limit")

	// SendContext waits for space but gives up when its context does.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := server.SendContext(ctx, Message{Data: payload})
	if err != nil {
		assert.True(t, err == context.DeadlineExceeded || err == ErrClosed, "got %v", err)
	}
}

func TestConn_PeerDisconnectEndsServe(t *testing.T) {
	url, conns, infos := serve(t, Options{}, &recorder{})
	client := dial(t, url)
	server := wait(t, conns)

	_ = client.CloseNow()
	info := wait(t, infos)
	assert.False(t, info.Local)
	<-server.Done()
}

func TestConn_HeartbeatDetectsDeadPeer(t *testing.T) {
	url, _, infos := serve(t, Options{PingInterval: 40 * time.Millisecond, PongTimeout: 30 * time.Millisecond}, &recorder{})
	// A client that never reads never answers pings.
	dial(t, url)

	info := wait(t, infos)
	assert.False(t, info.Local)
}

func TestConn_HeartbeatKeepsLivePeer(t *testing.T) {
	url, conns, infos := serve(t, Options{PingInterval: 30 * time.Millisecond, PongTimeout: 25 * time.Millisecond}, &recorder{})
	client := dial(t, url)
	server := wait(t, conns)

	// Reading is what makes the client library answer pings.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			if _, _, err := client.Read(ctx); err != nil {
				return
			}
		}
	}()

	time.Sleep(200 * time.Millisecond)
	select {
	case info := <-infos:
		t.Fatalf("live connection was closed: %+v", info)
	default:
	}
	server.Close(StatusNormalClosure, "done")
	wait(t, infos)
}

func TestRegistry_Limits(t *testing.T) {
	r := NewRegistry(3, 2)

	a1, err := r.Acquire("alice")
	require.NoError(t, err)
	_, err = r.Acquire("alice")
	require.NoError(t, err)
	_, err = r.Acquire("alice")
	assert.ErrorIs(t, err, ErrTooManyOwnerConnections)

	_, err = r.Acquire("bob")
	require.NoError(t, err)
	_, err = r.Acquire("carol")
	assert.ErrorIs(t, err, ErrTooManyConnections)
	assert.Equal(t, 3, r.Count())

	a1.Release()
	a1.Release()
	assert.Equal(t, 2, r.Count(), "a double release frees one slot only")
	_, err = r.Acquire("carol")
	require.NoError(t, err)
}

func TestRegistry_NewestConnectionReplacesAndShutdownWaits(t *testing.T) {
	r := NewRegistry(10, 10)
	first, second := &Conn{closing: make(chan struct{}), done: make(chan struct{})}, &Conn{closing: make(chan struct{}), done: make(chan struct{})}

	l1, _ := r.Acquire("alice")
	replaced, err := l1.Bind("session-1", first)
	require.NoError(t, err)
	assert.Nil(t, replaced)

	l2, _ := r.Acquire("alice")
	replaced, err = l2.Bind("session-1", second)
	require.NoError(t, err)
	assert.Same(t, first, replaced)

	// Releasing the displaced connection must not unregister its successor.
	l1.Release()
	l3, _ := r.Acquire("alice")
	replaced, _ = l3.Bind("session-1", &Conn{closing: make(chan struct{}), done: make(chan struct{})})
	assert.Same(t, second, replaced)
	l3.Release()

	// Shutdown asks live connections to close and waits for their leases.
	short, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, r.Shutdown(short, StatusGoingAway, "bye"), context.DeadlineExceeded)
	select {
	case <-second.closing:
	default:
		t.Fatal("live connection was not asked to close")
	}
	_, err = r.Acquire("alice")
	assert.ErrorIs(t, err, ErrShuttingDown)

	l2.Release()
	require.NoError(t, r.Shutdown(context.Background(), StatusGoingAway, "bye"))
}

func TestCheckOrigin(t *testing.T) {
	allowed := SplitOrigins([]string{"https://exam.example.com, http://localhost:3000"})
	require.Len(t, allowed, 2)

	cases := []struct {
		origin string
		host   string
		want   bool
	}{
		{"", "api.example.com", true},
		{"https://exam.example.com", "api.example.com", true},
		{"HTTPS://EXAM.EXAMPLE.COM", "api.example.com", true},
		{"http://localhost:3000", "api.example.com", true},
		{"https://api.example.com", "api.example.com", true},
		{"http://exam.example.com", "api.example.com", false},
		{"https://exam.example.com.evil.io", "api.example.com", false},
		{"https://evil.io", "api.example.com", false},
		{"null", "api.example.com", false},
		{"::not a url", "api.example.com", false},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "http://"+tc.host+"/", nil)
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		assert.Equal(t, tc.want, CheckOrigin(r, allowed), "origin %q", tc.origin)
	}

	r := httptest.NewRequest(http.MethodGet, "http://api.example.com/", nil)
	r.Header.Set("Origin", "https://anything.example")
	assert.True(t, CheckOrigin(r, []string{"*"}))
}
