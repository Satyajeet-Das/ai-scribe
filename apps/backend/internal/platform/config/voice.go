package config

import (
	"fmt"
	"strings"
	"time"
)

const (
	VoiceProviderDisabled  = "disabled"
	VoiceProviderDeepgram  = "deepgram"
	VoiceProviderRules     = "rules"
	VoiceProviderAnthropic = "anthropic"
	VoiceProviderOllama    = "ollama"
)

// VoiceConfig configures the real-time voice pipeline (WebSocket transport,
// audio ingestion, STT, intent classification and TTS).
// Every field has a safe default, so the block is optional in the environment.
type VoiceConfig struct {
	Enabled bool `koanf:"enabled"`

	// AllowedOrigins restricts the Origin header of WebSocket upgrades.
	// When empty, server.cors_allowed_origins is used.
	AllowedOrigins []string `koanf:"allowed_origins"`

	MaxConnections        int   `koanf:"max_connections"`
	MaxConnectionsPerUser int   `koanf:"max_connections_per_user"`
	MaxMessageBytes       int64 `koanf:"max_message_bytes"`
	SendQueueSize         int   `koanf:"send_queue_size"`

	WriteTimeout  time.Duration `koanf:"write_timeout"`
	PingInterval  time.Duration `koanf:"ping_interval"`
	PongTimeout   time.Duration `koanf:"pong_timeout"`
	ShutdownGrace time.Duration `koanf:"shutdown_grace"`

	// ActionTimeout bounds every call into the exam-session and answer services.
	ActionTimeout time.Duration `koanf:"action_timeout"`
	// ConfirmationTTL is how long a pending answer confirmation stays valid.
	ConfirmationTTL time.Duration `koanf:"confirmation_ttl"`
	// AuthExpiryGrace is how long a connection survives after its access token
	// expires without an auth.refresh message.
	AuthExpiryGrace time.Duration `koanf:"auth_expiry_grace"`

	Audio  VoiceAudioConfig  `koanf:"audio"`
	STT    VoiceSTTConfig    `koanf:"stt"`
	TTS    VoiceTTSConfig    `koanf:"tts"`
	Intent VoiceIntentConfig `koanf:"intent"`
}

type VoiceAudioConfig struct {
	MaxChunkBytes     int           `koanf:"max_chunk_bytes"`
	MaxStreamBytes    int64         `koanf:"max_stream_bytes"`
	MaxStreamDuration time.Duration `koanf:"max_stream_duration"`
	// BufferChunks is the bounded number of chunks held in memory between the
	// socket and the STT provider.
	BufferChunks int `koanf:"buffer_chunks"`
	// EnqueueTimeout is how long the reader waits on a full buffer before the
	// stream is failed with a backpressure error.
	EnqueueTimeout time.Duration `koanf:"enqueue_timeout"`
}

type VoiceSTTConfig struct {
	Provider       string        `koanf:"provider"`
	APIKey         string        `koanf:"api_key"`
	BaseURL        string        `koanf:"base_url"`
	Model          string        `koanf:"model"`
	Language       string        `koanf:"language"`
	ConnectTimeout time.Duration `koanf:"connect_timeout"`
	// FinalTimeout bounds the wait for the final transcript after audio.end.
	FinalTimeout  time.Duration `koanf:"final_timeout"`
	MaxConcurrent int           `koanf:"max_concurrent"`
}

type VoiceTTSConfig struct {
	Provider      string        `koanf:"provider"`
	APIKey        string        `koanf:"api_key"`
	BaseURL       string        `koanf:"base_url"`
	Model         string        `koanf:"model"`
	SampleRate    int           `koanf:"sample_rate"`
	ChunkBytes    int           `koanf:"chunk_bytes"`
	Timeout       time.Duration `koanf:"timeout"`
	MaxConcurrent int           `koanf:"max_concurrent"`
}

type VoiceIntentConfig struct {
	Provider string `koanf:"provider"`
	APIKey   string `koanf:"api_key"`
	Model    string `koanf:"model"`
	// BaseURL is the Ollama server; used only by the ollama provider.
	BaseURL string `koanf:"base_url"`
	// PromptVersion selects one of the prompts embedded in the binary.
	PromptVersion string `koanf:"prompt_version"`
	// PromptFile optionally overrides the embedded prompt with a file on disk.
	PromptFile         string        `koanf:"prompt_file"`
	Timeout            time.Duration `koanf:"timeout"`
	MinConfidence      float64       `koanf:"min_confidence"`
	MaxConcurrent      int           `koanf:"max_concurrent"`
	MaxTranscriptChars int           `koanf:"max_transcript_chars"`
}

// DefaultVoiceConfig returns the configuration used when no SCRIBE_VOICE.*
// variables are set: the endpoint is on, intents are rule-based, and the
// external speech providers are disabled until credentials are supplied.
func DefaultVoiceConfig() VoiceConfig {
	v := VoiceConfig{Enabled: true}
	v.ApplyDefaults()
	return v
}

