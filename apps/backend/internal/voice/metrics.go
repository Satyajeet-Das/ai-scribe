package voice

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
)

// Metric names. Counters carry their label as a dotted suffix, e.g.
// "connections.closed.replaced".
const (
	MetricConnectionsActive   = "connections.active"
	MetricConnectionsOpened   = "connections.opened"
	MetricConnectionsClosed   = "connections.closed"
	MetricConnectionsRejected = "connections.rejected"
	MetricConnectionDuration  = "connections.duration"
	MetricReconnects          = "connections.reconnects"
	MetricStreamsStarted      = "streams.started"
	MetricStreamsEnded        = "streams.ended"
	MetricStreamDuration      = "streams.duration"
	MetricStreamBytes         = "streams.bytes"
	MetricCancellations       = "cancellations"
	MetricFailures            = "failures"
	MetricProviderLatency     = "provider.latency"
	MetricProviderErrors      = "provider.errors"
	MetricIntents             = "intents"
	MetricActions             = "actions"
	MetricProtocolErrors      = "protocol.errors"
)

// Metrics records voice pipeline metrics in process and, when New Relic is
// configured, forwards them as custom metrics under "Custom/Voice/".
// The zero value is not usable; use NewMetrics.
type Metrics struct {
	app *newrelic.Application

	mu       sync.RWMutex
	counters map[string]*atomic.Int64
	timings  map[string]*timing
}

type timing struct {
	count atomic.Int64
	nanos atomic.Int64
}

// NewMetrics returns a recorder; app may be nil.
func NewMetrics(app *newrelic.Application) *Metrics {
	return &Metrics{
		app:      app,
		counters: make(map[string]*atomic.Int64),
		timings:  make(map[string]*timing),
	}
}

func metricName(name string, labels []string) string {
	if len(labels) == 0 {
		return name
	}
	return name + "." + strings.Join(labels, ".")
}

func (m *Metrics) counter(name string) *atomic.Int64 {
	m.mu.RLock()
	c := m.counters[name]
	m.mu.RUnlock()
	if c != nil {
		return c
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if c = m.counters[name]; c == nil {
		c = new(atomic.Int64)
		m.counters[name] = c
	}
	return c
}

// Add adjusts a counter or gauge by delta.
func (m *Metrics) Add(name string, delta int64, labels ...string) {
	full := metricName(name, labels)
	v := m.counter(full).Add(delta)
	if m.app != nil {
		m.app.RecordCustomMetric("Custom/Voice/"+full, float64(v))
	}
}

// Inc increments a counter.
func (m *Metrics) Inc(name string, labels ...string) { m.Add(name, 1, labels...) }

// Observe records one duration sample.
func (m *Metrics) Observe(name string, d time.Duration, labels ...string) {
	full := metricName(name, labels)
	m.mu.RLock()
	t := m.timings[full]
	m.mu.RUnlock()
	if t == nil {
		m.mu.Lock()
		if t = m.timings[full]; t == nil {
			t = new(timing)
			m.timings[full] = t
		}
		m.mu.Unlock()
	}
	t.count.Add(1)
	t.nanos.Add(int64(d))
	if m.app != nil {
		m.app.RecordCustomMetric("Custom/Voice/"+full+".ms", float64(d.Milliseconds()))
	}
}

// Value returns the current value of a counter or gauge.
func (m *Metrics) Value(name string, labels ...string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if c := m.counters[metricName(name, labels)]; c != nil {
		return c.Load()
	}
	return 0
}

// Samples returns how many durations were observed under a name.
func (m *Metrics) Samples(name string, labels ...string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t := m.timings[metricName(name, labels)]; t != nil {
		return t.count.Load()
	}
	return 0
}

// Snapshot returns every metric, with timings reported as a sample count and
// a mean in milliseconds. Keys are sorted for stable output.
func (m *Metrics) Snapshot() map[string]float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]float64, len(m.counters)+2*len(m.timings))
	for k, c := range m.counters {
		out[k] = float64(c.Load())
	}
	for k, t := range m.timings {
		n := t.count.Load()
		out[k+".count"] = float64(n)
		if n > 0 {
			out[k+".mean_ms"] = float64(t.nanos.Load()) / float64(n) / float64(time.Millisecond)
		}
	}
	return out
}

// SortedKeys returns a snapshot's keys in order.
func SortedKeys(snapshot map[string]float64) []string {
	keys := make([]string, 0, len(snapshot))
	for k := range snapshot {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
