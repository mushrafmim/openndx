package configs

import (
	"fmt"
	"os"
	"strconv"
	"time"

	coredb "github.com/OpenNSW/core/database"
)

// DatabaseFromEnv reads the schema database connection settings from environment variables.
// It returns the connection configuration and the pool's idle-connection timeout.
func DatabaseFromEnv() (coredb.Config, time.Duration, error) {
	port, err := intEnv("DB_PORT", "5432")
	if err != nil {
		return coredb.Config{}, 0, err
	}
	maxOpen, err := intEnv("DB_MAX_OPEN_CONNS", "25")
	if err != nil {
		return coredb.Config{}, 0, err
	}
	maxIdle, err := intEnv("DB_MAX_IDLE_CONNS", "5")
	if err != nil {
		return coredb.Config{}, 0, err
	}
	lifetime, err := intEnv("DB_CONN_MAX_LIFETIME_SECONDS", "3600")
	if err != nil {
		return coredb.Config{}, 0, err
	}
	idleTimeout, err := time.ParseDuration(getEnv("DB_CONN_MAX_IDLE_TIME", "30m"))
	if err != nil {
		return coredb.Config{}, 0, fmt.Errorf("DB_CONN_MAX_IDLE_TIME must be a duration: %w", err)
	}

	cfg := coredb.Config{
		Driver: coredb.Postgres,
		Postgres: &coredb.PostgresConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     port,
			User:     getEnv("DB_USERNAME", "postgres"),
			Password: getEnv("DB_PASSWORD", ""),
			Name:     getEnv("DB_NAME", "orchestration_engine"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
			Pool: coredb.PoolConfig{
				MaxOpenConns: maxOpen, MaxIdleConns: maxIdle, MaxConnLifetimeSeconds: lifetime,
			},
		},
	}
	return cfg, idleTimeout, nil
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func intEnv(name, defaultValue string) (int, error) {
	value, err := strconv.Atoi(getEnv(name, defaultValue))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return value, nil
}
