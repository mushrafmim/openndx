package database

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestWrapUsesProvidedPoolWithoutPing(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	defer sqlDB.Close()

	db, err := Wrap(sqlDB)
	require.NoError(t, err)
	underlying, err := db.DB()
	require.NoError(t, err)
	require.Same(t, sqlDB, underlying)

	mock.ExpectExec("SELECT 1").WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, db.WithContext(context.Background()).Exec("SELECT 1").Error)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWrapRejectsNilPool(t *testing.T) {
	_, err := Wrap(nil)
	require.ErrorContains(t, err, "SQL database is required")
}
