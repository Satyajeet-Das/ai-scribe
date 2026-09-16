package user

import (
	"github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type User struct {
	model.Base
	ClerkID   string `json:"clerkId" db:"clerk_id"`
	Email     string `json:"email" db:"email"`
	FirstName string `json:"firstName" db:"first_name"`
	LastName  string `json:"lastName" db:"last_name"`
	Role      string `json:"role" db:"role"`
}
