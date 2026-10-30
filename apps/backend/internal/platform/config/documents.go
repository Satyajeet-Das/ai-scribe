package config

import (
	"fmt"
	"strings"
	"time"
)

const (
	// DocumentProviderDisabled keeps uploads off: nothing is sent anywhere.
	DocumentProviderDisabled = "disabled"
	// DocumentProviderGroq structures documents with models hosted on Groq.
	DocumentProviderGroq = "groq"

	DocumentStorageLocalFS = "localfs"
	DocumentStorageS3      = "s3"
)

// Defaults for the groq extraction provider. Groq retires models from time to
// time (a retired one answers 404 model_not_found), so these are the models it
// serves at the time of writing; change them with
// SCRIBE_DOCUMENTS.EXTRACTION.TEXT_MODEL and .VISION_MODEL.
const (
	DefaultDocumentTextModel   = "openai/gpt-oss-120b"
	DefaultDocumentVisionModel = "qwen/qwen3.8-27b"
)

// DocumentsConfig controls exam document ingestion: a teacher uploads a PDF and
// draft questions are extracted for review. It is off unless explicitly
// enabled, and extraction needs a provider on top of that: while the provider
// is "disabled" uploads are refused and no exam content leaves the server.
type DocumentsConfig struct {
	Enabled bool `koanf:"enabled"`

	// MaxBytes and MaxPages bound one upload.
	MaxBytes int64 `koanf:"max_bytes"`
	MaxPages int   `koanf:"max_pages"`
	// ParseTimeout bounds one call into the PDF parser.
	ParseTimeout time.Duration `koanf:"parse_timeout"`
	// UploadsPerMinute limits uploads per user.
	UploadsPerMinute int `koanf:"uploads_per_minute"`

	Storage    DocumentsStorageConfig    `koanf:"storage"`
	Extraction DocumentsExtractionConfig `koanf:"extraction"`
	Job        DocumentsJobConfig        `koanf:"job"`
}

// DocumentsStorageConfig says where the uploaded PDFs live. They are private
// objects, served only through an endpoint that checks ownership.
type DocumentsStorageConfig struct {
	// Provider is "localfs" (files under Dir) or "s3" (an S3-compatible bucket).
	Provider string `koanf:"provider"`
	// Dir is the directory for localfs. It must not be served by a web server,
	// and it should not be the answer-audio directory.
	Dir string            `koanf:"dir"`
	S3  RecordingS3Config `koanf:"s3"`
}

// DocumentsExtractionConfig selects the model that structures the text.
type DocumentsExtractionConfig struct {
	// Provider is "disabled" or "groq".
	Provider string `koanf:"provider"`
	// APIKey is sent as a bearer token. Give it through the environment or a
	// secret store; it is never logged.
	APIKey  string `koanf:"api_key"`
	BaseURL string `koanf:"base_url"`
	// TextModel structures pages that have a text layer.
	TextModel string `koanf:"text_model"`
	// VisionModel reads pages that are pictures. Empty means such pages are not
	// read and the teacher is told so.
	VisionModel string `koanf:"vision_model"`
	// RequestTimeout bounds one call to the provider.
	RequestTimeout time.Duration `koanf:"request_timeout"`
	// MaxCharsPerRequest is how much page text goes in one call.
	MaxCharsPerRequest int `koanf:"max_chars_per_request"`
	// MaxImagesPerRequest is how many pictures go in one call (Groq allows 3).
	MaxImagesPerRequest int `koanf:"max_images_per_request"`
}

// DocumentsJobConfig controls the background extraction job.
type DocumentsJobConfig struct {
	// Timeout bounds one run of the job.
	Timeout time.Duration `koanf:"timeout"`
	// MaxRetry is how many times a transient failure is retried.
	MaxRetry int `koanf:"max_retry"`
	// Lease is how long a worker owns a document before another may take it
	// over; a working job renews it.
	Lease time.Duration `koanf:"lease"`
	// MaxConcurrent is how many documents one server parses at once.
	MaxConcurrent int `koanf:"max_concurrent"`
}

// Active reports whether uploads can be accepted.
func (d DocumentsConfig) Active() bool {
	return d.Enabled && d.Extraction.Provider != DocumentProviderDisabled
}

