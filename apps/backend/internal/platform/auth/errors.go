package auth

import "errors"

var (
	ErrInvalidToken        = errors.New("invalid or malformed token")
	ErrExpiredToken        = errors.New("token has expired")
	ErrRevokedToken        = errors.New("token has been revoked")
	ErrUserNotFound        = errors.New("user not found")
	ErrUserDeactivated     = errors.New("user account is deactivated")
	ErrInsufficientRole    = errors.New("insufficient role permissions")
	ErrRateLimitExceeded   = errors.New("rate limit exceeded")
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrProviderUnavailable = errors.New("authentication provider unavailable")
)
