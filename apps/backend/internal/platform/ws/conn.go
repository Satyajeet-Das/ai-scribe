// Package ws is the WebSocket transport: connection lifecycle, a single-writer
// bounded send queue, heartbeats, deadlines and connection accounting.
// It knows nothing about exams, sessions or audio.
package ws

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// StatusCode is a WebSocket close code.
type StatusCode = websocket.StatusCode

const (
	StatusNormalClosure   = websocket.StatusNormalClosure
	StatusGoingAway       = websocket.StatusGoingAway
	StatusPolicyViolation = websocket.StatusPolicyViolation
	StatusMessageTooBig   = websocket.StatusMessageTooBig
	StatusInternalError   = websocket.StatusInternalError
	StatusTryAgainLater   = websocket.StatusTryAgainLater
)

var (
	// ErrClosed is returned when sending on a connection that is closed or closing.
	ErrClosed = errors.New("ws: connection closed")
	// ErrSendQueueFull is returned by Send when the peer is not draining fast enough.
	ErrSendQueueFull = errors.New("ws: send queue full")
)

// Message is one outbound frame.
type Message struct {
	Binary bool
	Data   []byte
	// Drop, when set, is consulted immediately before the frame is written.
	// Returning true discards the frame, which lets a producer invalidate
	// frames that are already queued (for example a cancelled audio response).
	Drop func() bool
}

// Handler receives inbound frames. Both methods are called sequentially from
// the connection's single read goroutine, so a handler never sees two frames
// concurrently. A slow handler therefore applies backpressure to the peer.
type Handler interface {
	HandleText(ctx context.Context, data []byte)
	HandleBinary(ctx context.Context, data []byte)
}

// Options bounds the resources one connection may use.
type Options struct {
	Subprotocols    []string
	MaxMessageBytes int64
	SendQueueSize   int
	WriteTimeout    time.Duration
	PingInterval    time.Duration
	PongTimeout     time.Duration
}

func (o *Options) applyDefaults() {
	if o.MaxMessageBytes <= 0 {
		o.MaxMessageBytes = 64 * 1024
	}
	if o.SendQueueSize <= 0 {
		o.SendQueueSize = 64
	}
	if o.WriteTimeout <= 0 {
		o.WriteTimeout = 10 * time.Second
	}
	if o.PingInterval <= 0 {
		o.PingInterval = 20 * time.Second
	}
	if o.PongTimeout <= 0 {
		o.PongTimeout = 10 * time.Second
	}
}

// CloseInfo describes why a connection ended.
type CloseInfo struct {
	// Code is the close code sent locally, or received from the peer.
	Code StatusCode
	// Reason is a short machine-readable label.
	Reason string
	// Local is true when this side initiated the close.
	Local bool
}

// Conn is a WebSocket connection with exactly one writer goroutine.
type Conn struct {
	ws   *websocket.Conn
	opts Options
	send chan Message

	ctx    context.Context
	cancel context.CancelFunc

	closeOnce sync.Once
	closing   chan struct{}
	closeCode StatusCode
	closeText string

	done chan struct{}
}

// Accept upgrades the request. The caller must already have authenticated the
// request and verified its Origin (see CheckOrigin); Accept performs neither.
// On failure a response has already been written.
func Accept(w http.ResponseWriter, r *http.Request, opts Options) (*Conn, error) {
	opts.applyDefaults()
	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: opts.Subprotocols,
		// Origin is verified by the caller against the configured allow-list
		// before the upgrade, so the library's same-host check is redundant.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, err
	}
	wsConn.SetReadLimit(opts.MaxMessageBytes)

	return &Conn{
		ws:      wsConn,
		opts:    opts,
		send:    make(chan Message, opts.SendQueueSize),
		closing: make(chan struct{}),
		done:    make(chan struct{}),
	}, nil
}

// Subprotocol returns the negotiated subprotocol.
func (c *Conn) Subprotocol() string { return c.ws.Subprotocol() }

// Done is closed once Serve has returned and every goroutine has exited.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Send queues a frame without blocking. ErrSendQueueFull means the peer is a
// slow consumer; the caller decides whether to drop the frame or the peer.
func (c *Conn) Send(m Message) error {
	if c.isClosing() {
		return ErrClosed
	}
	select {
	case c.send <- m:
		return nil
	default:
		return ErrSendQueueFull
	}
}

