package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrRefreshTokenNotFound = errors.New("refresh token not found")
)

type Repository interface {
	CreateRefreshToken(ctx context.Context, token *RefreshToken) error
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*RefreshToken, error)
	UpdateRefreshTokenStatus(ctx context.Context, id uuid.UUID, status RefreshTokenStatus) error
	RevokeAllUserTokens(ctx context.Context, userID uuid.UUID) error
}

type pgRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{
		pool: pool,
	}
}

func (r *pgRepository) CreateRefreshToken(ctx context.Context, token *RefreshToken) error {
	if token.ID == uuid.Nil {
		token.ID = uuid.New()
	}
	now := time.Now().UTC()
	token.CreatedAt = now
	token.UpdatedAt = now
	if token.Status == "" {
		token.Status = RefreshTokenStatusActive
	}

	query := `
		INSERT INTO refresh_tokens (id, user_id, token_hash, status, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`
	return r.pool.QueryRow(ctx, query,
		token.ID,
		token.UserID,
		token.TokenHash,
		string(token.Status),
		token.ExpiresAt,
		token.CreatedAt,
		token.UpdatedAt,
	).Scan(&token.ID, &token.CreatedAt, &token.UpdatedAt)
}

func (r *pgRepository) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, status, expires_at, created_at, updated_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`
	var t RefreshToken
	var statusStr string
	err := r.pool.QueryRow(ctx, query, tokenHash).Scan(
		&t.ID,
		&t.UserID,
		&t.TokenHash,
		&statusStr,
		&t.ExpiresAt,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRefreshTokenNotFound
		}
		return nil, err
	}
	t.Status = RefreshTokenStatus(statusStr)
	return &t, nil
}

func (r *pgRepository) UpdateRefreshTokenStatus(ctx context.Context, id uuid.UUID, status RefreshTokenStatus) error {
	query := `
		UPDATE refresh_tokens
		SET status = $1, updated_at = NOW()
		WHERE id = $2
	`
	cmdTag, err := r.pool.Exec(ctx, query, string(status), id)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrRefreshTokenNotFound
	}
	return nil
}

func (r *pgRepository) RevokeAllUserTokens(ctx context.Context, userID uuid.UUID) error {
	query := `
		UPDATE refresh_tokens
		SET status = $1, updated_at = NOW()
		WHERE user_id = $2 AND status = $3
	`
	_, err := r.pool.Exec(ctx, query, string(RefreshTokenStatusRevoked), userID, string(RefreshTokenStatusActive))
	return err
}
