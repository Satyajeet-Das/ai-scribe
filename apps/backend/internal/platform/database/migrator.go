package database

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/jackc/pgx/v5"
	tern "github.com/jackc/tern/v2/migrate"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/migrations"
)

func Migrate(ctx context.Context, logger *zerolog.Logger, cfg *config.Config) error {
	hostPort := net.JoinHostPort(cfg.Database.Host, strconv.Itoa(cfg.Database.Port))
	encodedPassword := url.QueryEscape(cfg.Database.Password)
	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s",
		cfg.Database.User,
		encodedPassword,
		hostPort,
		cfg.Database.Name,
		cfg.Database.SSLMode,
	)

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connecting for database migration: %w", err)
	}
	defer conn.Close(ctx)

	m, err := tern.NewMigrator(ctx, conn, "schema_version")
	if err != nil {
		return fmt.Errorf("constructing database migrator: %w", err)
	}

	if err := m.LoadMigrations(migrations.FS); err != nil {
		return fmt.Errorf("loading database migrations: %w", err)
	}

	from, err := m.GetCurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("retrieving current database migration version: %w", err)
	}

	if err := m.Migrate(ctx); err != nil {
		return fmt.Errorf("applying database migrations: %w", err)
	}

	if from == int32(len(m.Migrations)) {
		logger.Info().Msgf("database schema up to date, version %d", len(m.Migrations))
	} else {
		logger.Info().Msgf("migrated database schema, from version %d to %d", from, len(m.Migrations))
	}
	return nil
}
