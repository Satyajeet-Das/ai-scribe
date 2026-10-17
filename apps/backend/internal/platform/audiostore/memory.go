package audiostore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// Memory is an in-process Store for tests.
type Memory struct {
	mu       sync.Mutex
	objects  map[string][]byte
	modified map[string]time.Time
	// PutErr, when set, is returned by Put. It lets tests simulate an outage.
	PutErr error
}

func NewMemory() *Memory {
	return &Memory{objects: map[string][]byte{}, modified: map[string]time.Time{}}
}

func (m *Memory) Name() string { return "memory" }

func (m *Memory) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	if !validKey(key) {
		return ErrInvalidKey
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.PutErr != nil {
		return m.PutErr
	}
	data, err := io.ReadAll(io.LimitReader(r, size+1))
	if err != nil {
		return err
	}
	if int64(len(data)) != size {
		return fmt.Errorf("audiostore: expected %d bytes, got %d", size, len(data))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.objects[key]; ok {
		return ErrExists
	}
	m.objects[key] = data
	m.modified[key] = time.Now()
	return nil
}

func (m *Memory) Open(_ context.Context, key string) (io.ReadCloser, int64, error) {
	if !validKey(key) {
		return nil, 0, ErrInvalidKey
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.objects[key]
	if !ok {
		return nil, 0, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

func (m *Memory) Delete(_ context.Context, key string) error {
	if !validKey(key) {
		return ErrInvalidKey
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	delete(m.modified, key)
	return nil
}

// List returns the stored objects.
func (m *Memory) List(ctx context.Context) ([]ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ObjectInfo, 0, len(m.objects))
	for k, v := range m.objects {
		out = append(out, ObjectInfo{Key: k, Size: int64(len(v)), ModifiedAt: m.modified[k]})
	}
	return out, nil
}

// SetModified changes when an object counts as last written, so tests can make
// an object look old without waiting.
func (m *Memory) SetModified(key string, t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.objects[key]; ok {
		m.modified[key] = t
	}
}

// Keys lists stored keys. Tests use it to assert nothing is left behind.
func (m *Memory) Keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.objects))
	for k := range m.objects {
		keys = append(keys, k)
	}
	return keys
}
