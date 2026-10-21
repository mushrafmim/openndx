package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/ce/handlers"
	"github.com/openndx/openndx-core/internal/ce/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestAuthenticate builds the portal authentication chain the way
// cmd/ce/main.go does. The JWKS URL is never fetched: every request in these
// tests is rejected before a signing key is needed.
func newTestAuthenticate(t *testing.T) func(http.Handler) http.Handler {
	t.Helper()
	manager, err := authn.NewManager(authn.Config{
		JWKSURL:  "http://localhost/.well-known/jwks.json",
		Issuer:   "test-issuer",
		Audience: "test-audience",
	})
	require.NoError(t, err)
	return func(next http.Handler) http.Handler {
		return manager.RequireAuthMiddleware()(middleware.OwnerSubject(middleware.SubjectClaim)(next))
	}
}

func TestNewRouter(t *testing.T) {
	internalHandler := &handlers.InternalHandler{}
	portalHandler := &handlers.PortalHandler{}

	router := NewRouter("http://localhost:5173", internalHandler, portalHandler, newTestAuthenticate(t))

	assert.NotNil(t, router)
	assert.Equal(t, internalHandler, router.internalHandler)
	assert.Equal(t, portalHandler, router.portalHandler)
	assert.NotNil(t, router.authenticate)
	assert.NotNil(t, router.corsMiddleware)
}

func TestRouter_RegisterRoutes(t *testing.T) {
	internalHandler := &handlers.InternalHandler{}
	portalHandler := &handlers.PortalHandler{}

	router := NewRouter("http://localhost:5173", internalHandler, portalHandler, newTestAuthenticate(t))
	mux := http.NewServeMux()

	router.RegisterRoutes(mux)

	// Test that internal routes are registered
	req := httptest.NewRequest("GET", "/internal/api/v1/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Should not return 404 (route exists)
	assert.NotEqual(t, http.StatusNotFound, w.Code)

	// Test that portal routes are registered
	req = httptest.NewRequest("GET", "/api/v1/health", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Should not return 404 (route exists)
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestRouter_PortalConsentRoutesRequireAuthentication(t *testing.T) {
	// The handlers have no service: reaching one would panic, so a 401 shows
	// the route rejected the request before the handler ran.
	router := NewRouter("http://localhost:5173", &handlers.InternalHandler{}, &handlers.PortalHandler{}, newTestAuthenticate(t))
	mux := http.NewServeMux()
	router.RegisterRoutes(mux)

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/consents"},
		{http.MethodGet, "/api/v1/consents/consent-1"},
		{http.MethodPut, "/api/v1/consents/consent-1"},
	} {
		for name, authorization := range map[string]string{
			"missing token": "",
			"invalid token": "Bearer not-a-jwt",
		} {
			t.Run(route.method+" "+route.path+"/"+name, func(t *testing.T) {
				req := httptest.NewRequest(route.method, route.path, nil)
				if authorization != "" {
					req.Header.Set("Authorization", authorization)
				}
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, req)

				assert.Equal(t, http.StatusUnauthorized, w.Code)
			})
		}
	}
}

func TestRouter_ApplyCORS(t *testing.T) {
	internalHandler := &handlers.InternalHandler{}
	portalHandler := &handlers.PortalHandler{}

	router := NewRouter("http://localhost:5173", internalHandler, portalHandler, newTestAuthenticate(t))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := router.ApplyCORS(handler)

	assert.NotNil(t, wrapped)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	// CORS headers should be set
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Origin"))
}
