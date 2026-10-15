package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openndx/openndx-core/internal/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withPrincipal puts p on the request the way authn's middleware does.
func withPrincipal(r *http.Request, p authn.Principal) *http.Request {
	ctx := context.WithValue(r.Context(), authn.AuthContextKey, &authn.AuthContext{Principal: p})
	return r.WithContext(ctx)
}

// serveFromAuthn runs r through FromAuthn and returns the response and the
// user the next handler saw, if it was reached.
func serveFromAuthn(r *http.Request) (*httptest.ResponseRecorder, *AuthenticatedUser) {
	var seen *AuthenticatedUser
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = GetAuthenticatedUser(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	FromAuthn(next).ServeHTTP(rec, r)
	return rec, seen
}

func TestFromAuthn_MapsPrincipalToUser(t *testing.T) {
	req := withPrincipal(httptest.NewRequest(http.MethodGet, "/api/v1/schemas", nil), authn.Principal{
		Subject:  "user-123",
		ClientID: "NDX_CLI",
		Roles:    []string{"OpenNDX_Member", "UnknownRole"},
		Email:    "member@example.com",
		OUID:     "ou-456",
	})

	rec, user := serveFromAuthn(req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, user)
	assert.Equal(t, "user-123", user.IdpUserID)
	assert.Equal(t, "ou-456", user.OuID)
	assert.Equal(t, "member@example.com", user.Email)
	assert.Equal(t, []Role{RoleMember}, user.Roles)
	assert.True(t, user.HasPermission(PermissionReadSchema))
}

func TestFromAuthn_AcceptsTokenWithoutEmail(t *testing.T) {
	req := withPrincipal(httptest.NewRequest(http.MethodGet, "/api/v1/schemas", nil), authn.Principal{
		Subject:  "system-client",
		ClientID: "PORTAL_BACKEND",
		Roles:    []string{"OpenNDX_System"},
	})

	rec, user := serveFromAuthn(req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, user)
	assert.Empty(t, user.Email)
	assert.True(t, user.IsSystem())
}

func TestFromAuthn_RejectsPrincipalWithoutValidRole(t *testing.T) {
	for name, roles := range map[string][]string{
		"no roles":      nil,
		"invalid roles": {"InvalidRole"},
	} {
		t.Run(name, func(t *testing.T) {
			req := withPrincipal(httptest.NewRequest(http.MethodGet, "/api/v1/schemas", nil), authn.Principal{
				Subject:  "user-123",
				ClientID: "NDX_CLI",
				Roles:    roles,
			})

			rec, user := serveFromAuthn(req)

			assert.Equal(t, http.StatusUnauthorized, rec.Code)
			assert.Nil(t, user)
		})
	}
}

func TestFromAuthn_RejectsRequestWithoutPrincipal(t *testing.T) {
	rec, user := serveFromAuthn(httptest.NewRequest(http.MethodGet, "/api/v1/schemas", nil))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Nil(t, user)
}
