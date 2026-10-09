package idpfactory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// setIDPEnv sets every IDP_* variable NewFromEnv reads, so each case starts
// from a known environment (t.Setenv restores the originals afterwards).
func setIDPEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, key := range []string{
		"IDP_BASE_URL", "IDP_JWKS_URL", "IDP_ISSUER", "IDP_TOKEN_URL",
		"IDP_CLIENT_ID", "IDP_CLIENT_SECRET", "IDP_SCOPE",
	} {
		t.Setenv(key, env[key])
	}
}

func TestNewFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name:    "missing all variables",
			env:     map[string]string{},
			wantErr: "failed to create IDP provider: missing required environment variables",
		},
		{
			name: "missing client credentials",
			env: map[string]string{
				"IDP_BASE_URL": "https://example.com",
			},
			wantErr: "failed to create IDP provider: missing required environment variables",
		},
		{
			name: "JWKS URL without issuer or token URL",
			env: map[string]string{
				"IDP_JWKS_URL":      "https://example.com/oauth2/jwks",
				"IDP_CLIENT_ID":     "client-id",
				"IDP_CLIENT_SECRET": "client-secret",
			},
			wantErr: "failed to create IDP provider: missing required environment variables",
		},
		{
			name: "base URL with scopes",
			env: map[string]string{
				"IDP_BASE_URL":      "https://api.asgardeo.io/t/testorg",
				"IDP_CLIENT_ID":     "test-client-id",
				"IDP_CLIENT_SECRET": "test-client-secret",
				"IDP_SCOPE":         "scope1 scope2 scope3",
			},
		},
		{
			name: "base URL with empty scopes",
			env: map[string]string{
				"IDP_BASE_URL":      "https://api.asgardeo.io/t/testorg",
				"IDP_CLIENT_ID":     "test-client-id",
				"IDP_CLIENT_SECRET": "test-client-secret",
			},
		},
		{
			name: "standard OIDC without base URL (issuer)",
			env: map[string]string{
				"IDP_JWKS_URL":      "https://example.com/oauth2/jwks",
				"IDP_ISSUER":        "https://example.com",
				"IDP_CLIENT_ID":     "client-id",
				"IDP_CLIENT_SECRET": "client-secret",
			},
		},
		{
			name: "standard OIDC without base URL (token URL)",
			env: map[string]string{
				"IDP_JWKS_URL":      "https://example.com/oauth2/jwks",
				"IDP_TOKEN_URL":     "https://example.com/oauth2/token",
				"IDP_CLIENT_ID":     "client-id",
				"IDP_CLIENT_SECRET": "client-secret",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setIDPEnv(t, tt.env)

			provider, err := NewFromEnv()
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, provider)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, provider)
		})
	}
}
