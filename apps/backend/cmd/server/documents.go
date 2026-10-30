package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/document"
	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract/groq"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
)

// openDocumentStore opens where uploaded exam PDFs are kept. They are private
// objects, read back only through an endpoint that checks who owns the exam. The
// store holds PDFs only: it refuses keys of any other kind, so pointing it at the
// answer-audio directory or bucket cannot make one module list the other's files.
func openDocumentStore(ctx context.Context, cfg config.DocumentsStorageConfig, log *zerolog.Logger) (audiostore.Store, error) {
	switch cfg.Provider {
	case config.DocumentStorageS3:
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		store, err := audiostore.NewS3(ctx, audiostore.S3Config{
			Endpoint: cfg.S3.Endpoint, Bucket: cfg.S3.Bucket,
			AccessKey: cfg.S3.AccessKey, SecretKey: cfg.S3.SecretKey,
			Region: cfg.S3.Region, UseSSL: cfg.S3.UseSSL, CreateBucket: cfg.S3.CreateBucket,
			Kind: audiostore.KindPDF,
		})
		if err != nil {
			return nil, fmt.Errorf("document store (s3): %w", err)
		}
		return store, nil
	default:
		store, err := audiostore.NewLocalFS(cfg.Dir, audiostore.WithKind(audiostore.KindPDF))
		if err != nil {
			return nil, fmt.Errorf("document store (localfs): %w", err)
		}
		return store, nil
	}
}

// buildDocumentExtractor returns the extractor the configuration asks for. With
// the provider "disabled" it is one that never calls anything.
func buildDocumentExtractor(cfg config.DocumentsExtractionConfig) (document.Extractor, error) {
	switch cfg.Provider {
	case config.DocumentProviderGroq:
		prompt, err := extract.Prompt(extract.DefaultPromptVersion)
		if err != nil {
			return nil, err
		}
		model := groq.New(groq.Config{
			APIKey: cfg.APIKey, BaseURL: cfg.BaseURL,
			TextModel: cfg.TextModel, VisionModel: cfg.VisionModel,
			Prompt: prompt, PromptVersion: extract.DefaultPromptVersion,
			Timeout: cfg.RequestTimeout,
		})
		return extract.New(model, extract.Options{MaxCharsPerRequest: cfg.MaxCharsPerRequest}), nil
	case config.DocumentProviderDisabled, "":
		return extract.New(extract.Disabled(), extract.Options{}), nil
	}
	return nil, fmt.Errorf("unsupported documents.extraction.provider %q", cfg.Provider)
}

// documentOptions maps configuration to the service's limits.
func documentOptions(cfg config.DocumentsConfig) document.Options {
	return document.Options{
		MaxBytes: cfg.MaxBytes, MaxPages: cfg.MaxPages, ParseTimeout: cfg.ParseTimeout,
		Lease: cfg.Job.Lease, MaxConcurrent: cfg.Job.MaxConcurrent,
		MaxImagesPerPage: cfg.Extraction.MaxImagesPerRequest,
		Active:           cfg.Active(),
	}
}
