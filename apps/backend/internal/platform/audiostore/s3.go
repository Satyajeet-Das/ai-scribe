package audiostore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config describes an S3-compatible bucket (MinIO, AWS S3, and others).
type S3Config struct {
	// Endpoint is host:port, without a scheme.
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	Region    string
	// UseSSL selects https. Anything outside a development machine should use it.
	UseSSL bool
	// CreateBucket creates the bucket when it does not exist. It is meant for
	// development: a production bucket should be provisioned deliberately, with
	// its access policy and encryption, and a missing one is then an error.
	CreateBucket bool
}

// S3 stores objects in an S3-compatible bucket. Objects are private; nothing
// here ever builds a public or pre-signed URL.
type S3 struct {
	client *minio.Client
	bucket string
}

// NewS3 connects and checks that the bucket is usable, so a misconfiguration
// is found at startup and not when the first student's audio is lost.
func NewS3(ctx context.Context, cfg S3Config) (*S3, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("audiostore: s3 endpoint, bucket, access key and secret key are required")
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("audiostore: s3 client: %w", err)
	}

	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("%w: checking bucket %q: %v", ErrUnavailable, cfg.Bucket, err)
	}
	if !exists {
		if !cfg.CreateBucket {
			return nil, fmt.Errorf("audiostore: bucket %q does not exist", cfg.Bucket)
		}
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			// Another instance may have created it a moment ago.
			if code := minio.ToErrorResponse(err).Code; code != "BucketAlreadyOwnedByYou" && code != "BucketAlreadyExists" {
				return nil, fmt.Errorf("%w: creating bucket %q: %v", ErrUnavailable, cfg.Bucket, err)
			}
		}
	}
	return &S3{client: client, bucket: cfg.Bucket}, nil
}

func (s *S3) Name() string { return "s3" }

// Put writes the object only if the key is unused, so audio is never silently
// replaced. The server enforces this (If-None-Match), not a racy check here.
func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	if !validKey(key) {
		return ErrInvalidKey
	}
	opts := minio.PutObjectOptions{ContentType: "audio/wav"}
	opts.SetMatchETagExcept("*")
	info, err := s.client.PutObject(ctx, s.bucket, key, r, size, opts)
	if err != nil {
		return s.mapError(err)
	}
	if info.Size != size {
		// The store accepted a different number of bytes than were promised.
		_ = s.client.RemoveObject(context.WithoutCancel(ctx), s.bucket, key, minio.RemoveObjectOptions{})
		return fmt.Errorf("audiostore: stored %d bytes, expected %d", info.Size, size)
	}
	return nil
}

// Open returns the object. Its size is read first, so a missing object is
// reported here and not on the first read.
func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	if !validKey(key) {
		return nil, 0, ErrInvalidKey
	}
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, 0, s.mapError(err)
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, s.mapError(err)
	}
	return obj, info.Size, nil
}

// Delete removes the object. Removing one that is already gone succeeds.
func (s *S3) Delete(ctx context.Context, key string) error {
	if !validKey(key) {
		return ErrInvalidKey
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		if err := s.mapError(err); !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}

// List returns every object in the bucket whose key this package generated. It
// reads the whole listing, which is fine for housekeeping at this scale.
func (s *S3) List(ctx context.Context) ([]ObjectInfo, error) {
	var out []ObjectInfo
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			return nil, s.mapError(obj.Err)
		}
		if !validKey(obj.Key) {
			continue
		}
		out = append(out, ObjectInfo{Key: obj.Key, Size: obj.Size, ModifiedAt: obj.LastModified})
	}
	return out, nil
}

// mapError turns the S3 library's errors into this package's.
func (s *S3) mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	resp := minio.ToErrorResponse(err)
	switch {
	case resp.Code == "NoSuchKey" || resp.StatusCode == http.StatusNotFound && resp.Code != "NoSuchBucket":
		return ErrNotFound
	case resp.Code == "PreconditionFailed" || resp.StatusCode == http.StatusPreconditionFailed:
		return ErrExists
	}
	// Everything else, including a missing bucket, bad credentials and network
	// trouble, means the store cannot be used right now.
	return fmt.Errorf("%w: %v", ErrUnavailable, err)
}
