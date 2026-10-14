package ws

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

var (
	ErrTooManyConnections      = errors.New("ws: connection limit reached")
	ErrTooManyOwnerConnections = errors.New("ws: per-owner connection limit reached")
	ErrShuttingDown            = errors.New("ws: server is shutting down")
)

// Registry accounts for live connections: it enforces a global and a per-owner
// limit, keeps at most one connection per key, and closes everything on
// shutdown. It is process-local.
type Registry struct {
	max         int
	maxPerOwner int

	mu       sync.Mutex
	closing  bool
	total    int
	perOwner map[string]int
	byKey    map[string]*Conn
	conns    map[*Conn]struct{}
	wg       sync.WaitGroup
}

func NewRegistry(max, maxPerOwner int) *Registry {
	return &Registry{
		max:         max,
		maxPerOwner: maxPerOwner,
		perOwner:    make(map[string]int),
		byKey:       make(map[string]*Conn),
		conns:       make(map[*Conn]struct{}),
	}
}

// Lease is a reserved connection slot. It must be released exactly once.
type Lease struct {
	r     *Registry
	owner string
	key   string
	conn  *Conn
	once  sync.Once
}

// Acquire reserves a slot before the upgrade, so that an over-limit client is
// refused with a plain HTTP error instead of an upgraded socket.
func (r *Registry) Acquire(owner string) (*Lease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closing {
		return nil, ErrShuttingDown
	}
	if r.max > 0 && r.total >= r.max {
		return nil, ErrTooManyConnections
	}
	if r.maxPerOwner > 0 && r.perOwner[owner] >= r.maxPerOwner {
		return nil, ErrTooManyOwnerConnections
	}
	r.total++
	r.perOwner[owner]++
	r.wg.Add(1)
	return &Lease{r: r, owner: owner}, nil
}

// Bind attaches the upgraded connection under key. If another connection holds
// the key it is returned so the caller can close it: the newest connection
// always wins, which is what makes reconnecting past a half-dead socket safe.
// ErrShuttingDown is returned if shutdown began after Acquire.
func (l *Lease) Bind(key string, c *Conn) (replaced *Conn, err error) {
	r := l.r
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closing {
		return nil, ErrShuttingDown
	}
	l.key, l.conn = key, c
	replaced = r.byKey[key]
	r.byKey[key] = c
	r.conns[c] = struct{}{}
	return replaced, nil
}

// Release frees the slot. Safe to call more than once.
func (l *Lease) Release() {
	l.once.Do(func() {
		r := l.r
		r.mu.Lock()
		r.total--
		if r.perOwner[l.owner]--; r.perOwner[l.owner] <= 0 {
			delete(r.perOwner, l.owner)
		}
		if l.conn != nil {
			delete(r.conns, l.conn)
			if r.byKey[l.key] == l.conn {
				delete(r.byKey, l.key)
			}
		}
		r.mu.Unlock()
		r.wg.Done()
	})
}

// Count returns the number of reserved slots.
func (r *Registry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.total
}

// Shutdown refuses new connections, asks every live one to close gracefully
// and waits for their handlers to return. If ctx expires first it returns
// ctx.Err(); the caller is expected to cancel the connections' base context.
func (r *Registry) Shutdown(ctx context.Context, code StatusCode, reason string) error {
	r.mu.Lock()
	r.closing = true
	conns := make([]*Conn, 0, len(r.conns))
	for c := range r.conns {
		conns = append(conns, c)
	}
	r.mu.Unlock()

	for _, c := range conns {
		c.Close(code, reason)
	}

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CheckOrigin reports whether the request's Origin is acceptable.
// A request without an Origin header does not come from a browser page and is
// allowed; cross-site WebSocket hijacking is a browser-only attack.
func CheckOrigin(r *http.Request, allowed []string) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	normalized := strings.ToLower(u.Scheme + "://" + u.Host)
	for _, a := range allowed {
		a = strings.ToLower(strings.TrimRight(strings.TrimSpace(a), "/"))
		if a == "*" || a == normalized {
			return true
		}
	}
	return false
}

// SplitOrigins flattens comma-separated origin lists, as they arrive from the
// environment, into individual origins.
func SplitOrigins(values []string) []string {
	var out []string
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
