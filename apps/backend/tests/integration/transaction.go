package integration

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type TxFn func(tx pgx.Tx) error

func WithTransaction(ctx context.Context, db *TestDB, fn TxFn) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func WithRollbackTransaction(ctx context.Context, db *TestDB, fn TxFn) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	return fn(tx)
}