// SendContext queues a frame, waiting for queue space until ctx is done.
// Blocking here is how backpressure reaches a streaming producer.
func (c *Conn) SendContext(ctx context.Context, m Message) error {
	if c.isClosing() {
		return ErrClosed
	}
	select {
	case c.send <- m:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closing:
		return ErrClosed
	case <-c.done:
		return ErrClosed
	}
}

// Close starts a graceful close: frames already queued are flushed, then the
// close handshake runs with the given code. It never blocks and is idempotent;
// the first call wins.
func (c *Conn) Close(code StatusCode, reason string) {
	c.closeOnce.Do(func() {
		c.closeCode = code
		c.closeText = reason
		close(c.closing)
	})
}

func (c *Conn) isClosing() bool {
	select {
	case <-c.closing:
		return true
	case <-c.done:
		return true
	default:
		return false
	}
}

// Serve runs the connection until the peer disconnects, Close is called, or
// ctx is cancelled. It blocks, and on return no goroutine started by the
// connection is still running.
func (c *Conn) Serve(ctx context.Context, h Handler) CloseInfo {
	c.ctx, c.cancel = context.WithCancel(ctx)
	defer close(c.done)
	defer c.cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		c.writeLoop()
	}()
	go func() {
		defer wg.Done()
		c.pingLoop()
	}()

	readErr := c.readLoop(h)

	// The read side is finished. If nobody asked for a close, the peer went
	// away (or broke the protocol) and there is nothing left to flush.
	info := CloseInfo{Code: websocket.CloseStatus(readErr), Reason: "peer_closed"}
	select {
	case <-c.closing:
		info = CloseInfo{Code: c.closeCode, Reason: c.closeText, Local: true}
	default:
		if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
			info.Reason = "context_done"
		} else if info.Code == -1 {
			info.Reason = "connection_lost"
		}
		c.cancel()
	}

	wg.Wait()
	return info
}

func (c *Conn) readLoop(h Handler) error {
	for {
		typ, data, err := c.ws.Read(c.ctx)
		if err != nil {
			return err
		}
		switch typ {
		case websocket.MessageText:
			h.HandleText(c.ctx, data)
		case websocket.MessageBinary:
			h.HandleBinary(c.ctx, data)
		}
	}
}

// writeLoop is the only goroutine that writes to the socket.
func (c *Conn) writeLoop() {
	for {
		select {
		case m := <-c.send:
			if !c.write(m) {
				c.abort()
				return
			}
		case <-c.closing:
			c.flushAndClose()
			return
		case <-c.ctx.Done():
			c.abort()
			return
		}
	}
}

func (c *Conn) write(m Message) bool {
	if m.Drop != nil && m.Drop() {
		return true
	}
	typ := websocket.MessageText
	if m.Binary {
		typ = websocket.MessageBinary
	}
	ctx, cancel := context.WithTimeout(c.ctx, c.opts.WriteTimeout)
	defer cancel()
	return c.ws.Write(ctx, typ, m.Data) == nil
}

// flushAndClose writes what is already queued, within one write timeout in
// total, then performs the close handshake.
func (c *Conn) flushAndClose() {
	deadline := time.Now().Add(c.opts.WriteTimeout)
flush:
	for time.Now().Before(deadline) {
		select {
		case m := <-c.send:
			if !c.write(m) {
				break flush
			}
		default:
			break flush
		}
	}
	// Close waits (bounded by the library) for the peer's close frame.
	_ = c.ws.Close(c.closeCode, c.closeText)
	c.cancel()
}

func (c *Conn) abort() {
	c.cancel()
	_ = c.ws.CloseNow()
}

// pingLoop detects dead peers: a missing pong tears the connection down, which
// is what bounds how long a silent connection can hold resources.
func (c *Conn) pingLoop() {
	ticker := time.NewTicker(c.opts.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(c.ctx, c.opts.PongTimeout)
			err := c.ws.Ping(ctx)
			cancel()
			if err != nil {
				c.cancel()
				return
			}
		case <-c.closing:
			return
		case <-c.ctx.Done():
			return
		}
	}
}
