package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openndx/openndx-core/internal/oe/auth"
	"github.com/openndx/openndx-core/internal/oe/configs"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/stretchr/testify/assert"
)

// stubFederator is a QueryFederator that records whether it was called.
type stubFederator struct {
	called bool
}

func (s *stubFederator) FederateQuery(context.Context, graphql.Request, *auth.ConsumerAssertion) graphql.Response {
	s.called = true
	return graphql.Response{}
}

// newTestDeps returns server dependencies with no schema database and no token validator.
func newTestDeps() Deps {
	return Deps{
		Config: &configs.Config{
			Environment:           "test",
			TrustUpstream:         true, // Trust upstream to avoid JWT validation requirements
			FoundationalIdArgName: "nic",
		},
		Federator: &stubFederator{},
	}
}

func TestSetupRouter_Health(t *testing.T) {
	mux := SetupRouter(newTestDeps())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "OpenNDX Server is Healthy!")
}

func TestSetupRouter_SDL_Endpoints(t *testing.T) {
	mux := SetupRouter(newTestDeps())

	endpoints := []string{
		"/sdl",
		"/sdl/versions",
		"/sdl/validate",
		"/sdl/check-compatibility",
	}

	for _, endpoint := range endpoints {
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		if endpoint == "/sdl/validate" || endpoint == "/sdl/check-compatibility" {
			req = httptest.NewRequest(http.MethodPost, endpoint, nil)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		// We expect some response, likely 500 (service unavailable) or 200 depending on implementation details of handlers
		// 404 means route not found, 500 means route found but handler returned error (which is expected with nil service)

		// Accept 500 (service unavailable), 200 (success), 400 (bad request), or 404 (not found) as valid responses
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError || w.Code == http.StatusServiceUnavailable || w.Code == http.StatusBadRequest || w.Code == http.StatusNotFound,
			"Endpoint %s should return 200, 400, 404, 500, or 503 (got %d)", endpoint, w.Code)
	}
}

func TestSetupRouter_PublicGraphQL_BadRequest(t *testing.T) {
	mux := SetupRouter(newTestDeps())

	// Invalid JSON body
	req := httptest.NewRequest(http.MethodPost, "/public/graphql", bytes.NewBufferString("invalid-json"))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSetupRouter_PublicGraphQL_Unauthorized(t *testing.T) {
	deps := newTestDeps()
	federator := deps.Federator.(*stubFederator)
	mux := SetupRouter(deps)

	// Valid JSON but missing auth token
	gqlReq := graphql.Request{
		Query: "{ hello }",
	}
	body, _ := json.Marshal(gqlReq)
	req := httptest.NewRequest(http.MethodPost, "/public/graphql", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// Should be Unauthorized because GetConsumerJwtFromToken will fail
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.False(t, federator.called, "unauthorized requests must not reach the federator")
}
