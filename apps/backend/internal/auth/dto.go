package auth

import (
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
)

var validate = validator.New()

type RegisterRequest struct {
	Email     string            `json:"email" validate:"required,email"`
	Password  string            `json:"password" validate:"required,min=8"`
	FirstName string            `json:"firstName" validate:"required"`
	LastName  string            `json:"lastName" validate:"required"`
	Role      platformauth.Role `json:"role" validate:"required"`
}

func (r *RegisterRequest) Validate() error {
	return validate.Struct(r)
}

type UserResponse struct {
	ID        uuid.UUID         `json:"id"`
	Email     string            `json:"email"`
	FirstName string            `json:"firstName"`
	LastName  string            `json:"lastName"`
	Role      platformauth.Role `json:"role"`
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
