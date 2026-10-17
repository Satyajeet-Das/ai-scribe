package config

import (
	"fmt"
	"strings"
	"time"
)

const (
	RecordingProviderLocalFS = "localfs"
	// RecordingProviderS3 is any S3-compatible service: MinIO, AWS S3, and so on.
	RecordingProviderS3 = "s3"
)

// RecordingConfig controls whether a student's dictated answers are retained
// as audio. It is off unless explicitly enabled: keeping audio of students is
// an institutional decision (notice, consent or authorization, retention), and
// the system must not start doing it by default.
type RecordingConfig struct {
	Enabled bool `koanf:"enabled"`
	// Provider selects the audio store: "localfs" keeps files under Dir, "s3"
	// uses an S3-compatible bucket.
	Provider string `koanf:"provider"`
	// Dir is the directory for the localfs provider. It must not be served by a
	// web server.
	Dir string `koanf:"dir"`
	// RetentionDays sets how long new recordings are kept. Zero means no
	// retention date is recorded, and nothing is deleted automatically.
	RetentionDays int `koanf:"retention_days"`
	// SaveTimeout bounds storing one recording, so a slow store cannot hold up
	// the student's answer.
	SaveTimeout time.Duration `koanf:"save_timeout"`

	S3 RecordingS3Config `koanf:"s3"`

	Cleanup RecordingCleanupConfig `koanf:"cleanup"`
}

// RecordingCleanupConfig controls the background job that deletes audio whose
// retention period has ended and tidies the store. Deleting students' data is
// never a surprise, so it is off until switched on.
type RecordingCleanupConfig struct {
	// Enabled turns the job on. It deletes audio only for recordings that have a
	// retention date (see RetentionDays) which has passed; it works even when
	// recording new audio is off, so old audio still expires.
	Enabled bool `koanf:"enabled"`
	// Interval is the time between runs.
	Interval time.Duration `koanf:"interval"`
	// BatchSize bounds how many recordings are purged per round trip.
	BatchSize int `koanf:"batch_size"`
	// UnconfirmedGrace is how long a dictation the student never confirmed is
	// kept before its audio is deleted.
	UnconfirmedGrace time.Duration `koanf:"unconfirmed_grace"`
	// OrphanGrace is how old stored audio must be, with no recording referring
	// to it, before it counts as an orphan.
	OrphanGrace time.Duration `koanf:"orphan_grace"`
	// DeleteOrphans deletes orphaned audio. When false they are only logged.
	// Use it only with a bucket (or directory) dedicated to this application.
	DeleteOrphans bool `koanf:"delete_orphans"`
}

// RecordingS3Config describes the bucket for the "s3" provider.
type RecordingS3Config struct {
	// Endpoint is host:port without a scheme, e.g. "minio:9000".
	Endpoint string `koanf:"endpoint"`
	Bucket   string `koanf:"bucket"`
	// AccessKey and SecretKey are credentials. Give them through the environment
	// or a secret store; they are never logged.
	AccessKey string `koanf:"access_key"`
	SecretKey string `koanf:"secret_key"`
	Region    string `koanf:"region"`
	// UseSSL selects https. Required in production.
	UseSSL bool `koanf:"use_ssl"`
	// CreateBucket creates a missing bucket at startup. For development only:
	// production buckets are provisioned deliberately, with their access policy
	// and encryption.
	CreateBucket bool `koanf:"create_bucket"`
}

// Configured reports whether enough is set to try connecting.
func (s RecordingS3Config) Configured() bool {
	return s.Endpoint != "" && s.Bucket != ""
}

// ApplyDefaults fills every unset field.
func (r *RecordingConfig) ApplyDefaults() {
	setStr(&r.Provider, RecordingProviderLocalFS)
	r.Provider = strings.ToLower(strings.TrimSpace(r.Provider))
	setStr(&r.Dir, "./data/audio")
	setDur(&r.SaveTimeout, 5*time.Second)
	setDur(&r.Cleanup.Interval, time.Hour)
	setInt(&r.Cleanup.BatchSize, 100)
	setDur(&r.Cleanup.OrphanGrace, time.Hour)
	setDur(&r.Cleanup.UnconfirmedGrace, time.Hour)
}

// Validate rejects a configuration that would start recording half-configured.
func (r *RecordingConfig) Validate() error {
	if err := r.Cleanup.validate(r.SaveTimeout); err != nil {
		return err
	}
	if !r.Enabled {
		return nil
	}
	switch r.Provider {
	case RecordingProviderLocalFS:
		if r.Dir == "" {
			return fmt.Errorf("recording.dir is required when recording is enabled")
		}
	case RecordingProviderS3:
		s := r.S3
		if s.Endpoint == "" || s.Bucket == "" || s.AccessKey == "" || s.SecretKey == "" {
			return fmt.Errorf("recording.s3.endpoint, bucket, access_key and secret_key are required when recording.provider is %q", r.Provider)
		}
		if strings.Contains(s.Endpoint, "://") {
			return fmt.Errorf("recording.s3.endpoint must be host:port without a scheme, got %q", s.Endpoint)
		}
	default:
		return fmt.Errorf("invalid recording.provider %q (must be one of: localfs, s3)", r.Provider)
	}
	if r.RetentionDays < 0 {
		return fmt.Errorf("recording.retention_days must not be negative")
	}
	return nil
}

// ValidateForEnvironment adds the checks that depend on where the server runs.
// Student audio must not travel unencrypted or into a bucket the server made
// up on its own in production.
func (r *RecordingConfig) ValidateForEnvironment(env string) error {
	if !r.Enabled && !r.S3.Configured() {
		return nil
	}
	if env != "production" || r.Provider != RecordingProviderS3 {
		return nil
	}
	if !r.S3.UseSSL {
		return fmt.Errorf("recording.s3.use_ssl must be true in production: student audio must not be sent unencrypted")
	}
	if r.S3.CreateBucket {
		return fmt.Errorf("recording.s3.create_bucket must be false in production: provision the bucket, with its access policy and encryption, deliberately")
	}
	return nil
}

func (c RecordingCleanupConfig) validate(saveTimeout time.Duration) error {
	if !c.Enabled {
		return nil
	}
	if c.Interval < time.Minute {
		return fmt.Errorf("recording.cleanup.interval must be at least 1m")
	}
	if c.BatchSize < 1 || c.BatchSize > 1000 {
		return fmt.Errorf("recording.cleanup.batch_size must be between 1 and 1000")
	}
	// A student has seconds to confirm a dictation; this leaves room for retries
	// and a slow network, so audio supporting a confirmation is never deleted.
	if c.UnconfirmedGrace < 10*time.Minute {
		return fmt.Errorf("recording.cleanup.unconfirmed_grace must be at least 10m")
	}
	// Audio is stored a moment before its database row. Shorter than this, audio
	// being saved right now could be taken for an orphan and deleted.
	if min := 10*time.Minute + saveTimeout; c.OrphanGrace < min {
		return fmt.Errorf("recording.cleanup.orphan_grace must be at least %s", min)
	}
	return nil
}
