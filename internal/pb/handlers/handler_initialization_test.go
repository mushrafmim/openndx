package handlers

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/openndx/openndx-core/internal/pb/auth"
	"github.com/stretchr/testify/assert"
)

func TestNewV1Handler_MissingEnvVars(t *testing.T) {
	// Save current env vars
	originalBaseURL := os.Getenv("IDP_BASE_URL")
	originalClientID := os.Getenv("IDP_CLIENT_ID")
	originalClientSecret := os.Getenv("IDP_CLIENT_SECRET")
	originalJWKS := os.Getenv("IDP_JWKS_URL")
	originalIssuer := os.Getenv("IDP_ISSUER")
	originalTokenURL := os.Getenv("IDP_TOKEN_URL")
	originalPDPURLStd := os.Getenv("PDP_SERVICE_URL")

	// Restore env vars after test
	defer func() {
		if originalBaseURL != "" {
			os.Setenv("IDP_BASE_URL", originalBaseURL)
		} else {
			os.Unsetenv("IDP_BASE_URL")
		}
		if originalClientID != "" {
			os.Setenv("IDP_CLIENT_ID", originalClientID)
		} else {
			os.Unsetenv("IDP_CLIENT_ID")
		}
		if originalClientSecret != "" {
			os.Setenv("IDP_CLIENT_SECRET", originalClientSecret)
		} else {
			os.Unsetenv("IDP_CLIENT_SECRET")
		}
		if originalJWKS != "" {
			os.Setenv("IDP_JWKS_URL", originalJWKS)
		} else {
			os.Unsetenv("IDP_JWKS_URL")
		}
		if originalIssuer != "" {
			os.Setenv("IDP_ISSUER", originalIssuer)
		} else {
			os.Unsetenv("IDP_ISSUER")
		}
		if originalTokenURL != "" {
			os.Setenv("IDP_TOKEN_URL", originalTokenURL)
		} else {
			os.Unsetenv("IDP_TOKEN_URL")
		}
		if originalPDPURLStd != "" {
			os.Setenv("PDP_SERVICE_URL", originalPDPURLStd)
		} else {
			os.Unsetenv("PDP_SERVICE_URL")
		}
	}()

	// Unset env vars
	os.Unsetenv("IDP_BASE_URL")
	os.Unsetenv("IDP_CLIENT_ID")
	os.Unsetenv("IDP_CLIENT_SECRET")
	os.Unsetenv("IDP_JWKS_URL")
	os.Unsetenv("IDP_ISSUER")
	os.Unsetenv("IDP_TOKEN_URL")
	os.Unsetenv("PDP_SERVICE_URL")

	// Test missing IDP config (NewIdpAPIProvider fails)

	// We need a DB connection
	db := setupSQLiteTestDB(t)

	// Case 1: Missing IDP config (BaseURL)
	handler, err := NewV1Handler(db)
	assert.Error(t, err)
	assert.Nil(t, handler)
	assert.Contains(t, err.Error(), "failed to create IDP provider")

	// Set IDP config
	os.Setenv("IDP_BASE_URL", "https://example.com")
	os.Setenv("IDP_CLIENT_ID", "client-id")
	os.Setenv("IDP_CLIENT_SECRET", "client-secret")

	// Case 2: Missing PDP URL
	handler, err = NewV1Handler(db)
	assert.Error(t, err)
	assert.Nil(t, handler)
	assert.Contains(t, err.Error(), "PDP_SERVICE_URL environment variable not set")

	// Set PDP URL (standard)
	os.Setenv("PDP_SERVICE_URL", "http://pdp:8080")

	// Case 3: Success
	handler, err = NewV1Handler(db)
	assert.NoError(t, err)
	assert.NotNil(t, handler)
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

func TestNewV1Handler_StandardOIDC_WithoutBaseURL(t *testing.T) {
	// Save current env vars
	originalBaseURL := os.Getenv("IDP_BASE_URL")
	originalClientID := os.Getenv("IDP_CLIENT_ID")
	originalClientSecret := os.Getenv("IDP_CLIENT_SECRET")
	originalJWKS := os.Getenv("IDP_JWKS_URL")
	originalIssuer := os.Getenv("IDP_ISSUER")
	originalTokenURL := os.Getenv("IDP_TOKEN_URL")
	originalPDPURLStd := os.Getenv("PDP_SERVICE_URL")

	// Restore env vars after test
	defer func() {
		if originalBaseURL != "" {
			os.Setenv("IDP_BASE_URL", originalBaseURL)
		} else {
			os.Unsetenv("IDP_BASE_URL")
		}
		if originalClientID != "" {
			os.Setenv("IDP_CLIENT_ID", originalClientID)
		} else {
			os.Unsetenv("IDP_CLIENT_ID")
		}
		if originalClientSecret != "" {
			os.Setenv("IDP_CLIENT_SECRET", originalClientSecret)
		} else {
			os.Unsetenv("IDP_CLIENT_SECRET")
		}
		if originalJWKS != "" {
			os.Setenv("IDP_JWKS_URL", originalJWKS)
		} else {
			os.Unsetenv("IDP_JWKS_URL")
		}
		if originalIssuer != "" {
			os.Setenv("IDP_ISSUER", originalIssuer)
		} else {
			os.Unsetenv("IDP_ISSUER")
		}
		if originalTokenURL != "" {
			os.Setenv("IDP_TOKEN_URL", originalTokenURL)
		} else {
			os.Unsetenv("IDP_TOKEN_URL")
		}
		if originalPDPURLStd != "" {
			os.Setenv("PDP_SERVICE_URL", originalPDPURLStd)
		} else {
			os.Unsetenv("PDP_SERVICE_URL")
		}
	}()

	// Unset IDP_BASE_URL
	os.Unsetenv("IDP_BASE_URL")

	// Configure standard OIDC
	os.Setenv("IDP_JWKS_URL", "https://example.com/oauth2/jwks")
	os.Setenv("IDP_ISSUER", "https://example.com")
	os.Setenv("IDP_CLIENT_ID", "client-id")
	os.Setenv("IDP_CLIENT_SECRET", "client-secret")
	os.Setenv("PDP_SERVICE_URL", "http://pdp:8080")

	db := setupSQLiteTestDB(t)

	handler, err := NewV1Handler(db)
	assert.NoError(t, err)
	assert.NotNil(t, handler)
}
