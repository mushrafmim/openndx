package database

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	coredb "github.com/OpenNSW/core/database"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/openndx/openndx-core/internal/ce/config"
	"github.com/stretchr/testify/require"
)

func TestPostgresConnectionPool(t *testing.T) {
	dsn := os.Getenv("CE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CE_TEST_DATABASE_URL to run PostgreSQL connection tests")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.NotNil(t, parsed.User)
	password, _ := parsed.User.Password()
	require.NotEmpty(t, parsed.Path, "CE_TEST_DATABASE_URL must name a test database")
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	portNumber, err := strconv.Atoi(port)
	require.NoError(t, err)
	cfg := config.Config{
		Database: coredb.Config{
			Driver: coredb.Postgres,
			Postgres: &coredb.PostgresConfig{
				Host: parsed.Hostname(), Port: portNumber, User: parsed.User.Username(),
				Password: password, Name: parsed.Path[1:], SSLMode: parsed.Query().Get("sslmode"),
				Pool: coredb.PoolConfig{MaxOpenConns: 25, MaxIdleConns: 5, MaxConnLifetimeSeconds: 3600},
			},
		},
		DatabaseIdleTimeout: 20 * time.Millisecond,
		Service:             config.ServiceConfig{Port: "8081", Timeout: 10 * time.Second},
		ConsentPortalUrl:    "http://localhost:5173",
	}
	require.NoError(t, cfg.Validate())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := coredb.New(ctx, cfg.Database)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetConnMaxIdleTime(cfg.DatabaseIdleTimeout)
	wrapped, err := Wrap(db)
	require.NoError(t, err)
	underlying, err := wrapped.DB()
	require.NoError(t, err)
	require.Same(t, db, underlying)
	require.NoError(t, wrapped.WithContext(ctx).Exec("SELECT 1").Error)
	require.Equal(t, 25, db.Stats().MaxOpenConnections)
	require.NoError(t, db.PingContext(ctx))

	// Exercise the real pool's idle cap rather than just checking configuration fields.
	var release []func() error
	for range 6 {
		conn, err := db.Conn(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		release = append(release, conn.Close)
	}
	for _, closeConn := range release {
		require.NoError(t, closeConn())
	}
	require.LessOrEqual(t, db.Stats().Idle, 5)
	require.Eventually(t, func() bool { return db.Stats().MaxIdleTimeClosed > 0 }, 3*time.Second, 20*time.Millisecond)
	require.NoError(t, db.Close())
	require.Error(t, db.PingContext(ctx))
}
