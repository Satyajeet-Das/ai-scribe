package jwt

import "time"

type Config struct {
	SecretKey            string
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration
	Issuer               string
}

func DefaultConfig(secretKey string) Config {
	return Config{
		SecretKey:            secretKey,
		AccessTokenDuration:  15 * time.Minute,
		RefreshTokenDuration: 7 * 24 * time.Hour,
		Issuer:               "ai-scribe",
	}
}
