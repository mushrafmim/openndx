package idpfactory

import (
	"fmt"
	"os"
	"strings"

	"github.com/openndx/openndx-core/internal/pb/idp"
	"github.com/openndx/openndx-core/internal/pb/idp/thunderid"
)

// NewFromEnv creates the Portal Backend's ThunderID management client from the
// IDP_* environment variables. IDP_BASE_URL falls back to IDP_ISSUER (or
// IDP_TOKEN_URL) when it is unset and IDP_JWKS_URL is configured, so a
// standard OIDC setup works without a base URL.
//
// IDP_TOKEN_RESOURCE is the System resource server identifier the `system`
// scope is requested for, and IDP_DATA_CONSUMER_ROLE_ID is the role every
// created application is assigned.
func NewFromEnv() (idp.ApplicationManager, error) {
	// Get scopes from environment variable, fallback to default if not set
	scopesEnv := os.Getenv("IDP_SCOPE")
	var scopes []string
	if scopesEnv != "" {
		// Split by space to handle multiple scopes
		scopes = strings.Fields(scopesEnv)
	}
	// Create the NewIdpProvider
	baseURL := os.Getenv("IDP_BASE_URL")
	jwksURL := os.Getenv("IDP_JWKS_URL")
	issuerURL := os.Getenv("IDP_ISSUER")
	tokenURL := os.Getenv("IDP_TOKEN_URL")

	if baseURL == "" {
		if jwksURL != "" && (issuerURL != "" || tokenURL != "") {
			if issuerURL != "" {
				baseURL = issuerURL
			} else {
				baseURL = tokenURL
			}
		}
	}

	clientID := os.Getenv("IDP_CLIENT_ID")
	clientSecret := os.Getenv("IDP_CLIENT_SECRET")

	if baseURL == "" || clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("failed to create IDP provider: missing required environment variables (IDP_BASE_URL, or IDP_JWKS_URL and IDP_ISSUER/IDP_TOKEN_URL, along with IDP_CLIENT_ID and IDP_CLIENT_SECRET)")
	}

	return thunderid.NewClient(thunderid.Config{
		BaseURL:            baseURL,
		ClientID:           clientID,
		ClientSecret:       clientSecret,
		Scopes:             scopes,
		Resource:           os.Getenv("IDP_TOKEN_RESOURCE"),
		DataConsumerRoleID: os.Getenv("IDP_DATA_CONSUMER_ROLE_ID"),
	}), nil
}
