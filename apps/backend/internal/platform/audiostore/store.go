// Package audiostore is the boundary between the application and wherever
// retained audio lives. The application depends only on Store, so the backing
// service (local disk now, an S3-compatible bucket later) can change without
// touching the recording or voice code.
package audiostore

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrNotFound means no object exists under the key.
	ErrNotFound = errors.New("audiostore: object not found")
	// ErrInvalidKey means the key could not be a key this package generated.
	ErrInvalidKey = errors.New("audiostore: invalid key")
	// ErrUnavailable means the backing store could not be reached.
	ErrUnavailable = errors.New("audiostore: store unavailable")
	// ErrExists means Put was given a key that is already in use.
	ErrExists = errors.New("audiostore: key already exists")
)

// Store keeps immutable blobs addressed by opaque keys.
type Store interface {
	// Name identifies the adapter in logs and metrics.
	Name() string
	// Put stores size bytes read from r under key. A key is written once:
	// putting an existing key fails, so audio is never silently replaced.
	Put(ctx context.Context, key string, r io.Reader, size int64) error
	// Open returns the object and its size. The caller closes it.
	Open(ctx context.Context, key string) (io.ReadCloser, int64, error)
	// Delete removes the object. Deleting a missing key is not an error.
	Delete(ctx context.Context, key string) error
	// List returns every object whose key this package generated. Anything
	// else in the same place (temporary files, other applications' data) is
	// not listed, so housekeeping can never be pointed at it.
	List(ctx context.Context) ([]ObjectInfo, error)
}

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Key        string
	Size       int64
	ModifiedAt time.Time
}

const keySuffix = ".wav"

// NewKey returns a fresh storage key: a random UUID and a fixed suffix. It
// deliberately carries nothing about the student, exam or question, so a key
// that leaks (in a log, a URL, a bucket listing) reveals nothing.
func NewKey() string { return uuid.NewString() + keySuffix }

// validKey accepts only the shape NewKey produces, which keeps every adapter
// safe from path traversal and keeps keys free of personal data.
func validKey(key string) bool {
	if len(key) != 36+len(keySuffix) || key[36:] != keySuffix {
		return false
	}
	for i := 0; i < 36; i++ {
		c := key[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		case c == '-' && (i == 8 || i == 13 || i == 18 || i == 23):
		default:
			return false
		}
	}
	return true
}
