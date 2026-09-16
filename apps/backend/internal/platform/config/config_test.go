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
