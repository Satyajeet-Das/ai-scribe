package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	jwtadapter "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth/adapters/jwt"
	"github.com/Satyajeet-Das/ai-scribe/internal/user"
)

var (
	ErrPasswordTooShort = errors.New("password must be at least 8 characters long")
	ErrInvalidRole      = errors.New("invalid user role")
)

type Service interface {
	Register(ctx context.Context, req RegisterRequest) (*UserResponse, error)
	Login(ctx context.Context, req LoginRequest) (*LoginResponse, string, time.Time, error)
	Refresh(ctx context.Context, rawRefreshToken string) (*RefreshResponse, string, time.Time, error)
	Logout(ctx context.Context, identity *platformauth.AuthenticatedIdentity, rawRefreshToken string) error
	GetMe(ctx context.Context, userID uuid.UUID) (*UserResponse, error)
}

type authService struct {
	userRepo   user.Repository
	authRepo   Repository
	jwtAdapter *jwtadapter.Adapter
	jwtConfig  jwtadapter.Config
	logger     *zerolog.Logger
}

func NewService(
	userRepo user.Repository,
	authRepo Repository,
	jwtAdapter *jwtadapter.Adapter,
	jwtConfig jwtadapter.Config,
	logger *zerolog.Logger,
) Service {
	return &authService{
		userRepo:   userRepo,
		authRepo:   authRepo,
		jwtAdapter: jwtAdapter,
		jwtConfig:  jwtConfig,
		logger:     logger,
	}
}

func (s *authService) Register(ctx context.Context, req RegisterRequest) (*UserResponse, error) {
	if len(req.Password) < 8 {
		return nil, ErrPasswordTooShort
	}

	if !req.Role.IsValid() {
		return nil, ErrInvalidRole
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	newUser := &user.User{
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Role:         string(req.Role),
		IsActive:     true,
	}

	if err := s.userRepo.Create(ctx, newUser); err != nil {
		if errors.Is(err, user.ErrUserAlreadyExists) {
			return nil, err
		}
		s.logger.Error().Err(err).Str("email", req.Email).Msg("failed to create user in register")
		return nil, fmt.Errorf("creating user: %w", err)
	}

	s.logger.Info().
		Str("user_id", newUser.ID.String()).
		Str("email", newUser.Email).
		Str("role", newUser.Role).
		Msg("user registered successfully")

	return &UserResponse{
		ID:        newUser.ID,
		Email:     newUser.Email,
		FirstName: newUser.FirstName,
		LastName:  newUser.LastName,
		Role:      platformauth.Role(newUser.Role),
	}, nil
}

func (s *authService) Login(ctx context.Context, req LoginRequest) (*LoginResponse, string, time.Time, error) {
	u, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, "", time.Time{}, platformauth.ErrInvalidCredentials
		}
		return nil, "", time.Time{}, err
	}

	if !u.IsActive {
		return nil, "", time.Time{}, platformauth.ErrUserDeactivated
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		return nil, "", time.Time{}, platformauth.ErrInvalidCredentials
	}

	// 1. Generate short-lived access token
	accessToken, _, accessExpiresAt, err := jwtadapter.GenerateAccessToken(s.jwtConfig, u.ID, platformauth.Role(u.Role), u.Email)
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("generating access token: %w", err)
	}

	// 2. Generate long-lived refresh token
	rawRefreshToken, tokenHash, refreshExpiresAt, err := jwtadapter.GenerateRefreshToken(s.jwtConfig)
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("generating refresh token: %w", err)
	}

	// 3. Persist refresh token in database
	refreshTokenModel := &RefreshToken{
		UserID:    u.ID,
		TokenHash: tokenHash,
		Status:    RefreshTokenStatusActive,
		ExpiresAt: refreshExpiresAt,
	}
	if err := s.authRepo.CreateRefreshToken(ctx, refreshTokenModel); err != nil {
		s.logger.Error().Err(err).Str("user_id", u.ID.String()).Msg("failed to persist refresh token")
		return nil, "", time.Time{}, fmt.Errorf("persisting refresh token: %w", err)
	}

	expiresIn := int64(time.Until(accessExpiresAt).Seconds())
	if expiresIn <= 0 {
		expiresIn = int64(s.jwtConfig.AccessTokenDuration.Seconds())
	}

	resp := &LoginResponse{
		AccessToken: accessToken,
		ExpiresIn:   expiresIn,
		User: UserResponse{
			ID:        u.ID,
			Email:     u.Email,
			FirstName: u.FirstName,
			LastName:  u.LastName,
			Role:      platformauth.Role(u.Role),
		},
	}

	s.logger.Info().
		Str("user_id", u.ID.String()).
		Str("email", u.Email).
		Msg("user logged in successfully")

	return resp, rawRefreshToken, refreshExpiresAt, nil
}

