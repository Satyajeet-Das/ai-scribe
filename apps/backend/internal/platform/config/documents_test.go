package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultedDocuments(mutate func(*DocumentsConfig)) DocumentsConfig {
	d := DocumentsConfig{}
	if mutate != nil {
		mutate(&d)
	}
	d.ApplyDefaults()
	return d
}

func TestDocuments_OffUnlessEnabledAndNeverSendsAnythingByDefault(t *testing.T) {
	d := defaultedDocuments(nil)
	assert.False(t, d.Enabled)
	assert.Equal(t, DocumentProviderDisabled, d.Extraction.Provider)
	assert.False(t, d.Active())
	assert.NoError(t, d.Validate())

	// Enabled but with no provider: uploads are refused, nothing leaves the server.
	d = defaultedDocuments(func(d *DocumentsConfig) { d.Enabled = true })
	assert.NoError(t, d.Validate())
	assert.False(t, d.Active(), "enabling documents does not by itself start sending exam content to a provider")
}

func TestDocuments_Defaults(t *testing.T) {
	d := defaultedDocuments(nil)
	assert.Equal(t, int64(20<<20), d.MaxBytes)
	assert.Equal(t, 60, d.MaxPages)
	assert.Equal(t, DocumentStorageLocalFS, d.Storage.Provider)
	assert.Equal(t, "./data/documents", d.Storage.Dir)
	assert.Equal(t, 2, d.Job.MaxConcurrent)
	assert.Equal(t, 3, d.Job.MaxRetry)
	assert.Equal(t, 2*time.Minute, d.Job.Lease)
	assert.Empty(t, d.Extraction.TextModel, "no provider, no model")
	assert.Empty(t, d.Extraction.APIKey, "no credentials are ever defaulted")
}

func TestDocuments_GroqDefaultsAndCredentials(t *testing.T) {
	d := defaultedDocuments(func(d *DocumentsConfig) {
		d.Enabled = true
		d.Extraction.Provider = " Groq "
		d.Extraction.APIKey = "gsk_x"
	})
	require.NoError(t, d.Validate())
	assert.True(t, d.Active())
	assert.Equal(t, DocumentProviderGroq, d.Extraction.Provider)
	assert.Equal(t, DefaultGroqBaseURL, d.Extraction.BaseURL)
	assert.Equal(t, DefaultDocumentTextModel, d.Extraction.TextModel)
	assert.Equal(t, DefaultDocumentVisionModel, d.Extraction.VisionModel)

	d = defaultedDocuments(func(d *DocumentsConfig) { d.Enabled, d.Extraction.Provider = true, "groq" })
	assert.ErrorContains(t, d.Validate(), "api_key is required", "a provider is never started without a configured key")
}

func TestDocuments_MistakesAreCaughtAtStartup(t *testing.T) {
	groq := func(f func(*DocumentsConfig)) DocumentsConfig {
		return defaultedDocuments(func(d *DocumentsConfig) {
			d.Enabled, d.Extraction.Provider, d.Extraction.APIKey = true, "groq", "gsk_x"
			if f != nil {
				f(d)
			}
		})
	}
	cases := map[string]struct {
		cfg  DocumentsConfig
		want string
	}{
		"unknown extraction provider": {defaultedDocuments(func(d *DocumentsConfig) { d.Enabled, d.Extraction.Provider = true, "openai" }), "invalid documents.extraction.provider"},
		"unknown storage provider":    {defaultedDocuments(func(d *DocumentsConfig) { d.Enabled, d.Storage.Provider = true, "ftp" }), "invalid documents.storage.provider"},
		"s3 without credentials": {defaultedDocuments(func(d *DocumentsConfig) {
			d.Enabled, d.Storage.Provider = true, "s3"
			d.Storage.S3.Endpoint, d.Storage.S3.Bucket = "minio:9000", "docs"
		}), "access_key and secret_key are required"},
		"s3 endpoint with a scheme": {defaultedDocuments(func(d *DocumentsConfig) {
			d.Enabled, d.Storage.Provider = true, "s3"
			d.Storage.S3 = RecordingS3Config{Endpoint: "https://minio:9000", Bucket: "b", AccessKey: "a", SecretKey: "s"}
		}), "without a scheme"},
		"a claude model left over from the voice setup": {groq(func(d *DocumentsConfig) { d.Extraction.TextModel = "claude-opus-5-5" }), "not a Groq model"},
		"the ollama address left over":                  {groq(func(d *DocumentsConfig) { d.Extraction.BaseURL = "http://localhost:11434/" }), "Ollama address"},
		"too many images per request":                   {groq(func(d *DocumentsConfig) { d.Extraction.MaxImagesPerRequest = 9 }), "max_images_per_request"},
		"size limit absurdly large":                     {groq(func(d *DocumentsConfig) { d.MaxBytes = 5 << 30 }), "max_bytes"},
		"page limit absurdly large":                     {groq(func(d *DocumentsConfig) { d.MaxPages = 5000 }), "max_pages"},
		"lease too short to renew":                      {groq(func(d *DocumentsConfig) { d.Job.Lease = 5 * time.Second }), "lease"},
		"job timeout too short":                         {groq(func(d *DocumentsConfig) { d.Job.Timeout = 10 * time.Second }), "timeout"},
		"too many parsers at once":                      {groq(func(d *DocumentsConfig) { d.Job.MaxConcurrent = 64 }), "max_concurrent"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := tc.cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestDocuments_DisabledConfigIsNotValidated(t *testing.T) {
	d := defaultedDocuments(func(d *DocumentsConfig) { d.Extraction.Provider = "nonsense" })
	assert.NoError(t, d.Validate(), "a broken block that is switched off does not stop the server starting")
}

func TestDocuments_ProductionRequiresEncryptedProvisionedStorage(t *testing.T) {
	s3 := func(ssl, create bool) DocumentsConfig {
		return defaultedDocuments(func(d *DocumentsConfig) {
			d.Enabled, d.Storage.Provider = true, "s3"
			d.Storage.S3 = RecordingS3Config{Endpoint: "s3:9000", Bucket: "docs", AccessKey: "a", SecretKey: "s", UseSSL: ssl, CreateBucket: create}
		})
	}
	good := s3(true, false)
	assert.NoError(t, good.ValidateForEnvironment("production"))

	noSSL := s3(false, false)
	assert.ErrorContains(t, noSSL.ValidateForEnvironment("production"), "use_ssl")
	made := s3(true, true)
	assert.ErrorContains(t, made.ValidateForEnvironment("production"), "create_bucket")

	dev := s3(false, true)
	assert.NoError(t, dev.ValidateForEnvironment("development"), "development may use a local unencrypted bucket")
	local := defaultedDocuments(func(d *DocumentsConfig) { d.Enabled = true })
	assert.NoError(t, local.ValidateForEnvironment("production"))
}
