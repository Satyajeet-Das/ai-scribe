package auth

import (
	"time"

	"github.com/google/uuid"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
)

type Role = platformauth.Role

const (
	RoleTeacher   Role = platformauth.RoleTeacher
	RoleStudent   Role = platformauth.RoleStudent
	RoleEducator  Role = platformauth.RoleEducator
	RoleCandidate Role = platformauth.RoleCandidate
	RoleProctor   Role = platformauth.RoleProctor
	RoleAdmin     Role = platformauth.RoleAdmin
)

type RefreshTokenStatus string

const (
	RefreshTokenStatusActive   RefreshTokenStatus = "ACTIVE"
	RefreshTokenStatusConsumed RefreshTokenStatus = "CONSUMED"
	RefreshTokenStatusRevoked  RefreshTokenStatus = "REVOKED"
)

type RefreshToken struct {
	ID        uuid.UUID          `json:"id" db:"id"`
	UserID    uuid.UUID          `json:"userId" db:"user_id"`
	TokenHash string             `json:"-" db:"token_hash"`
	Status    RefreshTokenStatus `json:"status" db:"status"`
	ExpiresAt time.Time          `json:"expiresAt" db:"expires_at"`
	CreatedAt time.Time          `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time          `json:"updatedAt" db:"updated_at"`
}

type UserClaims struct {
	Subject     string   `json:"sub"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	Email       string   `json:"email,omitempty"`
	FirstName   string   `json:"firstName,omitempty"`
	LastName    string   `json:"lastName,omitempty"`
	SessionID   string   `json:"sessionId,omitempty"`
}

type contextKey string

const (
	UserClaimsContextKey contextKey = "auth_user_claims"
)
