package configs

import (
	"testing"
	"time"

	coredb "github.com/OpenNSW/core/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var databaseEnvVars = []string{
	"DB_HOST", "DB_PORT", "DB_USERNAME", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE",
	"DB_MAX_OPEN_CONNS", "DB_MAX_IDLE_CONNS", "DB_CONN_MAX_LIFETIME_SECONDS", "DB_CONN_MAX_IDLE_TIME",
}

func clearDatabaseEnv(t *testing.T) {
	for _, name := range databaseEnvVars {
		t.Setenv(name, "")
	}
}

func TestDatabaseFromEnv(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		clearDatabaseEnv(t)

		cfg, idleTimeout, err := DatabaseFromEnv()
		require.NoError(t, err)
		assert.Equal(t, coredb.Postgres, cfg.Driver)
		assert.Equal(t, &coredb.PostgresConfig{
			Host: "localhost", Port: 5432, User: "postgres", Password: "",
			Name: "orchestration_engine", SSLMode: "disable",
			Pool: coredb.PoolConfig{MaxOpenConns: 25, MaxIdleConns: 5, MaxConnLifetimeSeconds: 3600},
		}, cfg.Postgres)
		assert.Equal(t, 30*time.Minute, idleTimeout)
	})

	t.Run("environment variables set", func(t *testing.T) {
		clearDatabaseEnv(t)
		t.Setenv("DB_HOST", "standard-host")
		t.Setenv("DB_PORT", "5434")
		t.Setenv("DB_USERNAME", "standard-user")
		t.Setenv("DB_PASSWORD", "standard-password")
		t.Setenv("DB_NAME", "standard-db")
		t.Setenv("DB_SSLMODE", "prefer")
		t.Setenv("DB_MAX_OPEN_CONNS", "10")
		t.Setenv("DB_MAX_IDLE_CONNS", "2")
		t.Setenv("DB_CONN_MAX_LIFETIME_SECONDS", "60")
		t.Setenv("DB_CONN_MAX_IDLE_TIME", "5m")

		cfg, idleTimeout, err := DatabaseFromEnv()
		require.NoError(t, err)
		assert.Equal(t, &coredb.PostgresConfig{
			Host: "standard-host", Port: 5434, User: "standard-user", Password: "standard-password",
			Name: "standard-db", SSLMode: "prefer",
			Pool: coredb.PoolConfig{MaxOpenConns: 10, MaxIdleConns: 2, MaxConnLifetimeSeconds: 60},
		}, cfg.Postgres)
		assert.Equal(t, 5*time.Minute, idleTimeout)
	})

	t.Run("rejects a non-integer port", func(t *testing.T) {
		clearDatabaseEnv(t)
		t.Setenv("DB_PORT", "not-a-port")

		_, _, err := DatabaseFromEnv()
		assert.EqualError(t, err, "DB_PORT must be an integer")
	})

	t.Run("rejects an invalid idle time", func(t *testing.T) {
		clearDatabaseEnv(t)
		t.Setenv("DB_CONN_MAX_IDLE_TIME", "soon")

		_, _, err := DatabaseFromEnv()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DB_CONN_MAX_IDLE_TIME must be a duration")
	})
}
