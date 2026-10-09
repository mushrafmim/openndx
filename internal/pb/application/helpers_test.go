package application

import (
	"net/http"
	"testing"

	"github.com/openndx/openndx-core/internal/pb/database/dbtest"
	"github.com/openndx/openndx-core/internal/pb/member"
	"github.com/openndx/openndx-core/internal/pb/schema"
	"gorm.io/gorm"
)

// setupSQLiteTestDB creates an in-memory SQLite database with all Portal
// Backend models migrated.
func setupSQLiteTestDB(t *testing.T) *gorm.DB {
	return dbtest.SetupSQLiteDB(t,
		&member.Member{},
		&Application{},
		&ApplicationSubmission{},
		&schema.Schema{},
		&schema.SchemaSubmission{},
	)
}

// MockRoundTripper is a mock implementation of http.RoundTripper
type MockRoundTripper struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

// RoundTrip executes the mock RoundTripFunc
func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}
