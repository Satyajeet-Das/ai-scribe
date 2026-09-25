package jwt

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
)

// GenerateAccessToken generates an HMAC-SHA256 signed JWT for the given user.
func GenerateAccessToken(cfg Config, userID uuid.UUID, role platformauth.Role, email string) (string, string, time.Time, error) {
	jti := uuid.New().String()
	now := time.Now().UTC()
	expiresAt := now.Add(cfg.AccessTokenDuration)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Issuer:    cfg.Issuer,
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{"ai-scribe-client"},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)), // ±30s clock skew tolerance
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
		UserID: userID,
		Role:   role,
		Email:  email,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.SecretKey))
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("signing access token: %w", err)
	}

	return tokenString, jti, expiresAt, nil
}

// GenerateRefreshToken generates a secure cryptographically random 32-byte token and its SHA-256 hash.
func GenerateRefreshToken(cfg Config) (rawToken string, tokenHash string, expiresAt time.Time, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", time.Time{}, fmt.Errorf("generating random refresh token: %w", err)
	}

	rawToken = hex.EncodeToString(bytes)
	tokenHash = HashRefreshToken(rawToken)
	expiresAt = time.Now().UTC().Add(cfg.RefreshTokenDuration)

	return rawToken, tokenHash, expiresAt, nil
}

// HashRefreshToken calculates the SHA-256 hex hash of a raw refresh token for safe database persistence.
func HashRefreshToken(rawToken string) string {
	hash := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(hash[:])
}
