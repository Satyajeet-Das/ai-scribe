package auth

import (
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/user"
)

var validate = validator.New()

type RegisterRequest struct {
	Email     string            `json:"email" validate:"required,email"`
	Password  string            `json:"password" validate:"required,min=8"`
	FirstName string            `json:"firstName" validate:"required"`
	LastName  string            `json:"lastName" validate:"required"`
	Role      platformauth.Role `json:"role" validate:"required"`
	RollNo    *string           `json:"rollNo,omitempty"`
}

func (r *RegisterRequest) Validate() error {
	if err := validate.Struct(r); err != nil {
		return err
	}
	if r.Role == platformauth.RoleStudent {
		if r.RollNo == nil || strings.TrimSpace(*r.RollNo) == "" {
			return user.ErrRollNoRequired
		}
	}
	return nil
}

type UserResponse struct {
	ID        uuid.UUID         `json:"id"`
	Email     string            `json:"email"`
	FirstName string            `json:"firstName"`
	LastName  string            `json:"lastName"`
	Role      platformauth.Role `json:"role"`
	RollNo    *string           `json:"rollNo,omitempty"`
}

type RegisterResponse struct {
	User UserResponse `json:"user"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

func (r *LoginRequest) Validate() error {
	return validate.Struct(r)
}

type LoginResponse struct {
	AccessToken string       `json:"accessToken"`
	ExpiresIn   int64        `json:"expiresIn"`
	User        UserResponse `json:"user"`
}

type RefreshResponse struct {
	AccessToken string `json:"accessToken"`
	ExpiresIn   int64  `json:"expiresIn"`
}

type LogoutResponse struct {
	Message string `json:"message"`
}
