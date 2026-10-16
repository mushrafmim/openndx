package auth

import (
	"context"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/openndx/openndx-core/internal/authn"
)

func TestGetUserFromRequest_MapsPrincipal(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/schemas", nil)
	req = req.WithContext(context.WithValue(req.Context(), authn.AuthContextKey, &authn.AuthContext{
		Principal: authn.Principal{
			Subject: "user-123",
			OUID:    "ou-123",
			Email:   "user@example.com",
			Roles:   []string{"OpenNDX_Member"},
			Scopes:  []string{ScopeSchemaRead, ScopeSchemaWrite},
		},
	}))

	user, err := GetUserFromRequest(req)
	if err != nil {
		t.Fatalf("GetUserFromRequest() error = %v", err)
	}
	if user.IdpUserID != "user-123" || user.OuID != "ou-123" || user.Email != "user@example.com" {
		t.Errorf("user = %+v, want subject, OU and email from the principal", user)
	}
	if !slices.Equal(user.Scopes, []string{ScopeSchemaRead, ScopeSchemaWrite}) {
		t.Errorf("Scopes = %v, want the principal's scopes", user.Scopes)
	}
}

func TestGetUserFromRequest_Unauthenticated(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/schemas", nil)
	if _, err := GetUserFromRequest(req); err == nil {
		t.Error("GetUserFromRequest() error = nil, want an error for a request without a principal")
	}
}

func TestAuthenticatedUser_HasScope(t *testing.T) {
	user := &AuthenticatedUser{Scopes: []string{ScopeSchemaRead, ScopeSchemaReadAll}}

	if !user.HasScope(ScopeSchemaRead) || !user.HasScope(ScopeSchemaReadAll) {
		t.Error("HasScope() = false for a granted scope")
	}
	if user.HasScope(ScopeSchemaWrite) {
		t.Error("HasScope(schema:write) = true, want false for a scope not granted")
	}
	if (&AuthenticatedUser{}).HasScope(ScopeSchemaRead) {
		t.Error("HasScope() = true for a user with no scopes")
	}
}