// ApplyDefaults fills every unset (zero) field with its default.
func (v *VoiceConfig) ApplyDefaults() {
	setInt(&v.MaxConnections, 500)
	setInt(&v.MaxConnectionsPerUser, 3)
	setInt64(&v.MaxMessageBytes, 64*1024)
	setInt(&v.SendQueueSize, 128)
	setDur(&v.WriteTimeout, 10*time.Second)
	setDur(&v.PingInterval, 20*time.Second)
	setDur(&v.PongTimeout, 10*time.Second)
	setDur(&v.ShutdownGrace, 10*time.Second)
	setDur(&v.ActionTimeout, 8*time.Second)
	setDur(&v.ConfirmationTTL, 45*time.Second)
	setDur(&v.AuthExpiryGrace, 30*time.Second)

	setInt(&v.Audio.MaxChunkBytes, 32*1024)
	setInt64(&v.Audio.MaxStreamBytes, 2*1024*1024)
	setDur(&v.Audio.MaxStreamDuration, 45*time.Second)
	setInt(&v.Audio.BufferChunks, 32)
	setDur(&v.Audio.EnqueueTimeout, 2*time.Second)

	setStr(&v.STT.Provider, VoiceProviderDisabled)
	setStr(&v.STT.BaseURL, "wss://api.deepgram.com")
	setStr(&v.STT.Model, "nova-3")
	setStr(&v.STT.Language, "en")
	setDur(&v.STT.ConnectTimeout, 5*time.Second)
	setDur(&v.STT.FinalTimeout, 8*time.Second)
	setInt(&v.STT.MaxConcurrent, 50)

	setStr(&v.TTS.Provider, VoiceProviderDisabled)
	setStr(&v.TTS.BaseURL, "https://api.deepgram.com")
	setStr(&v.TTS.Model, "aura-2-thalia-en")
	setInt(&v.TTS.SampleRate, 24000)
	setInt(&v.TTS.ChunkBytes, 8*1024)
	setDur(&v.TTS.Timeout, 60*time.Second)
	setInt(&v.TTS.MaxConcurrent, 50)

	setStr(&v.Intent.Provider, VoiceProviderRules)
	v.Intent.Provider = strings.ToLower(strings.TrimSpace(v.Intent.Provider))
	if v.Intent.Provider == VoiceProviderOllama {
		setStr(&v.Intent.BaseURL, "http://localhost:11434")
		setStr(&v.Intent.Model, "llama3.2")
	} else {
		setStr(&v.Intent.Model, "claude-opus-5-5")
	}
	setStr(&v.Intent.PromptVersion, "v1")
	setDur(&v.Intent.Timeout, 8*time.Second)
	if v.Intent.MinConfidence == 0 {
		v.Intent.MinConfidence = 0.75
	}
	setInt(&v.Intent.MaxConcurrent, 20)
	setInt(&v.Intent.MaxTranscriptChars, 2000)

	v.STT.Provider = strings.ToLower(strings.TrimSpace(v.STT.Provider))
	v.TTS.Provider = strings.ToLower(strings.TrimSpace(v.TTS.Provider))
}

// Validate rejects configurations that would start a misconfigured pipeline.
func (v *VoiceConfig) Validate() error {
	if !v.Enabled {
		return nil
	}

	switch v.STT.Provider {
	case VoiceProviderDisabled:
	case VoiceProviderDeepgram:
		if v.STT.APIKey == "" {
			return fmt.Errorf("voice.stt.api_key is required when voice.stt.provider is %q", v.STT.Provider)
		}
	default:
		return fmt.Errorf("invalid voice.stt.provider %q (must be one of: disabled, deepgram)", v.STT.Provider)
	}

	switch v.TTS.Provider {
	case VoiceProviderDisabled:
	case VoiceProviderDeepgram:
		if v.TTS.APIKey == "" {
			return fmt.Errorf("voice.tts.api_key is required when voice.tts.provider is %q", v.TTS.Provider)
		}
	default:
		return fmt.Errorf("invalid voice.tts.provider %q (must be one of: disabled, deepgram)", v.TTS.Provider)
	}

	switch v.Intent.Provider {
	case VoiceProviderRules:
	case VoiceProviderOllama:
		if v.Intent.BaseURL == "" || v.Intent.Model == "" {
			return fmt.Errorf("voice.intent.base_url and voice.intent.model are required when voice.intent.provider is %q", v.Intent.Provider)
		}
	case VoiceProviderAnthropic:
		if v.Intent.APIKey == "" {
			return fmt.Errorf("voice.intent.api_key is required when voice.intent.provider is %q", v.Intent.Provider)
		}
	default:
		return fmt.Errorf("invalid voice.intent.provider %q (must be one of: rules, anthropic, ollama)", v.Intent.Provider)
	}

	if v.Intent.MinConfidence < 0 || v.Intent.MinConfidence > 1 {
		return fmt.Errorf("voice.intent.min_confidence must be between 0 and 1")
	}
	// A binary audio frame carries a fixed header in front of the chunk.
	if int64(v.Audio.MaxChunkBytes)+64 > v.MaxMessageBytes {
		return fmt.Errorf("voice.max_message_bytes (%d) must exceed voice.audio.max_chunk_bytes (%d) by at least 64 bytes", v.MaxMessageBytes, v.Audio.MaxChunkBytes)
	}
	if v.Audio.MaxStreamBytes < int64(v.Audio.MaxChunkBytes) {
		return fmt.Errorf("voice.audio.max_stream_bytes must be at least voice.audio.max_chunk_bytes")
	}
	if v.PongTimeout >= v.PingInterval {
		return fmt.Errorf("voice.pong_timeout must be shorter than voice.ping_interval")
	}
	return nil
}

func setInt(p *int, d int) {
	if *p <= 0 {
		*p = d
	}
}

func setInt64(p *int64, d int64) {
	if *p <= 0 {
		*p = d
	}
}

func setDur(p *time.Duration, d time.Duration) {
	if *p <= 0 {
		*p = d
	}
}

func setStr(p *string, d string) {
	if strings.TrimSpace(*p) == "" {
		*p = d
	}
}
