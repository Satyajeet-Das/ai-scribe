package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var (
	ErrSessionLocked = errors.New("session is currently locked by another operation")
	ErrCacheMiss     = errors.New("session not found in cache")
)

// Cache defines the interface for session runtime state and concurrency management.
type Cache interface {
	// AcquireLock attempts to acquire an exclusive lock for the given session.
	// It returns a function to release the lock, and an error if the lock cannot be acquired.
	AcquireLock(ctx context.Context, sessionID uuid.UUID, ttl time.Duration) (func(context.Context) error, error)

	// SetState caches the session state.
	SetState(ctx context.Context, sessionID uuid.UUID, session *Session, ttl time.Duration) error

	// GetState retrieves the cached session state.
	GetState(ctx context.Context, sessionID uuid.UUID) (*Session, error)

	// DeleteState removes the session from the cache.
	DeleteState(ctx context.Context, sessionID uuid.UUID) error
}

type redisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new Redis-backed session cache.
func NewRedisCache(client *redis.Client) Cache {
	return &redisCache{
		client: client,
	}
}

func lockKey(sessionID uuid.UUID) string {
	return fmt.Sprintf("session:lock:%s", sessionID.String())
}

func stateKey(sessionID uuid.UUID) string {
	return fmt.Sprintf("session:state:%s", sessionID.String())
}

func (c *redisCache) AcquireLock(ctx context.Context, sessionID uuid.UUID, ttl time.Duration) (func(context.Context) error, error) {
	key := lockKey(sessionID)
	
	// Retry loop: try to acquire the lock for up to ~3 seconds
	maxRetries := 30
	baseDelay := 50 * time.Millisecond
	
	for i := 0; i < maxRetries; i++ {
		ok, err := c.client.SetNX(ctx, key, sessionID.String(), ttl).Result()
		if err != nil {
			return nil, fmt.Errorf("redis setnx error: %w", err)
		}
		if ok {
			unlockFn := func(unlockCtx context.Context) error {
				return c.client.Del(unlockCtx, key).Err()
			}
			return unlockFn, nil
		}
		
		// If context is canceled, abort early
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(baseDelay):
			// Wait and retry
			// Increase delay slightly (jitter/backoff)
			baseDelay = baseDelay + 10*time.Millisecond
		}
	}
	
	return nil, ErrSessionLocked
}

func (c *redisCache) SetState(ctx context.Context, sessionID uuid.UUID, session *Session, ttl time.Duration) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	err = c.client.Set(ctx, stateKey(sessionID), data, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set session in redis: %w", err)
	}

	return nil
}

func (c *redisCache) GetState(ctx context.Context, sessionID uuid.UUID) (*Session, error) {
	data, err := c.client.Get(ctx, stateKey(sessionID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("failed to get session from redis: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

func (c *redisCache) DeleteState(ctx context.Context, sessionID uuid.UUID) error {
	err := c.client.Del(ctx, stateKey(sessionID)).Err()
	if err != nil {
		return fmt.Errorf("failed to delete session from redis: %w", err)
	}
	return nil
}
