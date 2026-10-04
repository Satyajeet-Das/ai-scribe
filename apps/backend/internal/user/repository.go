package user

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByEmailWithPassword(ctx context.Context, email string) (*User, error)
	GetByClerkID(ctx context.Context, clerkID string) (*User, error)
	GetByRollNo(ctx context.Context, rollNo string) (*User, error)
	SearchStudents(ctx context.Context, query string, limit int) ([]User, error)
	Create(ctx context.Context, u *User) error
}

type pgRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{
		pool: pool,
	}
}

func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	query := `
		SELECT id, clerk_id, email, password_hash, first_name, last_name, role, roll_no, is_active, created_at, updated_at
		FROM users
		WHERE id = $1
	`
	var u User
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&u.ID,
		&u.ClerkID,
		&u.Email,
		&u.PasswordHash,
		&u.FirstName,
		&u.LastName,
		&u.Role,
		&u.RollNo,
		&u.IsActive,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *pgRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, clerk_id, email, password_hash, first_name, last_name, role, roll_no, is_active, created_at, updated_at
		FROM users
		WHERE LOWER(email) = LOWER($1)
	`
	var u User
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&u.ID,
		&u.ClerkID,
		&u.Email,
		&u.PasswordHash,
		&u.FirstName,
		&u.LastName,
		&u.Role,
		&u.RollNo,
		&u.IsActive,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *pgRepository) GetByEmailWithPassword(ctx context.Context, email string) (*User, error) {
	return r.GetByEmail(ctx, email)
}

func (r *pgRepository) GetByClerkID(ctx context.Context, clerkID string) (*User, error) {
	query := `
		SELECT id, clerk_id, email, password_hash, first_name, last_name, role, roll_no, is_active, created_at, updated_at
		FROM users
		WHERE clerk_id = $1
	`
	var u User
	err := r.pool.QueryRow(ctx, query, clerkID).Scan(
		&u.ID,
		&u.ClerkID,
		&u.Email,
		&u.PasswordHash,
		&u.FirstName,
		&u.LastName,
		&u.Role,
		&u.RollNo,
		&u.IsActive,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *pgRepository) GetByRollNo(ctx context.Context, rollNo string) (*User, error) {
	query := `
		SELECT id, clerk_id, email, password_hash, first_name, last_name, role, roll_no, is_active, created_at, updated_at
		FROM users
		WHERE LOWER(roll_no) = LOWER($1)
	`
	var u User
	err := r.pool.QueryRow(ctx, query, strings.TrimSpace(rollNo)).Scan(
		&u.ID,
		&u.ClerkID,
		&u.Email,
		&u.PasswordHash,
		&u.FirstName,
		&u.LastName,
		&u.Role,
		&u.RollNo,
		&u.IsActive,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *pgRepository) SearchStudents(ctx context.Context, query string, limit int) ([]User, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	trimmed := strings.TrimSpace(query)
	var rows pgx.Rows
	var err error

	if trimmed == "" {
		sqlQuery := `
			SELECT id, clerk_id, email, password_hash, first_name, last_name, role, roll_no, is_active, created_at, updated_at
			FROM users
			WHERE role = 'STUDENT' AND is_active = true
			ORDER BY roll_no ASC, first_name ASC
			LIMIT $1
		`
		rows, err = r.pool.Query(ctx, sqlQuery, limit)
	} else {
		pattern := "%" + trimmed + "%"
		prefix := trimmed + "%"
		sqlQuery := `
			SELECT id, clerk_id, email, password_hash, first_name, last_name, role, roll_no, is_active, created_at, updated_at
			FROM users
			WHERE role = 'STUDENT'
			  AND is_active = true
			  AND (
			    roll_no ILIKE $1
			    OR first_name ILIKE $1
			    OR last_name ILIKE $1
			    OR email ILIKE $1
			    OR (first_name || ' ' || last_name) ILIKE $1
			  )
			ORDER BY
			  CASE
			    WHEN roll_no ILIKE $2 THEN 1
			    WHEN roll_no ILIKE $1 THEN 2
			    WHEN (first_name || ' ' || last_name) ILIKE $2 THEN 3
			    ELSE 4
			  END,
			  roll_no ASC,
			  first_name ASC
			LIMIT $3
		`
		rows, err = r.pool.Query(ctx, sqlQuery, pattern, prefix, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(
			&u.ID,
			&u.ClerkID,
			&u.Email,
			&u.PasswordHash,
			&u.FirstName,
			&u.LastName,
			&u.Role,
			&u.RollNo,
			&u.IsActive,
			&u.CreatedAt,
			&u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

func (r *pgRepository) Create(ctx context.Context, u *User) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now

	query := `
		INSERT INTO users (id, clerk_id, email, password_hash, first_name, last_name, role, roll_no, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at, updated_at
	`
	err := r.pool.QueryRow(ctx, query,
		u.ID,
		u.ClerkID,
		u.Email,
		u.PasswordHash,
		u.FirstName,
		u.LastName,
		u.Role,
		u.RollNo,
		u.IsActive,
		u.CreatedAt,
		u.UpdatedAt,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if strings.Contains(strings.ToLower(pgErr.ConstraintName), "roll_no") {
				return ErrRollNoAlreadyExists
			}
			return ErrUserAlreadyExists
		}
		return err
	}
	return nil
}
