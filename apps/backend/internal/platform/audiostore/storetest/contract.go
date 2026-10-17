// Package storetest holds the behaviour every audiostore.Store must have, so
// the local-disk, in-memory and S3 adapters are held to one standard.
package storetest

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
)

// Run checks the contract against a fresh store.
func Run(t *testing.T, s audiostore.Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("round trip and write once", func(t *testing.T) {
		key := audiostore.NewKey()
		data := []byte("RIFF....audio bytes")
		require.NoError(t, s.Put(ctx, key, bytes.NewReader(data), int64(len(data))))

		rc, size, err := s.Open(ctx, key)
		require.NoError(t, err)
		got, _ := io.ReadAll(rc)
		rc.Close()
		assert.Equal(t, data, got)
		assert.Equal(t, int64(len(data)), size)

		// Audio is never silently replaced.
		assert.ErrorIs(t, s.Put(ctx, key, strings.NewReader("other"), 5), audiostore.ErrExists)
		rc, _, err = s.Open(ctx, key)
		require.NoError(t, err)
		again, _ := io.ReadAll(rc)
		rc.Close()
		assert.Equal(t, data, again, "the original is untouched by the refused overwrite")
	})

	t.Run("a missing object is not found", func(t *testing.T) {
		_, _, err := s.Open(ctx, audiostore.NewKey())
		assert.ErrorIs(t, err, audiostore.ErrNotFound)
	})

	t.Run("delete removes it, and deleting again is fine", func(t *testing.T) {
		key := audiostore.NewKey()
		require.NoError(t, s.Put(ctx, key, strings.NewReader("abc"), 3))
		require.NoError(t, s.Delete(ctx, key))
		_, _, err := s.Open(ctx, key)
		assert.ErrorIs(t, err, audiostore.ErrNotFound)
		assert.NoError(t, s.Delete(ctx, key))
		assert.NoError(t, s.Delete(ctx, audiostore.NewKey()), "deleting a key that never existed is not an error")
	})

	t.Run("keys that this package did not generate are refused", func(t *testing.T) {
		for _, key := range []string{"", "../x.wav", "a/b.wav", `..\x.wav`, "not-a-uuid.wav", audiostore.NewKey() + "/..", strings.ToUpper(audiostore.NewKey())} {
			assert.ErrorIs(t, s.Put(ctx, key, strings.NewReader("x"), 1), audiostore.ErrInvalidKey, key)
			_, _, err := s.Open(ctx, key)
			assert.ErrorIs(t, err, audiostore.ErrInvalidKey, key)
			assert.ErrorIs(t, s.Delete(ctx, key), audiostore.ErrInvalidKey, key)
		}
	})

	t.Run("a short write stores nothing", func(t *testing.T) {
		key := audiostore.NewKey()
		assert.Error(t, s.Put(ctx, key, strings.NewReader("short"), 100))
		_, _, err := s.Open(ctx, key)
		assert.ErrorIs(t, err, audiostore.ErrNotFound, "no partial object may be left under a real key")
	})

	t.Run("a cancelled context stops the write", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		key := audiostore.NewKey()
		assert.Error(t, s.Put(cctx, key, strings.NewReader("data"), 4))
		_, _, err := s.Open(ctx, key)
		assert.ErrorIs(t, err, audiostore.ErrNotFound)
	})

	t.Run("list reports what is stored, with size and a recent time", func(t *testing.T) {
		a, b := audiostore.NewKey(), audiostore.NewKey()
		before := time.Now().Add(-2 * time.Minute)
		require.NoError(t, s.Put(ctx, a, strings.NewReader("aaa"), 3))
		require.NoError(t, s.Put(ctx, b, strings.NewReader("bbbbb"), 5))

		listed, err := s.List(ctx)
		require.NoError(t, err)
		bykey := map[string]audiostore.ObjectInfo{}
		for _, o := range listed {
			bykey[o.Key] = o
		}
		require.Contains(t, bykey, a)
		require.Contains(t, bykey, b)
		assert.Equal(t, int64(3), bykey[a].Size)
		assert.Equal(t, int64(5), bykey[b].Size)
		assert.True(t, bykey[a].ModifiedAt.After(before), "modified time is when it was written, not zero")

		require.NoError(t, s.Delete(ctx, a))
		listed, err = s.List(ctx)
		require.NoError(t, err)
		for _, o := range listed {
			assert.NotEqual(t, a, o.Key, "a deleted object is no longer listed")
		}
	})

	t.Run("a larger object survives intact", func(t *testing.T) {
		key := audiostore.NewKey()
		data := bytes.Repeat([]byte{0x01, 0x7f, 0x80, 0xff}, 512*1024) // 2 MiB, like a long answer
		require.NoError(t, s.Put(ctx, key, bytes.NewReader(data), int64(len(data))))
		rc, size, err := s.Open(ctx, key)
		require.NoError(t, err)
		got, _ := io.ReadAll(rc)
		rc.Close()
		assert.Equal(t, int64(len(data)), size)
		assert.True(t, bytes.Equal(data, got))
	})
}
