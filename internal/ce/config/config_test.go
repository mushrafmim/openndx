package config

import (
	"flag"
	"testing"
	"time"

	"github.com/OpenNSW/core/database"
	"github.com/stretchr/testify/require"
)

func cleanConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"ENVIRONMENT", "PORT", "HOST", "DB_HOST", "DB_PORT", "DB_USERNAME", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE",
		"DB_MAX_OPEN_CONNS", "DB_MAX_IDLE_CONNS", "DB_CONN_MAX_LIFETIME_SECONDS", "DB_CONN_MAX_IDLE_TIME",
		"CONSENT_PORTAL_URL", "CORS_ALLOWED_ORIGINS", "IDP_ISSUER", "IDP_AUDIENCE", "IDP_JWKS_URL",
		"IDP_CLIENT_ID", "IDP_SUBJECT_CLAIM", "IDP_JWKS_INSECURE_SKIP_VERIFY",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("DB_PASSWORD", "test-password")
}

func TestLoadConfigBuildsValidatedGlobalDatabaseConfig(t *testing.T) {
	cleanConfigEnv(t)
	t.Setenv("DB_HOST", "db.example")
	t.Setenv("DB_PORT", "55432")
	t.Setenv("DB_USERNAME", "ce-user")
	t.Setenv("DB_PASSWORD", "p@ss:/?# word")
	t.Setenv("DB_NAME", "ce-db")
	t.Setenv("DB_SSLMODE", "disable")
	cfg, err := loadConfig("consent-engine", nil)
	require.NoError(t, err)
	require.Equal(t, database.Config{
		Driver: database.Postgres,
		Postgres: &database.PostgresConfig{
			Host: "db.example", Port: 55432, User: "ce-user", Password: "p@ss:/?# word",
			Name: "ce-db", SSLMode: "disable",
			Pool: database.PoolConfig{MaxOpenConns: 25, MaxIdleConns: 5, MaxConnLifetimeSeconds: 3600},
		},
	}, cfg.Database)
	require.Equal(t, 30*time.Minute, cfg.DatabaseIdleTimeout)
	require.NoError(t, cfg.Validate())
}

func TestLoadConfigAcceptsProvidedPoolSettingsAndFlagOverrides(t *testing.T) {
	cleanConfigEnv(t)
	t.Setenv("PORT", "8081")
	t.Setenv("DB_MAX_OPEN_CONNS", "12")
	t.Setenv("DB_MAX_IDLE_CONNS", "3")
	t.Setenv("DB_CONN_MAX_LIFETIME_SECONDS", "120")
	t.Setenv("DB_CONN_MAX_IDLE_TIME", "2m")
	for range 2 { // Repeated loading must not register duplicate global flags.
		cfg, err := loadConfig("consent-engine", []string{"-port", "9081", "-timeout", "3s"})
		require.NoError(t, err)
		require.Equal(t, "9081", cfg.Service.Port)
		require.Equal(t, 3*time.Second, cfg.Service.Timeout)
		require.Equal(t, database.PoolConfig{MaxOpenConns: 12, MaxIdleConns: 3, MaxConnLifetimeSeconds: 120}, cfg.Database.Postgres.Pool)
		require.Equal(t, 2*time.Minute, cfg.DatabaseIdleTimeout)
	}
}

func TestLoadConfigRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct{ key, value, message string }{
		{"DB_PORT", "abc", "DB_PORT must be an integer"},
		{"DB_PORT", "0", "DB_PORT must be between"},
		{"DB_PORT", "65536", "DB_PORT must be between"},
		{"DB_PASSWORD", "", "password is required"},
		{"DB_MAX_OPEN_CONNS", "0", "invalid database pool"},
		{"DB_MAX_IDLE_CONNS", "0", "invalid database pool"},
		{"DB_MAX_IDLE_CONNS", "26", "invalid database pool"},
		{"DB_CONN_MAX_LIFETIME_SECONDS", "-1", "invalid database pool"},
		{"DB_CONN_MAX_IDLE_TIME", "soon", "must be a duration"},
		{"DB_CONN_MAX_IDLE_TIME", "0s", "idle timeout must be positive"},
		{"PORT", "abc", "service port"},
		{"CONSENT_PORTAL_URL", "portal", "consent portal URL"},
		{"IDP_JWKS_INSECURE_SKIP_VERIFY", "sometimes", "must be a boolean"},
	} {
		t.Run(tc.key+"_"+tc.value, func(t *testing.T) {
			cleanConfigEnv(t)
			t.Setenv(tc.key, tc.value)
			_, err := loadConfig("consent-engine", nil)
			require.ErrorContains(t, err, tc.message)
		})
	}
}

func TestValidateUsesSuppliedConfig(t *testing.T) {
	cleanConfigEnv(t)
	cfg, err := loadConfig("consent-engine", nil)
	require.NoError(t, err)
	cfg.Database.Postgres.Host = ""
	require.ErrorContains(t, cfg.Validate(), "host is required")
	cfg.Database = database.Config{Driver: database.SQLite, SQLite: &database.SQLiteConfig{Path: ":memory:"}}
	require.ErrorContains(t, cfg.Validate(), "only PostgreSQL")
	var missing *Config
	require.ErrorContains(t, missing.Validate(), "configuration is required")
}

func TestLoadConfigReportsHelpAndUnknownFlags(t *testing.T) {
	cleanConfigEnv(t)
	_, err := loadConfig("consent-engine", []string{"-help"})
	require.ErrorIs(t, err, flag.ErrHelp)
	_, err = loadConfig("consent-engine", []string{"-unknown"})
	require.Error(t, err)
}
