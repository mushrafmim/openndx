package database

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	coredb "github.com/OpenNSW/core/database"
	"github.com/stretchr/testify/require"
)

func TestWrapGORMUsesProvidedPoolWithoutPing(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	defer sqlDB.Close()

	db, err := wrapGORM(sqlDB)
	require.NoError(t, err)
	underlying, err := db.DB()
	require.NoError(t, err)
	require.Same(t, sqlDB, underlying)

	mock.ExpectExec("SELECT 1").WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, db.WithContext(context.Background()).Exec("SELECT 1").Error)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWrapGORMRejectsNilPool(t *testing.T) {
	_, err := wrapGORM(nil)
	require.ErrorContains(t, err, "SQL database is required")
}

func TestInitializersRejectInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name        string
		cfg         coredb.Config
		idleTimeout time.Duration
		want        string
	}{
		{name: "unsupported driver", cfg: coredb.Config{Driver: coredb.MySQL}, idleTimeout: time.Minute, want: "only PostgreSQL"},
		{name: "missing postgres settings", cfg: coredb.Config{Driver: coredb.Postgres}, idleTimeout: time.Minute, want: "postgres config is required"},
		{name: "zero idle timeout", cfg: coredb.Config{Driver: coredb.Postgres}, want: "idle timeout must be positive"},
		{name: "negative idle timeout", cfg: coredb.Config{Driver: coredb.Postgres}, idleTimeout: -time.Minute, want: "idle timeout must be positive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sqlDB, err := Open(context.Background(), tc.cfg, tc.idleTimeout)
			require.ErrorContains(t, err, tc.want)
			require.Nil(t, sqlDB)
			gormDB, sqlDB, err := OpenGORM(context.Background(), tc.cfg, tc.idleTimeout)
			require.ErrorContains(t, err, tc.want)
			require.Nil(t, gormDB)
			require.Nil(t, sqlDB)
		})
	}
}
