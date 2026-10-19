package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/openndx/openndx-core/internal/oe/auth"
	"github.com/openndx/openndx-core/internal/oe/configs"
	"github.com/openndx/openndx-core/internal/oe/handlers"
	"github.com/openndx/openndx-core/internal/oe/logger"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
)

type Response struct {
	Message string `json:"message"`
}

const DefaultPort = "4000"

// RunServer starts an HTTP server with graceful shutdown support.
// The server will shut down gracefully when the context is cancelled (e.g., on SIGINT/SIGTERM).
func RunServer(ctx context.Context, deps Deps) {
	mux := SetupRouter(deps)

	// Get port configuration
	port := os.Getenv("PORT")
	if port == "" {
		port = DefaultPort
	}

	// Convert port to string with colon prefix
	if port[0] != ':' {
		port = ":" + port
	}

	// Create HTTP server with proper configuration
	srv := &http.Server{
		Addr:    port,
		Handler: corsMiddleware(mux),
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
	// Config supplies the JWT settings and trustUpstream for consumer tokens.
	Config    *configs.Config
	Federator QueryFederator
	// SchemaService backs the /sdl routes; nil when the schema database is unavailable.
	SchemaService handlers.SchemaService
	// TokenValidator verifies consumer token signatures; nil when not configured.
	TokenValidator *auth.TokenValidator
}

// SetupRouter initializes the router and registers all endpoints
func SetupRouter(deps Deps) *http.ServeMux {
	mux := http.NewServeMux()

	schemaHandler := handlers.NewSchemaHandler(deps.SchemaService)

	// /health route
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		resp := Response{Message: "OpenNDX Server is Healthy!"}
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(resp)
		if err != nil {
			return
		}
	})

	// Schema management routes
	mux.HandleFunc("GET /sdl", schemaHandler.GetActiveSchema)
	mux.HandleFunc("POST /sdl", schemaHandler.CreateSchema)
	mux.HandleFunc("GET /sdl/versions", schemaHandler.GetSchemas)
	mux.HandleFunc("POST /sdl/validate", schemaHandler.ValidateSDL)
	mux.HandleFunc("POST /sdl/check-compatibility", schemaHandler.CheckCompatibility)

	// Handle activation endpoint with proper path matching
	mux.HandleFunc("POST /sdl/versions/{version}/activate", schemaHandler.ActivateSchema)

	// Publicly accessible Endpoints
	mux.HandleFunc("POST /public/graphql", func(w http.ResponseWriter, r *http.Request) {
		// Parse request body
		var req graphql.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Log.Error("Failed to decode request body", "error", err)
			http.Error(w, "Bad request: invalid JSON", http.StatusBadRequest)
			return
		}

		// decode the token using the cached TokenValidator
		consumerAssertion, err := auth.GetConsumerJwtFromTokenWithValidator(&deps.Config.JWT, deps.Config.TrustUpstream, r, deps.TokenValidator)
		if err != nil {
			logger.Log.Error("Failed to get consumer JWT from token", "error", err)
			// Return generic error to client to avoid exposing internal details
			http.Error(w, "Unauthorized: invalid or expired token", http.StatusUnauthorized)
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

		err = json.NewEncoder(w).Encode(response)
		if err != nil {
			logger.Log.Error("Failed to write response", "error", err)
			return
		}
	})

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