// ApplyDefaults fills every unset field.
func (d *DocumentsConfig) ApplyDefaults() {
	setInt64(&d.MaxBytes, 20<<20)
	setInt(&d.MaxPages, 60)
	setDur(&d.ParseTimeout, 30*time.Second)
	setInt(&d.UploadsPerMinute, 10)

	setStr(&d.Storage.Provider, DocumentStorageLocalFS)
	d.Storage.Provider = strings.ToLower(strings.TrimSpace(d.Storage.Provider))
	setStr(&d.Storage.Dir, "./data/documents")

	setStr(&d.Extraction.Provider, DocumentProviderDisabled)
	d.Extraction.Provider = strings.ToLower(strings.TrimSpace(d.Extraction.Provider))
	if d.Extraction.Provider == DocumentProviderGroq {
		setStr(&d.Extraction.BaseURL, DefaultGroqBaseURL)
		setStr(&d.Extraction.TextModel, DefaultDocumentTextModel)
		setStr(&d.Extraction.VisionModel, DefaultDocumentVisionModel)
	}
	setDur(&d.Extraction.RequestTimeout, 2*time.Minute)
	setInt(&d.Extraction.MaxCharsPerRequest, 12000)
	setInt(&d.Extraction.MaxImagesPerRequest, 3)

	setDur(&d.Job.Timeout, 15*time.Minute)
	if d.Job.MaxRetry <= 0 {
		d.Job.MaxRetry = 3
	}
	setDur(&d.Job.Lease, 2*time.Minute)
	setInt(&d.Job.MaxConcurrent, 2)
}

// Validate rejects a configuration that would start half-configured.
func (d *DocumentsConfig) Validate() error {
	if !d.Enabled {
		return nil
	}
	switch d.Storage.Provider {
	case DocumentStorageLocalFS:
		if d.Storage.Dir == "" {
			return fmt.Errorf("documents.storage.dir is required when documents.storage.provider is %q", d.Storage.Provider)
		}
	case DocumentStorageS3:
		s := d.Storage.S3
		if s.Endpoint == "" || s.Bucket == "" || s.AccessKey == "" || s.SecretKey == "" {
			return fmt.Errorf("documents.storage.s3.endpoint, bucket, access_key and secret_key are required when documents.storage.provider is %q", d.Storage.Provider)
		}
		if strings.Contains(s.Endpoint, "://") {
			return fmt.Errorf("documents.storage.s3.endpoint must be host:port without a scheme, got %q", s.Endpoint)
		}
	default:
		return fmt.Errorf("invalid documents.storage.provider %q (must be one of: localfs, s3)", d.Storage.Provider)
	}

	switch d.Extraction.Provider {
	case DocumentProviderDisabled:
	case DocumentProviderGroq:
		e := d.Extraction
		if e.APIKey == "" {
			return fmt.Errorf("documents.extraction.api_key is required when documents.extraction.provider is %q", e.Provider)
		}
		if e.TextModel == "" {
			return fmt.Errorf("documents.extraction.text_model is required when documents.extraction.provider is %q", e.Provider)
		}
		// The same mistakes the voice intent config guards against: another
		// provider's model or address left in place would fail with a status code
		// that does not say why.
		for _, m := range []string{e.TextModel, e.VisionModel} {
			if strings.HasPrefix(strings.ToLower(m), "claude") {
				return fmt.Errorf("documents.extraction model %q is not a Groq model: unset it to use the default", m)
			}
		}
		if strings.TrimRight(e.BaseURL, "/") == "http://localhost:11434" {
			return fmt.Errorf("documents.extraction.base_url %q is the Ollama address: unset it to use Groq at %s", e.BaseURL, DefaultGroqBaseURL)
		}
		if e.MaxImagesPerRequest < 1 || e.MaxImagesPerRequest > 5 {
			return fmt.Errorf("documents.extraction.max_images_per_request must be between 1 and 5")
		}
	default:
		return fmt.Errorf("invalid documents.extraction.provider %q (must be one of: disabled, groq)", d.Extraction.Provider)
	}

	if d.MaxBytes < 1024 || d.MaxBytes > 100<<20 {
		return fmt.Errorf("documents.max_bytes must be between 1 KiB and 100 MiB")
	}
	if d.MaxPages < 1 || d.MaxPages > 500 {
		return fmt.Errorf("documents.max_pages must be between 1 and 500")
	}
	if d.Extraction.MaxCharsPerRequest < 1000 {
		return fmt.Errorf("documents.extraction.max_chars_per_request must be at least 1000")
	}
	if d.Job.Lease < 30*time.Second {
		return fmt.Errorf("documents.job.lease must be at least 30s")
	}
	if d.Job.Timeout < time.Minute {
		return fmt.Errorf("documents.job.timeout must be at least 1m")
	}
	if d.Job.MaxConcurrent > 16 {
		return fmt.Errorf("documents.job.max_concurrent must not exceed 16")
	}
	return nil
}

// ValidateForEnvironment adds the checks that depend on where the server runs.
// Exam papers must not travel unencrypted or into a bucket the server made up
// on its own in production.
func (d *DocumentsConfig) ValidateForEnvironment(env string) error {
	if !d.Enabled || env != "production" || d.Storage.Provider != DocumentStorageS3 {
		return nil
	}
	if !d.Storage.S3.UseSSL {
		return fmt.Errorf("documents.storage.s3.use_ssl must be true in production: exam papers must not be sent unencrypted")
	}
	if d.Storage.S3.CreateBucket {
		return fmt.Errorf("documents.storage.s3.create_bucket must be false in production: provision the bucket, with its access policy and encryption, deliberately")
	}
	return nil
}
