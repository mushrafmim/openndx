package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	coredb "github.com/OpenNSW/core/database"
	"gorm.io/gorm"
)

// dbIdleTimeout is how long an idle pooled connection is kept.
const dbIdleTimeout = 30 * time.Minute

// databaseConfigFromEnv builds PB's PostgreSQL configuration from the DB_*
// environment variables. DB_PASSWORD and DB_NAME are required; the other
// settings default to a local server with TLS required.
func databaseConfigFromEnv() (coredb.Config, error) {
	port, err := strconv.Atoi(envOrDefault("DB_PORT", "5432"))
	if err != nil {
		return coredb.Config{}, fmt.Errorf("DB_PORT must be a number: %w", err)
	}
	cfg := coredb.Config{
		Driver: coredb.Postgres,
		Postgres: &coredb.PostgresConfig{
			Host:     envOrDefault("DB_HOST", "localhost"),
			Port:     port,
			User:     envOrDefault("DB_USERNAME", "postgres"),
			Password: os.Getenv("DB_PASSWORD"),
			Name:     os.Getenv("DB_NAME"),
			SSLMode:  envOrDefault("DB_SSLMODE", "require"),
			Pool: coredb.PoolConfig{
				MaxOpenConns:           25,
				MaxIdleConns:           5,
				MaxConnLifetimeSeconds: int(time.Hour / time.Second),
			},
		},
	}
	if cfg.Postgres.Password == "" {
		return coredb.Config{}, fmt.Errorf("DB_PASSWORD is required")
	}
	if cfg.Postgres.Name == "" {
		return coredb.Config{}, fmt.Errorf("DB_NAME is required")
	}
	if err := cfg.Validate(); err != nil {
		return coredb.Config{}, fmt.Errorf("invalid database configuration: %w", err)
	}
	return cfg, nil
}

func envOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// autoMigrate runs GORM auto-migration for the given models one at a time, in
// the order given. Callers must list referenced models before the models that
// reference them, so tables exist before GORM creates foreign keys to them.
func autoMigrate(db *gorm.DB, models ...any) error {
	slog.Info("Running GORM auto-migration")
	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return fmt.Errorf("failed to run auto-migration for %T: %w", m, err)
		}
	}
	slog.Info("GORM auto-migration completed successfully")
	return nil
}
