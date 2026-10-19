package database

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // Same driver internal/database.Open uses.
	"github.com/stretchr/testify/require"
)

// TestSchemaDBPostgres verifies the schema store against a real PostgreSQL server
// through the pgx driver used by internal/database.Open.
func TestSchemaDBPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL schema store tests")
	}
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("DROP TABLE IF EXISTS unified_schemas")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE IF EXISTS unified_schemas") })

	store, err := NewSchemaDB(db)
	require.NoError(t, err)
	_, err = NewSchemaDB(db)
	require.NoError(t, err, "creating the tables must be idempotent")

	active, err := store.GetActiveSchema()
	require.NoError(t, err)
	require.Nil(t, active)

	for _, version := range []string{"v1", "v2"} {
		require.NoError(t, store.CreateSchema(&Schema{
			ID: "id-" + version, Version: version, SDL: "type Query { " + version + ": String }",
			Status: "draft", CreatedBy: "test", Checksum: "sum-" + version,
		}))
	}
	require.NoError(t, store.ActivateSchema("v2"))
	require.Error(t, store.ActivateSchema("missing"))

	active, err = store.GetActiveSchema()
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, "v2", active.Version)
	require.Equal(t, "type Query { v2: String }", active.SDL)

	got, err := store.GetSchemaByVersion("v1")
	require.NoError(t, err)
	require.False(t, got.IsActive)

	all, err := store.GetAllSchemas()
	require.NoError(t, err)
	require.Len(t, all, 2)
}

func TestNewSchemaDBRejectsNilPool(t *testing.T) {
	_, err := NewSchemaDB(nil)
	require.EqualError(t, err, "SQL database is required")
}
