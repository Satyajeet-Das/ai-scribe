package auth

import (
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
