package voice

import (
	"fmt"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech/deepgram"
	"github.com/Satyajeet-Das/ai-scribe/internal/voice/intent"
)

// Providers are the external capabilities selected by configuration.
type Providers struct {
	STT        speech.STTProvider
	TTS        speech.TTSProvider
	Classifier intent.Classifier
}

// BuildProviders constructs the configured STT, TTS and intent providers,
// each wrapped in its concurrency limit. Credentials come only from cfg.
func BuildProviders(cfg config.VoiceConfig) (Providers, error) {
	var p Providers

	switch cfg.STT.Provider {
	case config.VoiceProviderDeepgram:
		p.STT = speech.LimitSTT(deepgram.NewSTT(deepgram.STTConfig{
			APIKey:  cfg.STT.APIKey,
			BaseURL: cfg.STT.BaseURL,
			Model:   cfg.STT.Model,
		}), cfg.STT.MaxConcurrent)
	case config.VoiceProviderDisabled, "":
		p.STT = speech.DisabledSTT()
	default:
		return p, fmt.Errorf("unsupported STT provider %q", cfg.STT.Provider)
	}

	switch cfg.TTS.Provider {
	case config.VoiceProviderDeepgram:
		p.TTS = speech.LimitTTS(deepgram.NewTTS(deepgram.TTSConfig{
			APIKey:     cfg.TTS.APIKey,
			BaseURL:    cfg.TTS.BaseURL,
			Model:      cfg.TTS.Model,
			SampleRate: cfg.TTS.SampleRate,
			ChunkBytes: cfg.TTS.ChunkBytes,
		}), cfg.TTS.MaxConcurrent)
	case config.VoiceProviderDisabled, "":
		p.TTS = speech.DisabledTTS()
	default:
		return p, fmt.Errorf("unsupported TTS provider %q", cfg.TTS.Provider)
	}

	switch cfg.Intent.Provider {
	case config.VoiceProviderAnthropic:
		prompt, err := intent.LoadPrompt(cfg.Intent.PromptVersion, cfg.Intent.PromptFile)
		if err != nil {
			return p, err
		}
		version := cfg.Intent.PromptVersion
		if cfg.Intent.PromptFile != "" {
			version = "file"
		}
		// Exact commands are matched locally first; the model is consulted
		// only for utterances the fixed vocabulary cannot place.
		p.Classifier = intent.Chain{
			First: intent.Rules{},
			Second: intent.NewAnthropic(intent.AnthropicConfig{
				APIKey:        cfg.Intent.APIKey,
				Model:         cfg.Intent.Model,
				Prompt:        prompt,
				PromptVersion: version,
				Timeout:       cfg.Intent.Timeout,
				MaxConcurrent: cfg.Intent.MaxConcurrent,
			}),
		}
	case config.VoiceProviderOllama:
		prompt, err := intent.LoadPrompt(cfg.Intent.PromptVersion, cfg.Intent.PromptFile)
		if err != nil {
			return p, err
		}
		version := cfg.Intent.PromptVersion
		if cfg.Intent.PromptFile != "" {
			version = "file"
		}
		p.Classifier = intent.Chain{
			First: intent.Rules{},
			Second: intent.NewOllama(intent.OllamaConfig{
				BaseURL:       cfg.Intent.BaseURL,
				Model:         cfg.Intent.Model,
				Prompt:        prompt,
				PromptVersion: version,
				Timeout:       cfg.Intent.Timeout,
				MaxConcurrent: cfg.Intent.MaxConcurrent,
			}),
		}
	case config.VoiceProviderRules, "":
		p.Classifier = intent.Rules{}
	default:
		return p, fmt.Errorf("unsupported intent provider %q", cfg.Intent.Provider)
	}

	return p, nil
}
