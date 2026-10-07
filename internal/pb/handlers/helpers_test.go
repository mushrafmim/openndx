package handlers

import (
	"net/http"
	"testing"

	"github.com/openndx/openndx-core/internal/pb/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupSQLiteTestDB creates an in-memory SQLite database with all Portal
// Backend models migrated.
func setupSQLiteTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("Failed to connect to SQLite test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.Member{},
		&models.Application{},
		&models.ApplicationSubmission{},
		&models.Schema{},
		&models.SchemaSubmission{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	return db
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(req *http.Request) (*http.Response, error)

// RoundTrip calls f(req).
func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
