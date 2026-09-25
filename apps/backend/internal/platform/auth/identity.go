package auth

import (
	"time"

	"github.com/google/uuid"
)

type AuthenticatedIdentity struct {
	UserID      uuid.UUID `json:"userId"`
	Role        Role      `json:"role"`
	Permissions []string  `json:"permissions,omitempty"`
	Email       string    `json:"email,omitempty"`
	FirstName   string    `json:"firstName,omitempty"`
	LastName    string    `json:"lastName,omitempty"`
	Provider    string    `json:"provider"`
	TokenID     string    `json:"tokenId,omitempty"`
	ExpiresAt   time.Time `json:"expiresAt"`
}
