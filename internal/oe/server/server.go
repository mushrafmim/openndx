package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/openndx/openndx-core/internal/oe/auth"
	"github.com/openndx/openndx-core/internal/oe/logger"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
)

type Response struct {
	Message string `json:"message"`
}

// RunServer starts an HTTP server on port with graceful shutdown support.
// timeout bounds reading a request and writing its response. The server will
// shut down gracefully when the context is cancelled (e.g., on SIGINT/SIGTERM).
func RunServer(ctx context.Context, port int, timeout time.Duration, deps Deps) {
	mux := SetupRouter(deps)

	// Create HTTP server with proper configuration
	srv := &http.Server{
		Addr:         ":" + strconv.Itoa(port),
		Handler:      corsMiddleware(mux),
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
	}

	// Channel to signal server errors
	serverErrors := make(chan error, 1)

	// Start server in a goroutine
	go func() {
		logger.Log.Info("Server is Listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErrors:
		logger.Log.Error("Server error", "error", err)
	case <-ctx.Done():
		logger.Log.Info("Shutdown signal received, starting graceful shutdown")

		// Create shutdown context with timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Log.Error("Server shutdown error", "error", err)
			// Force close if graceful shutdown fails
			if closeErr := srv.Close(); closeErr != nil {
				logger.Log.Error("Server close error", "error", closeErr)
			}
		} else {
			logger.Log.Info("Server stopped gracefully")
		}
	}
}

// QueryFederator federates consumer GraphQL queries across providers.
type QueryFederator interface {
	FederateQuery(ctx context.Context, request graphql.Request, consumerInfo *auth.ConsumerAssertion) graphql.Response
}

// Deps are what the HTTP server serves. cmd/oe/main.go builds them.
type Deps struct {
	Federator QueryFederator
	// Authenticate rejects requests without a valid consumer token and otherwise
	// adds the consumer's identity to the request context (internal/authn's
	// RequireAuthMiddleware). Required.
	Authenticate func(http.Handler) http.Handler
}

// SetupRouter initializes the router and registers all endpoints
func SetupRouter(deps Deps) *http.ServeMux {
	mux := http.NewServeMux()

	// /health route
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		resp := Response{Message: "OpenNDX Server is Healthy!"}
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(resp)
		if err != nil {
			return
		}
	})

	// Consumer endpoint: requires a token the IdP issued for OE's audience
	mux.Handle("POST /public/graphql", deps.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		consumerAssertion := auth.ConsumerFromContext(r.Context())
		if consumerAssertion == nil {
			// Authenticate always sets the consumer; fail closed if it did not.
			logger.Log.Error("Consumer identity missing from an authenticated request")
			http.Error(w, "Unauthorized: authentication required", http.StatusUnauthorized)
			return
		}

		// Parse request body
		var req graphql.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Log.Error("Failed to decode request body", "error", err)
			http.Error(w, "Bad request: invalid JSON", http.StatusBadRequest)
			return
		}

		// Add panic recovery for federator calls
		var response graphql.Response
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Log.Error("Panic in FederateQuery", "panic", r, "stack", string(debug.Stack()))
					response = graphql.Response{
						Data: nil,
						Errors: []interface{}{
							map[string]interface{}{
								"message": fmt.Sprintf("Internal server error: %v", r),
							},
						},
					}
				}
			}()
			response = deps.Federator.FederateQuery(r.Context(), req, consumerAssertion)
		}()

		w.WriteHeader(http.StatusOK)
		// Set content type to application/json

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Log.Error("Failed to write response", "error", err)
			return
		}
	})))

	return mux
}

// corsMiddleware sets CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow all origins
		w.Header().Set("Access-Control-Allow-Origin", "*")
		// Allow specific methods
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		// Allow specific headers
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours

		// Handle preflight (OPTIONS) requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
