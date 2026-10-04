package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX is an interface that abstracts over both pgxpool.Pool and pgx.Tx.
// It allows repositories to accept either a standalone connection or a transaction.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, arguments ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, arguments ...interface{}) pgx.Row
}

// TxManager provides transaction management capabilities.
type TxManager interface {
	// WithTx executes the given function within a database transaction.
	// If the function returns an error, the transaction is rolled back.
	// If the function succeeds, the transaction is committed.
	WithTx(ctx context.Context, fn func(tx DBTX) error) error
}

// WithTx executes the given function within a database transaction.
// It automatically handles committing or rolling back the transaction.
func (db *Database) WithTx(ctx context.Context, fn func(tx DBTX) error) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Defer rollback. It is a no-op if the transaction is already committed.
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
