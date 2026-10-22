package main

import (
	"testing"

	coredb "github.com/OpenNSW/core/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setDBEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, key := range []string{"DB_HOST", "DB_PORT", "DB_USERNAME", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE"} {
		t.Setenv(key, env[key])
	}
}

func TestDatabaseConfigFromEnv(t *testing.T) {
	t.Run("Applies defaults to unset settings", func(t *testing.T) {
		setDBEnv(t, map[string]string{"DB_PASSWORD": "secret", "DB_NAME": "portal_backend"})

		cfg, err := databaseConfigFromEnv()
		require.NoError(t, err)
		assert.Equal(t, coredb.Config{
			Driver: coredb.Postgres,
			Postgres: &coredb.PostgresConfig{
				Host: "localhost", Port: 5432, User: "postgres", Password: "secret",
				Name: "portal_backend", SSLMode: "require",
				Pool: coredb.PoolConfig{MaxOpenConns: 25, MaxIdleConns: 5, MaxConnLifetimeSeconds: 3600},
			},
		}, cfg)
	})

	t.Run("Reads every setting", func(t *testing.T) {
		setDBEnv(t, map[string]string{
			"DB_HOST": "db.internal", "DB_PORT": "5434", "DB_USERNAME": "pb",
			"DB_PASSWORD": "p@ss word'", "DB_NAME": "pb_db", "DB_SSLMODE": "disable",
		})

		cfg, err := databaseConfigFromEnv()
		require.NoError(t, err)
		pg := cfg.Postgres
		assert.Equal(t, "db.internal", pg.Host)
		assert.Equal(t, 5434, pg.Port)
		assert.Equal(t, "pb", pg.User)
		assert.Equal(t, "p@ss word'", pg.Password)
		assert.Equal(t, "pb_db", pg.Name)
		assert.Equal(t, "disable", pg.SSLMode)
	})

	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"Requires DB_PASSWORD", map[string]string{"DB_NAME": "pb_db"}, "DB_PASSWORD is required"},
		{"Requires DB_NAME", map[string]string{"DB_PASSWORD": "secret"}, "DB_NAME is required"},
		{"Rejects a non-numeric DB_PORT", map[string]string{"DB_PASSWORD": "secret", "DB_NAME": "pb_db", "DB_PORT": "abc"}, "DB_PORT must be a number"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setDBEnv(t, tc.env)

			_, err := databaseConfigFromEnv()
			require.ErrorContains(t, err, tc.want)
		})
	}
}

type testParent struct {
	ID string `gorm:"primarykey"`
}

type testChild struct {
	ID       string     `gorm:"primarykey"`
	ParentID string     `gorm:"not null"`
	Parent   testParent `gorm:"foreignKey:ParentID;references:ID"`
}

func newSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// A single connection keeps every query on the same in-memory database.
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func TestAutoMigrate(t *testing.T) {
	t.Run("Migrates models in the given order", func(t *testing.T) {
		db := newSQLiteDB(t)

		err := autoMigrate(db, &testParent{}, &testChild{})
		require.NoError(t, err)

		assert.True(t, db.Migrator().HasTable(&testParent{}))
		assert.True(t, db.Migrator().HasTable(&testChild{}))
	})

	t.Run("Returns error naming the failing model", func(t *testing.T) {
		db := newSQLiteDB(t)

		err := autoMigrate(db, &testParent{}, "not-a-model")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to run auto-migration for string")
		assert.True(t, db.Migrator().HasTable(&testParent{}))
	})
}
