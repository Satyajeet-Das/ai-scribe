package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func s3Recording() RecordingConfig {
	r := RecordingConfig{
		Enabled:  true,
		Provider: RecordingProviderS3,
		S3: RecordingS3Config{
			Endpoint: "minio:9000", Bucket: "answer-audio", AccessKey: "key", SecretKey: "secret", UseSSL: true,
		},
	}
	r.ApplyDefaults()
	return r
}

func TestRecordingConfig_IsOffByDefaultAndSafelyDefaulted(t *testing.T) {
	var r RecordingConfig
	r.ApplyDefaults()

	assert.False(t, r.Enabled, "keeping student audio must be an explicit decision")
	assert.Equal(t, RecordingProviderLocalFS, r.Provider)
	assert.Equal(t, "./data/audio", r.Dir)
	assert.Equal(t, 5*time.Second, r.SaveTimeout)
	assert.Zero(t, r.RetentionDays, "no retention date, and so no automatic deletion, unless asked for")
	assert.NoError(t, r.Validate())
	assert.NoError(t, r.ValidateForEnvironment("production"))
}

func TestRecordingConfig_S3NeedsEverythingItConnectsWith(t *testing.T) {
	require.NoError(t, (func() error { r := s3Recording(); return r.Validate() })())

	for name, mutate := range map[string]func(*RecordingConfig){
		"endpoint":   func(r *RecordingConfig) { r.S3.Endpoint = "" },
		"bucket":     func(r *RecordingConfig) { r.S3.Bucket = "" },
		"access key": func(r *RecordingConfig) { r.S3.AccessKey = "" },
		"secret key": func(r *RecordingConfig) { r.S3.SecretKey = "" },
	} {
		t.Run("missing "+name, func(t *testing.T) {
			r := s3Recording()
			mutate(&r)
			assert.Error(t, r.Validate())
		})
	}
}

func TestRecordingConfig_RejectsASchemeInTheEndpoint(t *testing.T) {
	r := s3Recording()
	r.S3.Endpoint = "https://minio:9000"
	err := r.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "without a scheme")
}

func TestRecordingConfig_UnknownProviderIsRejected(t *testing.T) {
	r := s3Recording()
	r.Provider = "dropbox"
	assert.Error(t, r.Validate())
}

func TestRecordingConfig_NothingIsValidatedWhileOff(t *testing.T) {
	r := RecordingConfig{Provider: "nonsense"}
	assert.NoError(t, r.Validate(), "a disabled feature must not stop the server from starting")
}

func TestRecordingConfig_ProductionMustUseTLSAndAProvisionedBucket(t *testing.T) {
	t.Run("encrypted and provisioned is fine", func(t *testing.T) {
		r := s3Recording()
		assert.NoError(t, r.ValidateForEnvironment("production"))
	})
	t.Run("plain http is refused", func(t *testing.T) {
		r := s3Recording()
		r.S3.UseSSL = false
		err := r.ValidateForEnvironment("production")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "use_ssl")
	})
	t.Run("a bucket the server creates itself is refused", func(t *testing.T) {
		r := s3Recording()
		r.S3.CreateBucket = true
		err := r.ValidateForEnvironment("production")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create_bucket")
	})
	t.Run("both are fine on a development machine", func(t *testing.T) {
		r := s3Recording()
		r.S3.UseSSL, r.S3.CreateBucket = false, true
		for _, env := range []string{"local", "development", "test"} {
			assert.NoError(t, r.ValidateForEnvironment(env), env)
		}
	})
	t.Run("playback of earlier audio is held to the same rule when a bucket is configured", func(t *testing.T) {
		r := s3Recording()
		r.Enabled = false
		r.S3.UseSSL = false
		assert.Error(t, r.ValidateForEnvironment("production"))
	})
	t.Run("an unconfigured, disabled feature is left alone", func(t *testing.T) {
		r := RecordingConfig{Provider: RecordingProviderS3}
		r.ApplyDefaults()
		assert.NoError(t, r.ValidateForEnvironment("production"))
	})
}

func TestRecordingCleanup_IsOffByDefaultAndSafelyDefaulted(t *testing.T) {
	var r RecordingConfig
	r.ApplyDefaults()

	assert.False(t, r.Cleanup.Enabled, "deleting students' data is never a surprise")
	assert.False(t, r.Cleanup.DeleteOrphans, "removing unreferenced files is a separate opt-in")
	assert.Equal(t, time.Hour, r.Cleanup.Interval)
	assert.Equal(t, 100, r.Cleanup.BatchSize)
	assert.Equal(t, time.Hour, r.Cleanup.OrphanGrace)
	assert.Equal(t, time.Hour, r.Cleanup.UnconfirmedGrace)
	assert.NoError(t, r.Validate())
}

func TestRecordingCleanup_RejectsSettingsThatCouldDeleteAudioBeingSaved(t *testing.T) {
	enabled := func() RecordingConfig {
		var r RecordingConfig
		r.Cleanup.Enabled = true
		r.ApplyDefaults()
		return r
	}
	require.NoError(t, (func() error { r := enabled(); return r.Validate() })())

	t.Run("a grace period shorter than a save can take", func(t *testing.T) {
		r := enabled()
		r.Cleanup.OrphanGrace = time.Minute
		err := r.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "orphan_grace")
	})
	t.Run("a grace period that does not allow for a slow save", func(t *testing.T) {
		r := enabled()
		r.SaveTimeout = 5 * time.Minute
		r.Cleanup.OrphanGrace = 12 * time.Minute // more than 10 minutes, less than 10 minutes + the save timeout
		assert.Error(t, r.Validate())
	})
	t.Run("an unconfirmed-dictation grace period shorter than a student needs to confirm", func(t *testing.T) {
		r := enabled()
		r.Cleanup.UnconfirmedGrace = time.Minute
		err := r.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unconfirmed_grace")
	})
	t.Run("an interval so short it would hammer the database", func(t *testing.T) {
		r := enabled()
		r.Cleanup.Interval = time.Second
		assert.Error(t, r.Validate())
	})
	t.Run("a batch size out of range", func(t *testing.T) {
		for _, n := range []int{-1, 5000} {
			r := enabled()
			r.Cleanup.BatchSize = n
			if n < 1 {
				r.Cleanup.BatchSize = n // ApplyDefaults would have repaired 0; negatives and large values must fail
			}
			assert.Error(t, r.Validate(), n)
		}
	})
}

func TestRecordingCleanup_IsCheckedEvenWhenNewRecordingIsOff(t *testing.T) {
	// Old audio must still expire after recording is switched off.
	var r RecordingConfig
	r.Cleanup.Enabled = true
	r.ApplyDefaults()
	r.Cleanup.Interval = time.Second
	assert.Error(t, r.Validate())
}

func TestRecordingCleanup_DisabledSettingsAreNotValidated(t *testing.T) {
	var r RecordingConfig
	r.ApplyDefaults()
	r.Cleanup.Interval = time.Second
	assert.NoError(t, r.Validate())
}
