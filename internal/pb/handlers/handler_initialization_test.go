package handlers

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/openndx/openndx-core/internal/pb/auth"
	"github.com/openndx/openndx-core/internal/pb/idp/idptest"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/stretchr/testify/assert"
)

// TestNewV1Handler checks that NewV1Handler wires the given dependencies.
// Environment parsing is covered by idpfactory.TestNewFromEnv and
// policy.TestNewClientFromEnv.
func TestNewV1Handler(t *testing.T) {
	db := setupSQLiteTestDB(t)
	resolver := &stubMemberResolver{}

	handler := NewV1Handler(db, &idptest.Mock{}, policy.NewClient("http://localhost:9999"), resolver)

	assert.NotNil(t, handler)
	assert.Same(t, resolver, handler.members)
	assert.NotNil(t, handler.schemaService)
	assert.NotNil(t, handler.applicationService)
}

// stubMemberResolver records the arguments it was called with
type stubMemberResolver struct {
	gotCtx  context.Context
	gotUser *auth.AuthenticatedUser
	id      string
	err     error
}

func (s *stubMemberResolver) ResolveMemberID(ctx context.Context, user *auth.AuthenticatedUser) (string, error) {
	s.gotCtx, s.gotUser = ctx, user
	return s.id, s.err
}

// TestGetUserMemberID_DelegatesToResolver checks that getUserMemberID passes the
// request context and user to the member resolver and returns its result.
// Lookup and caching behavior is covered by member.TestResolveMemberID_Caching.
func TestGetUserMemberID_DelegatesToResolver(t *testing.T) {
	user := &auth.AuthenticatedUser{IdpUserID: "test-user-id"}
	req := httptest.NewRequest("GET", "/", nil)

	t.Run("returns resolved ID", func(t *testing.T) {
		resolver := &stubMemberResolver{id: "mem_123"}
		h := &V1Handler{members: resolver}

		id, err := h.getUserMemberID(req, user)
		assert.NoError(t, err)
		assert.Equal(t, "mem_123", id)
		assert.Same(t, user, resolver.gotUser)
		assert.Equal(t, req.Context(), resolver.gotCtx)
	})

	t.Run("returns resolver error", func(t *testing.T) {
		resolver := &stubMemberResolver{err: errors.New("user member record not found")}
		h := &V1Handler{members: resolver}

		id, err := h.getUserMemberID(req, user)
		assert.EqualError(t, err, "user member record not found")
		assert.Empty(t, id)
	})
}
