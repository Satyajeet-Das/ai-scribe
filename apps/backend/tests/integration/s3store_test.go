package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore/storetest"
	"github.com/Satyajeet-Das/ai-scribe/internal/recording"
)

const (
	minioUser     = "testuser"
	minioPassword = "testpassword-123"
)

// startMinIO runs a throwaway MinIO server and returns its host:port.
func startMinIO(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "minio/minio:latest",
			Cmd:          []string{"server", "/data"},
			ExposedPorts: []string{"9000/tcp"},
			Env:          map[string]string{"MINIO_ROOT_USER": minioUser, "MINIO_ROOT_PASSWORD": minioPassword},
			WaitingFor:   wait.ForHTTP("/minio/health/ready").WithPort("9000/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err, "failed to start minio container")
	t.Cleanup(func() { _ = c.Terminate(ctx) })

	host, err := c.Host(ctx)
	require.NoError(t, err)
	port, err := c.MappedPort(ctx, "9000")
	require.NoError(t, err)
	return fmt.Sprintf("%s:%s", host, port.Port())
}

func s3Config(endpoint, bucket string) audiostore.S3Config {
	return audiostore.S3Config{
		Endpoint: endpoint, Bucket: bucket, AccessKey: minioUser, SecretKey: minioPassword,
		Region: "us-east-1", CreateBucket: true,
	}
}

func TestS3Store_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	endpoint := startMinIO(t)
	ctx := context.Background()

	store, err := audiostore.NewS3(ctx, s3Config(endpoint, "answer-audio"))
	require.NoError(t, err)

	t.Run("meets the same contract as every other store", func(t *testing.T) {
		storetest.Run(t, store)
	})

	t.Run("starting again against an existing bucket is fine", func(t *testing.T) {
		_, err := audiostore.NewS3(ctx, s3Config(endpoint, "answer-audio"))
		assert.NoError(t, err)
	})

	t.Run("a missing bucket is an error unless creating it was asked for", func(t *testing.T) {
		cfg := s3Config(endpoint, "never-created")
		cfg.CreateBucket = false
		_, err := audiostore.NewS3(ctx, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	t.Run("wrong credentials are reported as the store being unavailable, never as not found", func(t *testing.T) {
		cfg := s3Config(endpoint, "answer-audio")
		cfg.SecretKey = "not-the-secret"
		_, err := audiostore.NewS3(ctx, cfg)
		assert.ErrorIs(t, err, audiostore.ErrUnavailable)
	})

	t.Run("an unreachable server is unavailable, and quickly", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		start := time.Now()
		_, err := audiostore.NewS3(cctx, s3Config("127.0.0.1:1", "answer-audio"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, audiostore.ErrUnavailable) || errors.Is(err, context.DeadlineExceeded), "got %v", err)
		assert.Less(t, time.Since(start), 12*time.Second)
	})

	t.Run("stored audio cannot be read without credentials", func(t *testing.T) {
		key := audiostore.NewKey()
		require.NoError(t, store.Put(ctx, key, strings.NewReader("private audio"), 13))

		resp, err := http.Get(fmt.Sprintf("http://%s/answer-audio/%s", endpoint, key))
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "the bucket must not be public")
	})
}

// failingRepo accepts nothing, like a database that is down.
type failingRepo struct{}

func (failingRepo) Insert(context.Context, *recording.Recording, *recording.Transcription) (*recording.Recording, uuid.UUID, bool, error) {
	return nil, uuid.Nil, false, errors.New("database down")
}

func TestRecordingOverS3_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	endpoint := startMinIO(t)
	ctx := context.Background()
	store, err := audiostore.NewS3(ctx, s3Config(endpoint, "answer-audio"))
	require.NoError(t, err)

	input := recording.SaveInput{
		SessionID: uuid.New(), QuestionID: uuid.New(), StudentID: uuid.New(), StreamID: uuid.New(),
		SampleRate: 16000, ChunkCount: 2, PCM: bytes.Repeat([]byte{5, 0}, 16000), Transcript: "hello",
	}

	t.Run("audio whose metadata could not be written is removed from the bucket", func(t *testing.T) {
		svc := recording.NewService(failingRepo{}, store, recording.Options{}, nil)
		_, err := svc.Save(ctx, input)
		require.ErrorIs(t, err, recording.ErrStorage)

		// Nothing in the bucket may be left unaccounted for. (Other tests use
		// their own buckets, so this one holds only what this test stored.)
		listed, err := store.List(ctx)
		require.NoError(t, err)
		assert.Empty(t, listed)
	})

	t.Run("audio that was stored and recorded stays in the bucket", func(t *testing.T) {
		repo := &memoryRepo{}
		svc := recording.NewService(repo, store, recording.Options{}, nil)
		saved, err := svc.Save(ctx, input)
		require.NoError(t, err)
		assert.False(t, saved.Duplicate)

		listed, err := store.List(ctx)
		require.NoError(t, err)
		require.Len(t, listed, 1)
		assert.Equal(t, listed[0].Key, repo.rec.StorageKey)

		// A retry of the same stream stores nothing new.
		again, err := svc.Save(ctx, input)
		require.NoError(t, err)
		assert.True(t, again.Duplicate)
		listed, _ = store.List(ctx)
		assert.Len(t, listed, 1)
	})
}

// memoryRepo remembers one recording and treats a repeat of its stream as a duplicate.
type memoryRepo struct {
	rec *recording.Recording
	tr  *recording.Transcription
}

func (m *memoryRepo) Insert(_ context.Context, rec *recording.Recording, tr *recording.Transcription) (*recording.Recording, uuid.UUID, bool, error) {
	if m.rec != nil && m.rec.StreamID == rec.StreamID {
		return m.rec, m.tr.ID, false, nil
	}
	m.rec, m.tr = rec, tr
	return nil, uuid.Nil, true, nil
}
