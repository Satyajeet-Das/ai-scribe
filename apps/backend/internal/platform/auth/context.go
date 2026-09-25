package auth

import (
	"errors"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

const (
	IdentityContextKey = "auth_identity"
	UserIDKey          = "user_id"
	UserRoleKey        = "user_role"
)

// SetIdentity stores the authenticated identity into the echo.Context.
// For seamless backwards compatibility with existing handlers and middlewares,
// it also sets "user_id" (as UUID string) and "user_role" (as role string).
func SetIdentity(c echo.Context, identity *AuthenticatedIdentity) {
	if identity == nil {
		return
	}
	c.Set(IdentityContextKey, identity)
	c.Set(UserIDKey, identity.UserID.String())
	c.Set(UserRoleKey, string(identity.Role))
}

// GetIdentity retrieves the AuthenticatedIdentity from the echo.Context.
func GetIdentity(c echo.Context) (*AuthenticatedIdentity, bool) {
	val := c.Get(IdentityContextKey)
	if val == nil {
		return nil, false
	}
	identity, ok := val.(*AuthenticatedIdentity)
	return identity, ok
}

// GetCallerUserID retrieves the internal UUID of the authenticated user.
func GetCallerUserID(c echo.Context) (uuid.UUID, error) {
	if identity, ok := GetIdentity(c); ok && identity.UserID != uuid.Nil {
		return identity.UserID, nil
	}

	// Fallback to "user_id" string if present
	if val := c.Get(UserIDKey); val != nil {
		if idStr, ok := val.(string); ok && idStr != "" {
			return uuid.Parse(idStr)
		}
	}

	return uuid.Nil, errors.New("unauthenticated: user ID not found in context")
}

// GetCallerRole retrieves the role of the authenticated user.
func GetCallerRole(c echo.Context) Role {
	if identity, ok := GetIdentity(c); ok {
		return identity.Role
	}
	if val := c.Get(UserRoleKey); val != nil {
		if roleStr, ok := val.(string); ok {
			return Role(roleStr)
		}
	}
	return ""
}
