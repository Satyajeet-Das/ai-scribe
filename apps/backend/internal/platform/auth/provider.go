package auth

import "context"

// AuthProvider defines the contract for authenticating bearer tokens.
// Implementations (JWT, Clerk, etc.) translate provider-specific tokens
// into a provider-independent AuthenticatedIdentity.
type AuthProvider interface {
	Authenticate(ctx context.Context, rawToken string) (*AuthenticatedIdentity, error)
}
