package jwt

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
)

type Claims struct {
	jwt.RegisteredClaims
	UserID      uuid.UUID         `json:"user_id"`
	Role        platformauth.Role `json:"role"`
	Permissions []string          `json:"permissions,omitempty"`
	Email       string            `json:"email,omitempty"`
}
