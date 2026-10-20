package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/oe/auth"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testIssuer   = "https://idp.test/oauth2/token"
	testAudience = "http://api.openndx.local"
	testKid      = "test-kid"
)

// stubFederator is a QueryFederator that records the consumer it was called for.
type stubFederator struct {
	called   bool
	consumer *auth.ConsumerAssertion
}

func (s *stubFederator) FederateQuery(_ context.Context, _ graphql.Request, consumer *auth.ConsumerAssertion) graphql.Response {
	s.called = true
	s.consumer = consumer
	return graphql.Response{Data: map[string]interface{}{"ok": true}}
}

// testIdP signs consumer tokens and serves its public key as a JWKS, like the IdP OE trusts.
type testIdP struct {
	key  *rsa.PrivateKey
	jwks *httptest.Server
}

func newTestIdP(t *testing.T) *testIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kid": testKid,
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}},
		})
	}))
	t.Cleanup(jwks.Close)
	return &testIdP{key: key, jwks: jwks}
}

// claims returns valid consumer token claims for clientID, which tests then alter.
func (idp *testIdP) claims(clientID string) jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"iss":       testIssuer,
		"aud":       testAudience,
		"sub":       clientID,
		"client_id": clientID,
		"iat":       now.Unix(),
		"exp":       now.Add(time.Hour).Unix(),
	}
}

func (idp *testIdP) sign(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testKid
	signed, err := token.SignedString(idp.key)
	require.NoError(t, err)
	return signed
}

// newTestDeps returns server dependencies with no schema database, authenticating
// consumers with internal/authn against idp, the way cmd/oe/main.go does.
func newTestDeps(t *testing.T, idp *testIdP) Deps {
	t.Helper()
	manager, err := authn.NewManager(authn.Config{
		JWKSURL:  idp.jwks.URL,
		Issuer:   testIssuer,
		Audience: testAudience,
	})
	require.NoError(t, err)
	return Deps{
		Federator:    &stubFederator{},
		Authenticate: manager.RequireAuthMiddleware(),
	}
}

func postGraphQL(mux http.Handler, body []byte, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/public/graphql", bytes.NewBuffer(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestSetupRouter_Health(t *testing.T) {
	mux := SetupRouter(newTestDeps(t, newTestIdP(t)))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "OpenNDX Server is Healthy!")
}

// TestSetupRouter_NoSchemaManagementRoutes checks that the removed /sdl routes
// are not served: the consumer schema comes only from the schema file.
func TestSetupRouter_NoSchemaManagementRoutes(t *testing.T) {
	mux := SetupRouter(newTestDeps(t, newTestIdP(t)))

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/sdl"},
		{http.MethodPost, "/sdl"},
		{http.MethodGet, "/sdl/versions"},
		{http.MethodPost, "/sdl/validate"},
		{http.MethodPost, "/sdl/check-compatibility"},
		{http.MethodPost, "/sdl/versions/1.0.0/activate"},
	} {
		req := httptest.NewRequest(route.method, route.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code, "%s %s", route.method, route.path)
	}
}

func TestSetupRouter_PublicGraphQL(t *testing.T) {
	idp := newTestIdP(t)
	body, err := json.Marshal(graphql.Request{Query: "{ hello }"})
	require.NoError(t, err)

	t.Run("federates the query for the token's client_id", func(t *testing.T) {
		deps := newTestDeps(t, idp)
		federator := deps.Federator.(*stubFederator)
		claims := idp.claims("app-1")
		claims["sub"] = "service-account-1"

		w := postGraphQL(SetupRouter(deps), body, idp.sign(t, claims))

		assert.Equal(t, http.StatusOK, w.Code)
		require.True(t, federator.called)
		assert.Equal(t, &auth.ConsumerAssertion{
			ApplicationID: "app-1",
			ClientID:      "app-1",
			Subscriber:    "service-account-1",
		}, federator.consumer)
	})

	t.Run("rejects invalid JSON from an authenticated consumer", func(t *testing.T) {
		deps := newTestDeps(t, idp)

		w := postGraphQL(SetupRouter(deps), []byte("invalid-json"), idp.sign(t, idp.claims("app-1")))

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.False(t, deps.Federator.(*stubFederator).called)
	})

	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, idp.claims("app-1")).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)
	otherIdP := newTestIdP(t)

	for name, token := range map[string]string{
		"a missing token":               "",
		"an unsigned token":             unsigned,
		"a token signed by another key": otherIdP.sign(t, idp.claims("app-1")),
		"a token for another audience": idp.sign(t, func() jwt.MapClaims {
			c := idp.claims("app-1")
			c["aud"] = "http://pb.openndx.local"
			return c
		}()),
		"a token from another issuer": idp.sign(t, func() jwt.MapClaims {
			c := idp.claims("app-1")
			c["iss"] = "https://other-idp.test/oauth2/token"
			return c
		}()),
		"an expired token": idp.sign(t, func() jwt.MapClaims {
			c := idp.claims("app-1")
			c["exp"] = time.Now().Add(-time.Hour).Unix()
			return c
		}()),
		"a token without sub": idp.sign(t, func() jwt.MapClaims {
			c := idp.claims("app-1")
			delete(c, "sub")
			return c
		}()),
		"a token without client_id": idp.sign(t, func() jwt.MapClaims {
			c := idp.claims("app-1")
			delete(c, "client_id")
			return c
		}()),
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			deps := newTestDeps(t, idp)

			w := postGraphQL(SetupRouter(deps), body, token)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.False(t, deps.Federator.(*stubFederator).called, "unauthorized requests must not reach the federator")
		})
	}
}
