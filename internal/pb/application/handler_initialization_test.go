package application

import (
	"testing"

	"github.com/openndx/openndx-core/internal/pb/idp/idptest"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/stretchr/testify/assert"
)

// TestNewHandler checks that NewHandler wires the given dependencies.
// Environment parsing is covered by idpfactory.TestNewFromEnv and
// policy.TestNewClientFromEnv.
func TestNewHandler(t *testing.T) {
	db := setupSQLiteTestDB(t)
	service := NewService(db, policy.NewClient("http://localhost:9999"), &idptest.Mock{})

	handler := NewHandler(service)

	assert.NotNil(t, handler)
	assert.Same(t, service, handler.service)
}
