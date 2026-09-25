package jwt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/user"
)

const (
	redisBlocklistPrefix = "auth:blocked:"
	redisUserCachePrefix = "auth:user:"
	defaultCacheTTL      = 15 * time.Minute
)

// Adapter implements platformauth.AuthProvider using HMAC-SHA256 JWT tokens
// backed by Redis caching and PostgreSQL user resolution.
type Adapter struct {
	cfg      Config
	userRepo user.Repository
	redis    *redis.Client
	logger   *zerolog.Logger
}

func NewAdapter(cfg Config, userRepo user.Repository, redisClient *redis.Client, logger *zerolog.Logger) *Adapter {
	return &Adapter{
		cfg:      cfg,
		userRepo: userRepo,
		redis:    redisClient,
		logger:   logger,
	}
}

// Authenticate verifies the raw JWT token string, validates claims, checks the revocation blocklist,
// resolves the internal user entity (via cache or database), and returns an AuthenticatedIdentity.
func (a *Adapter) Authenticate(ctx context.Context, rawToken string) (*platformauth.AuthenticatedIdentity, error) {
	if rawToken == "" {
		return nil, platformauth.ErrInvalidToken
	}

	// 1. Parse and validate signature using HMAC-SHA256 only (mitigate alg:none attack)
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(rawToken, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing algorithm: %v", t.Header["alg"])
		}
		return []byte(a.cfg.SecretKey), nil
	}, jwt.WithLeeway(30*time.Second))

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, platformauth.ErrExpiredToken
		}
		return nil, platformauth.ErrInvalidToken
	}

	if !token.Valid || claims.UserID == uuid.Nil {
		return nil, platformauth.ErrInvalidToken
	}

	// 2. Check token revocation blocklist in Redis (if JTI is set)
	if claims.ID != "" && a.redis != nil {
		blockKey := redisBlocklistPrefix + claims.ID
		blocked, err := a.redis.Exists(ctx, blockKey).Result()
		if err == nil && blocked > 0 {
			return nil, platformauth.ErrRevokedToken
		}
	}

	// 3. Check identity cache in Redis to avoid a PostgreSQL lookup on every request
	userCacheKey := fmt.Sprintf("%s%s", redisUserCachePrefix, claims.UserID.String())
	if a.redis != nil {
		cachedBytes, err := a.redis.Get(ctx, userCacheKey).Bytes()
		if err == nil && len(cachedBytes) > 0 {
			var cached platformauth.AuthenticatedIdentity
			if jsonErr := json.Unmarshal(cachedBytes, &cached); jsonErr == nil {
				cached.TokenID = claims.ID
				if claims.ExpiresAt != nil {
					cached.ExpiresAt = claims.ExpiresAt.Time
				}
				return &cached, nil
			}
		}
	}

	// 4. Cache miss: Look up the user in PostgreSQL
	u, err := a.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, platformauth.ErrUserNotFound
		}
		a.logger.Error().Err(err).Str("user_id", claims.UserID.String()).Msg("error fetching user during auth")
		return nil, err
	}

	if !u.IsActive {
		return nil, platformauth.ErrUserDeactivated
	}

	// 5. Construct provider-independent AuthenticatedIdentity
	identity := &platformauth.AuthenticatedIdentity{
		UserID:    u.ID,
		Role:      platformauth.Role(u.Role),
		Email:     u.Email,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Provider:  "jwt",
		TokenID:   claims.ID,
	}
	if claims.ExpiresAt != nil {
		identity.ExpiresAt = claims.ExpiresAt.Time
	}

	// 6. Asynchronously or synchronously cache identity in Redis (graceful degradation)
	if a.redis != nil {
		if idBytes, mErr := json.Marshal(identity); mErr == nil {
			cacheTTL := defaultCacheTTL
			if claims.ExpiresAt != nil {
				remaining := time.Until(claims.ExpiresAt.Time)
				if remaining > 0 && remaining < cacheTTL {
					cacheTTL = remaining
				}
			}
			_ = a.redis.Set(ctx, userCacheKey, idBytes, cacheTTL).Err()
		}
	}

	return identity, nil
}

// RevokeToken adds an access token's JTI to the Redis blocklist for its remaining lifetime.
func (a *Adapter) RevokeToken(ctx context.Context, jti string, remainingTTL time.Duration) error {
	if a.redis == nil || jti == "" || remainingTTL <= 0 {
		return nil
	}
	return a.redis.Set(ctx, redisBlocklistPrefix+jti, "1", remainingTTL).Err()
}

// InvalidateUserCache deletes the user's cached identity from Redis.
func (a *Adapter) InvalidateUserCache(ctx context.Context, userID uuid.UUID) error {
	if a.redis == nil || userID == uuid.Nil {
		return nil
	}
	return a.redis.Del(ctx, fmt.Sprintf("%s%s", redisUserCachePrefix, userID.String())).Err()
}