func (s *authService) Refresh(ctx context.Context, rawRefreshToken string) (*RefreshResponse, string, time.Time, error) {
	if rawRefreshToken == "" {
		return nil, "", time.Time{}, platformauth.ErrInvalidToken
	}

	tokenHash := jwtadapter.HashRefreshToken(rawRefreshToken)
	storedToken, err := s.authRepo.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, ErrRefreshTokenNotFound) {
			return nil, "", time.Time{}, platformauth.ErrInvalidToken
		}
		return nil, "", time.Time{}, err
	}

	// Replay attack detection: if token is already consumed or revoked
	if storedToken.Status != RefreshTokenStatusActive {
		s.logger.Warn().
			Str("user_id", storedToken.UserID.String()).
			Str("status", string(storedToken.Status)).
			Msg("refresh token replay attempt detected! Revoking all sessions for user")

		_ = s.authRepo.RevokeAllUserTokens(ctx, storedToken.UserID)
		if s.jwtAdapter != nil {
			_ = s.jwtAdapter.InvalidateUserCache(ctx, storedToken.UserID)
		}
		return nil, "", time.Time{}, platformauth.ErrRevokedToken
	}

	// Expiration check
	if storedToken.ExpiresAt.Before(time.Now().UTC()) {
		_ = s.authRepo.UpdateRefreshTokenStatus(ctx, storedToken.ID, RefreshTokenStatusRevoked)
		return nil, "", time.Time{}, platformauth.ErrExpiredToken
	}

	// User existence and active check
	u, err := s.userRepo.GetByID(ctx, storedToken.UserID)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, "", time.Time{}, platformauth.ErrUserNotFound
		}
		return nil, "", time.Time{}, err
	}

	if !u.IsActive {
		return nil, "", time.Time{}, platformauth.ErrUserDeactivated
	}

	// Mark old refresh token as CONSUMED (rotation)
	if err := s.authRepo.UpdateRefreshTokenStatus(ctx, storedToken.ID, RefreshTokenStatusConsumed); err != nil {
		s.logger.Error().Err(err).Msg("failed to update old refresh token status to consumed")
		return nil, "", time.Time{}, err
	}

	// Issue new access token
	newAccessToken, _, accessExpiresAt, err := jwtadapter.GenerateAccessToken(s.jwtConfig, u.ID, platformauth.Role(u.Role), u.Email)
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("generating access token during refresh: %w", err)
	}

	// Issue new refresh token
	newRawRefreshToken, newHash, newRefreshExpiresAt, err := jwtadapter.GenerateRefreshToken(s.jwtConfig)
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("generating new refresh token: %w", err)
	}

	newRefreshModel := &RefreshToken{
		UserID:    u.ID,
		TokenHash: newHash,
		Status:    RefreshTokenStatusActive,
		ExpiresAt: newRefreshExpiresAt,
	}
	if err := s.authRepo.CreateRefreshToken(ctx, newRefreshModel); err != nil {
		return nil, "", time.Time{}, fmt.Errorf("persisting new refresh token: %w", err)
	}

	expiresIn := int64(time.Until(accessExpiresAt).Seconds())
	if expiresIn <= 0 {
		expiresIn = int64(s.jwtConfig.AccessTokenDuration.Seconds())
	}

	resp := &RefreshResponse{
		AccessToken: newAccessToken,
		ExpiresIn:   expiresIn,
	}

	s.logger.Info().
		Str("user_id", u.ID.String()).
		Msg("token refreshed successfully")

	return resp, newRawRefreshToken, newRefreshExpiresAt, nil
}

func (s *authService) Logout(ctx context.Context, identity *platformauth.AuthenticatedIdentity, rawRefreshToken string) error {
	// Revoke the refresh token if provided
	if rawRefreshToken != "" {
		tokenHash := jwtadapter.HashRefreshToken(rawRefreshToken)
		if stored, err := s.authRepo.GetRefreshTokenByHash(ctx, tokenHash); err == nil && stored != nil {
			_ = s.authRepo.UpdateRefreshTokenStatus(ctx, stored.ID, RefreshTokenStatusRevoked)
		}
	}

	// Block the access token in Redis if identity is present
	if identity != nil {
		if s.jwtAdapter != nil && identity.TokenID != "" && !identity.ExpiresAt.IsZero() {
			remainingTTL := time.Until(identity.ExpiresAt)
			if remainingTTL > 0 {
				_ = s.jwtAdapter.RevokeToken(ctx, identity.TokenID, remainingTTL)
			}
		}

		if s.jwtAdapter != nil && identity.UserID != uuid.Nil {
			_ = s.jwtAdapter.InvalidateUserCache(ctx, identity.UserID)
		}

		s.logger.Info().
			Str("user_id", identity.UserID.String()).
			Msg("user logged out successfully")
	}

	return nil
}

func (s *authService) GetMe(ctx context.Context, userID uuid.UUID) (*UserResponse, error) {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, platformauth.ErrUserNotFound
		}
		return nil, err
	}

	return &UserResponse{
		ID:        u.ID,
		Email:     u.Email,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Role:      platformauth.Role(u.Role),
	}, nil
}
