package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
	_ "github.com/joho/godotenv/autoload"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
	"github.com/rs/zerolog"
)

type Config struct {
	Primary       Primary              `koanf:"primary" validate:"required"`
	Server        ServerConfig         `koanf:"server" validate:"required"`
	Database      DatabaseConfig       `koanf:"database" validate:"required"`
	Auth          AuthConfig           `koanf:"auth" validate:"required"`
	Redis         RedisConfig          `koanf:"redis" validate:"required"`
	Integration   IntegrationConfig    `koanf:"integration"`
	Observability *ObservabilityConfig `koanf:"observability"`
}

type Primary struct {
	Env string `koanf:"env" validate:"required"`
}

type ServerConfig struct {
	Port               string   `koanf:"port" validate:"required"`
	ReadTimeout        int      `koanf:"read_timeout" validate:"required"`
	WriteTimeout       int      `koanf:"write_timeout" validate:"required"`
	IdleTimeout        int      `koanf:"idle_timeout" validate:"required"`
	CORSAllowedOrigins []string `koanf:"cors_allowed_origins" validate:"required"`
}

type DatabaseConfig struct {
	Host            string `koanf:"host" validate:"required"`
	Port            int    `koanf:"port" validate:"required"`
	User            string `koanf:"user" validate:"required"`
	Password        string `koanf:"password"`
	Name            string `koanf:"name" validate:"required"`
	SSLMode         string `koanf:"ssl_mode" validate:"required"`
	MaxOpenConns    int    `koanf:"max_open_conns" validate:"required"`
	MaxIdleConns    int    `koanf:"max_idle_conns" validate:"required"`
	ConnMaxLifetime int    `koanf:"conn_max_lifetime" validate:"required"`
	ConnMaxIdleTime int    `koanf:"conn_max_idle_time" validate:"required"`
}

type RedisConfig struct {
	Address string `koanf:"address" validate:"required"`
}

type IntegrationConfig struct {
	ResendAPIKey string `koanf:"resend_api_key"`
}

type AuthConfig struct {
	SecretKey string `koanf:"secret_key" validate:"required"`
}

func LoadConfig() (*Config, error) {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()

	k := koanf.New(".")

	// First load legacy BOILERPLATE_ variables if any exist
	_ = k.Load(env.Provider("BOILERPLATE_", ".", func(s string) string {
		return strings.ToLower(strings.TrimPrefix(s, "BOILERPLATE_"))
	}), nil)

	// Then overlay with SCRIBE_ variables
	err := k.Load(env.Provider("SCRIBE_", ".", func(s string) string {
		return strings.ToLower(strings.TrimPrefix(s, "SCRIBE_"))
	}), nil)
	if err != nil {
		logger.Error().Err(err).Msg("could not load env variables")
		return nil, fmt.Errorf("loading env variables: %w", err)
	}

	mainConfig := &Config{}

	if err := k.Unmarshal("", mainConfig); err != nil {
		logger.Error().Err(err).Msg("could not unmarshal config")
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	validate := validator.New()
	if err := validate.Struct(mainConfig); err != nil {
		logger.Error().Err(err).Msg("config validation failed")
		return nil, fmt.Errorf("validating config: %w", err)
	}

	if mainConfig.Observability == nil {
		mainConfig.Observability = DefaultObservabilityConfig()
	}

	if mainConfig.Observability.ServiceName == "" {
		mainConfig.Observability.ServiceName = "ai-scribe"
	}
	mainConfig.Observability.Environment = mainConfig.Primary.Env

	if err := mainConfig.Observability.Validate(); err != nil {
		logger.Error().Err(err).Msg("invalid observability config")
		return nil, fmt.Errorf("validating observability config: %w", err)
	}

	return mainConfig, nil
}
