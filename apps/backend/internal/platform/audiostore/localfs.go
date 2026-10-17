package audiostore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalFS stores objects as files under one directory. It is meant for
// development and single-node deployments; the directory must not be served by
// any web server.
type LocalFS struct {
	dir string
}

// NewLocalFS creates the directory if needed.
func NewLocalFS(dir string) (*LocalFS, error) {
	if dir == "" {
		return nil, errors.New("audiostore: directory is required")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("audiostore: creating %s: %w", abs, err)
	}
	return &LocalFS{dir: abs}, nil
}

func (s *LocalFS) Name() string { return "localfs" }

func (s *LocalFS) path(key string) (string, error) {
	if !validKey(key) {
		return "", ErrInvalidKey
	}
	return filepath.Join(s.dir, key), nil
}

// Put writes to a temporary file and links it into place, so a crash or a
// short read never leaves a partial object under a real key, and an existing
// key is never overwritten.
func (s *LocalFS) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	dst, err := s.path(key)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".put-*")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	n, err := io.Copy(tmp, io.LimitReader(&ctxReader{ctx: ctx, r: r}, size+1))
	if err == nil && n != size {
		err = fmt.Errorf("audiostore: expected %d bytes, got %d", size, n)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	// Link fails if dst exists, which is the write-once guarantee.
	if err := os.Link(tmpName, dst); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrExists
		}
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

func (s *LocalFS) Open(_ context.Context, key string) (io.ReadCloser, int64, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}

func (s *LocalFS) Delete(_ context.Context, key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// List returns the stored objects. Temporary files from writes in progress and
// any other file whose name is not a key are skipped.
func (s *LocalFS) List(ctx context.Context) ([]ObjectInfo, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	var out []ObjectInfo
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if e.IsDir() || !validKey(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue // removed since the directory was read
		}
		out = append(out, ObjectInfo{Key: e.Name(), Size: info.Size(), ModifiedAt: info.ModTime()})
	}
	return out, nil
}

// ctxReader stops a copy when the context ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
