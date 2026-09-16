package auth

import (
	"context"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwt"
)

type Service interface {
	VerifyToken(ctx context.Context, token string) (*UserClaims, error)
}

type authService struct {
	secretKey string
}

func NewService(secretKey string) Service {
	clerk.SetKey(secretKey)
	return &authService{
		secretKey: secretKey,
	}
}

func (s *authService) VerifyToken(ctx context.Context, token string) (*UserClaims, error) {
	claims, err := jwt.Verify(ctx, &jwt.VerifyParams{
		Token: token,
	})
	if err != nil {
		return nil, err
	}

	return &UserClaims{
		Subject: claims.Subject,
	}, nil
}
