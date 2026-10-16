// Package authtest provides authenticated test personas and request helpers
// for Portal Backend handler tests. Import it only from _test.go files.
package authtest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/pb/auth"
)

// TestUser is an authenticated caller with the scopes its token was granted.
// For testing authentication failures (no caller in context), use
// NewUnauthenticatedRequest instead.
type TestUser struct {
	IdpUserID string
	OuID      string
	Email     string
	Scopes    []string
}

// MemberScopes are the scopes the OpenNDX_Member role grants: read and write
// within the member's own organization unit.
var MemberScopes = []string{
	auth.ScopeMemberRead, auth.ScopeMemberWrite,
	auth.ScopeSchemaRead, auth.ScopeSchemaWrite,
	auth.ScopeSchemaSubmissionRead, auth.ScopeSchemaSubmissionWrite,
	auth.ScopeApplicationRead, auth.ScopeApplicationWrite,
	auth.ScopeApplicationSubmissionRead, auth.ScopeApplicationSubmissionWrite,
}

// ReadAllScopes are read access across every organization unit and nothing else.
var ReadAllScopes = []string{
	auth.ScopeMemberReadAll,
	auth.ScopeSchemaReadAll,
	auth.ScopeSchemaSubmissionReadAll,
	auth.ScopeApplicationReadAll,
	auth.ScopeApplicationSubmissionReadAll,
}

// AdminScopes are the scopes the OpenNDX_Admin role grants: every PB scope.
var AdminScopes = auth.AllScopes

// Predefined test users for testing authorization scenarios
var (
	// AdminUser holds every PB scope
	AdminUser = TestUser{
		IdpUserID: "admin-test-user-123",
		OuID:      "admin-test-ou-123",
		Email:     "admin@test.com",
		Scopes:    AdminScopes,
	}

	// MemberUser holds the own-organization-unit scopes
	MemberUser = TestUser{
		IdpUserID: "member-test-user-456",
		OuID:      "member-test-ou-456",
		Email:     "member@test.com",
		Scopes:    MemberScopes,
	}

	// ReadOnlyUser can read across organization units but write nothing
	ReadOnlyUser = TestUser{
		IdpUserID: "readonly-test-user-789",
		OuID:      "readonly-test-ou-789",
		Email:     "readonly@test.com",
		Scopes:    ReadAllScopes,
	}
)

// WithAuth returns req carrying testUser as the principal internal/authn
// would have validated.
func WithAuth(req *http.Request, testUser TestUser) *http.Request {
	ctx := context.WithValue(req.Context(), authn.AuthContextKey, &authn.AuthContext{
		Principal: authn.Principal{
			Subject: testUser.IdpUserID,
			OUID:    testUser.OuID,
			Email:   testUser.Email,
			Scopes:  testUser.Scopes,
		},
	})
	return req.WithContext(ctx)
}

// NewAuthenticatedRequest creates a new HTTP request with authentication context
func NewAuthenticatedRequest(method, url string, body io.Reader, testUser TestUser) *http.Request {
	req := httptest.NewRequest(method, url, body)
	return WithAuth(req, testUser)
}

// NewAdminRequest creates a new HTTP request authenticated as AdminUser
func NewAdminRequest(method, url string, body io.Reader) *http.Request {
	return NewAuthenticatedRequest(method, url, body, AdminUser)
}

// NewMemberRequest creates a new HTTP request authenticated as MemberUser
func NewMemberRequest(method, url string, body io.Reader) *http.Request {
	return NewAuthenticatedRequest(method, url, body, MemberUser)
}

// NewReadOnlyRequest creates a new HTTP request authenticated as ReadOnlyUser
func NewReadOnlyRequest(method, url string, body io.Reader) *http.Request {
	return NewAuthenticatedRequest(method, url, body, ReadOnlyUser)
}

// NewUnauthenticatedRequest creates a new HTTP request without authentication context.
// Use this to test authentication failure scenarios where no authenticated user exists.
// This simulates a request from a user who hasn't provided valid JWT credentials.
func NewUnauthenticatedRequest(method, url string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, url, body)
	// Intentionally don't add authentication context to simulate authentication failure
	return req
}

// CreateCustomTestUser creates a test user in the given organization unit
// with the given scopes
func CreateCustomTestUser(idpUserID, email, ouID string, scopes []string) TestUser {
	return TestUser{
		IdpUserID: idpUserID,
		OuID:      ouID,
		Email:     email,
		Scopes:    scopes,
	}
}
