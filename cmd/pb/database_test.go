package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

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
