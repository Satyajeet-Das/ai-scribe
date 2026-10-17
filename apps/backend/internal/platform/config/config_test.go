package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultObservabilityConfig(t *testing.T) {
	cfg := DefaultObservabilityConfig()

	require.NotNil(t, cfg)
	assert.Equal(t, "ai-scribe", cfg.ServiceName)
	assert.Equal(t, "development", cfg.Environment)
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, "json", cfg.Logging.Format)
	assert.Equal(t, 100*time.Millisecond, cfg.Logging.SlowQueryThreshold)
	assert.True(t, cfg.HealthChecks.Enabled)
	assert.Contains(t, cfg.HealthChecks.Checks, "database")
	assert.Contains(t, cfg.HealthChecks.Checks, "redis")

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestObservabilityConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(c *ObservabilityConfig)
		wantErr bool
	}{
		{
			name: "valid config",
			modify: func(c *ObservabilityConfig) {
				// No modifications, valid by default
			},
			wantErr: false,
		},
		{
			name: "missing service name",
			modify: func(c *ObservabilityConfig) {
				c.ServiceName = ""
			},
			wantErr: true,
		},
		{
			name: "invalid logging level",
			modify: func(c *ObservabilityConfig) {
				c.Logging.Level = "verbose"
			},
			wantErr: true,
		},
		{
			name: "negative slow query threshold",
			modify: func(c *ObservabilityConfig) {
				c.Logging.SlowQueryThreshold = -1 * time.Second
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultObservabilityConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestObservabilityConfig_Environment(t *testing.T) {
	cfg := DefaultObservabilityConfig()
	assert.False(t, cfg.IsProduction())

	cfg.Environment = "production"
	assert.True(t, cfg.IsProduction())
}

func TestDefaultVoiceConfig(t *testing.T) {
	cfg := DefaultVoiceConfig()

	assert.True(t, cfg.Enabled)
	assert.Equal(t, VoiceProviderDisabled, cfg.STT.Provider, "no external provider is used without credentials")
	assert.Equal(t, VoiceProviderDisabled, cfg.TTS.Provider)
	assert.Equal(t, VoiceProviderRules, cfg.Intent.Provider)
	assert.Equal(t, "v1", cfg.Intent.PromptVersion)
	assert.Greater(t, cfg.MaxMessageBytes, int64(cfg.Audio.MaxChunkBytes))
	assert.Less(t, cfg.PongTimeout, cfg.PingInterval)
	assert.NoError(t, cfg.Validate())
}

func TestVoiceConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(c *VoiceConfig)
		wantErr bool
	}{
		{"defaults", func(c *VoiceConfig) {}, false},
		{"deepgram stt without key", func(c *VoiceConfig) { c.STT.Provider = "deepgram" }, true},
		{"deepgram stt with key", func(c *VoiceConfig) { c.STT.Provider = "Deepgram"; c.STT.APIKey = "k" }, false},
		{"deepgram tts without key", func(c *VoiceConfig) { c.TTS.Provider = "deepgram" }, true},
		{"anthropic intent without key", func(c *VoiceConfig) { c.Intent.Provider = "anthropic" }, true},
		{"anthropic intent with key", func(c *VoiceConfig) { c.Intent.Provider = "anthropic"; c.Intent.APIKey = "k" }, false},
		{"groq intent without key", func(c *VoiceConfig) { c.Intent.Provider = "groq" }, true},
		{"groq intent with key", func(c *VoiceConfig) {
			c.Intent.Provider = "groq"
			c.Intent.APIKey = "k"
			c.Intent.Model = DefaultGroqModel
		}, false},
		{"unknown stt provider", func(c *VoiceConfig) { c.STT.Provider = "whisper" }, true},
		{"unknown tts provider", func(c *VoiceConfig) { c.TTS.Provider = "espeak" }, true},
		{"unknown intent provider", func(c *VoiceConfig) { c.Intent.Provider = "gpt" }, true},
		{"confidence out of range", func(c *VoiceConfig) { c.Intent.MinConfidence = 1.5 }, true},
		{"chunk larger than message limit", func(c *VoiceConfig) { c.Audio.MaxChunkBytes = int(c.MaxMessageBytes) }, true},
		{"stream limit below chunk limit", func(c *VoiceConfig) { c.Audio.MaxStreamBytes = 10 }, true},
		{"pong timeout not below ping interval", func(c *VoiceConfig) { c.PongTimeout = c.PingInterval }, true},
		{"disabled voice skips validation", func(c *VoiceConfig) { c.Enabled = false; c.STT.Provider = "deepgram" }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultVoiceConfig()
			tt.modify(&cfg)
			cfg.ApplyDefaults()
			err := cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadConfig_VoiceFromEnvironment(t *testing.T) {
	for k, v := range map[string]string{
		"SCRIBE_PRIMARY.ENV":                 "test",
		"SCRIBE_SERVER.PORT":                 "8080",
		"SCRIBE_SERVER.READ_TIMEOUT":         "30",
		"SCRIBE_SERVER.WRITE_TIMEOUT":        "30",
		"SCRIBE_SERVER.IDLE_TIMEOUT":         "60",
		"SCRIBE_SERVER.CORS_ALLOWED_ORIGINS": "http://localhost:3000",
		"SCRIBE_DATABASE.HOST":               "localhost",
		"SCRIBE_DATABASE.PORT":               "5432",
		"SCRIBE_DATABASE.USER":               "postgres",
		"SCRIBE_DATABASE.NAME":               "ai_scribe",
		"SCRIBE_DATABASE.SSL_MODE":           "disable",
		"SCRIBE_DATABASE.MAX_OPEN_CONNS":     "5",
		"SCRIBE_DATABASE.MAX_IDLE_CONNS":     "5",
		"SCRIBE_DATABASE.CONN_MAX_LIFETIME":  "300",
		"SCRIBE_DATABASE.CONN_MAX_IDLE_TIME": "300",
		"SCRIBE_REDIS.ADDRESS":               "localhost:6379",
		"SCRIBE_AUTH.SECRET_KEY":             "secret",
		"SCRIBE_VOICE.STT.PROVIDER":          "deepgram",
		"SCRIBE_VOICE.STT.API_KEY":           "dg-key",
		"SCRIBE_VOICE.STT.FINAL_TIMEOUT":     "3s",
		"SCRIBE_VOICE.INTENT.MIN_CONFIDENCE": "0.9",
		"SCRIBE_VOICE.MAX_CONNECTIONS":       "42",
		"SCRIBE_VOICE.ALLOWED_ORIGINS":       "https://exam.example.com,https://admin.example.com",
	} {
		t.Setenv(k, v)
	}

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.True(t, cfg.Voice.Enabled, "voice is on unless SCRIBE_VOICE.ENABLED says otherwise")
	assert.Equal(t, "deepgram", cfg.Voice.STT.Provider)
	assert.Equal(t, "dg-key", cfg.Voice.STT.APIKey)
	assert.Equal(t, 3*time.Second, cfg.Voice.STT.FinalTimeout)
	assert.Equal(t, 0.9, cfg.Voice.Intent.MinConfidence)
	assert.Equal(t, 42, cfg.Voice.MaxConnections)
	assert.Equal(t, VoiceProviderDisabled, cfg.Voice.TTS.Provider)
	assert.NotEmpty(t, cfg.Voice.AllowedOrigins)

	t.Setenv("SCRIBE_VOICE.ENABLED", "false")
	cfg, err = LoadConfig()
	require.NoError(t, err)
	assert.False(t, cfg.Voice.Enabled)

	t.Setenv("SCRIBE_VOICE.ENABLED", "true")
	t.Setenv("SCRIBE_VOICE.STT.API_KEY", "")
	_, err = LoadConfig()
	assert.Error(t, err, "a provider without credentials fails at startup, not at the first student")
}

func TestVoiceConfig_GroqDefaults(t *testing.T) {
	t.Run("a Groq provider gets Groq's endpoint and a Groq model, not another provider's", func(t *testing.T) {
		v := VoiceConfig{Enabled: true}
		v.Intent.Provider = "Groq" // case does not matter
		v.ApplyDefaults()

		assert.Equal(t, VoiceProviderGroq, v.Intent.Provider)
		assert.Equal(t, "https://api.groq.com/openai/v1", v.Intent.BaseURL)
		assert.Equal(t, "llama-3.3-70b-versatile", v.Intent.Model)
		assert.NotContains(t, v.Intent.Model, "claude")
	})
	t.Run("what the operator sets is kept", func(t *testing.T) {
		v := VoiceConfig{Enabled: true}
		v.Intent.Provider = "groq"
		v.Intent.Model = "openai/gpt-oss-20b"
		v.Intent.BaseURL = "https://proxy.example/openai/v1"
		v.ApplyDefaults()
		assert.Equal(t, "openai/gpt-oss-20b", v.Intent.Model)
		assert.Equal(t, "https://proxy.example/openai/v1", v.Intent.BaseURL)
	})
	t.Run("other providers are unaffected", func(t *testing.T) {
		a := VoiceConfig{Enabled: true}
		a.Intent.Provider = "anthropic"
		a.ApplyDefaults()
		assert.Equal(t, "claude-opus-5-5", a.Intent.Model)
		assert.Empty(t, a.Intent.BaseURL)

		o := VoiceConfig{Enabled: true}
		o.Intent.Provider = "ollama"
		o.ApplyDefaults()
		assert.Equal(t, "llama3.2", o.Intent.Model)
		assert.Equal(t, "http://localhost:11434", o.Intent.BaseURL)
	})
	t.Run("the key is required, and the error says which provider", func(t *testing.T) {
		v := VoiceConfig{Enabled: true}
		v.Intent.Provider = "groq"
		v.ApplyDefaults()
		err := v.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "api_key")
		assert.Contains(t, err.Error(), "groq")
	})
	t.Run("another provider's leftover model or address is refused with a reason", func(t *testing.T) {
		stale := func(mutate func(*VoiceConfig)) error {
			v := VoiceConfig{Enabled: true}
			v.Intent.Provider = "groq"
			v.Intent.APIKey = "k"
			v.ApplyDefaults()
			mutate(&v)
			return v.Validate()
		}
		err := stale(func(v *VoiceConfig) { v.Intent.Model = "claude-opus-5-5" })
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a Groq model")
		assert.Contains(t, err.Error(), "llama-3.3-70b-versatile", "it says what to use instead")

		err = stale(func(v *VoiceConfig) { v.Intent.BaseURL = "http://localhost:11434/" })
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Ollama")

		assert.NoError(t, stale(func(v *VoiceConfig) {}), "the defaults are fine")
		assert.NoError(t, stale(func(v *VoiceConfig) { v.Intent.Model = "openai/gpt-oss-20b" }))
		assert.NoError(t, stale(func(v *VoiceConfig) { v.Intent.BaseURL = "https://proxy.example/openai/v1" }))
	})
	t.Run("the list of valid providers mentions groq", func(t *testing.T) {
		v := DefaultVoiceConfig()
		v.Intent.Provider = "nonsense"
		err := v.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "groq")
	})
}
