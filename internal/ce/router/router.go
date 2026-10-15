package router

import (
	"net/http"

	"github.com/openndx/openndx-core/internal/ce/auth"
	"github.com/openndx/openndx-core/internal/ce/handlers"
	"github.com/openndx/openndx-core/internal/ce/middleware"
	"github.com/openndx/openndx-core/internal/utils"
)

// Router handles all API route registration
type Router struct {
	internalHandler *handlers.InternalHandler
	portalHandler   *handlers.PortalHandler
	authMiddleware  *middleware.JWTAuthMiddleware
	corsMiddleware  func(http.Handler) http.Handler
}

// NewRouter creates a new router with all dependencies
func NewRouter(
	allowedOrigins string,
	internalHandler *handlers.InternalHandler,
	portalHandler *handlers.PortalHandler,
	jwtVerifier *auth.JWTVerifier,
) *Router {
	return &Router{
		internalHandler: internalHandler,
		portalHandler:   portalHandler,
		authMiddleware:  middleware.NewJWTAuthMiddleware(jwtVerifier),
		corsMiddleware:  middleware.NewCORSMiddleware(allowedOrigins),
	}
}

// RegisterRoutes registers all API routes to the provided mux
func (r *Router) RegisterRoutes(mux *http.ServeMux) {
	r.registerInternalRoutes(mux)
	r.registerPortalRoutes(mux)
}

// registerInternalRoutes registers internal API routes (no authentication required)
func (r *Router) registerInternalRoutes(mux *http.ServeMux) {
	// Health check
	mux.Handle("/internal/api/v1/health",
		utils.PanicRecoveryMiddleware(http.HandlerFunc(r.internalHandler.HealthCheck)))

	// Consents endpoint
	mux.Handle("GET /internal/api/v1/consents",
		utils.PanicRecoveryMiddleware(http.HandlerFunc(r.internalHandler.GetConsent)))
	mux.Handle("POST /internal/api/v1/consents",
		utils.PanicRecoveryMiddleware(http.HandlerFunc(r.internalHandler.CreateConsent)))
}

// registerPortalRoutes registers portal API routes (authentication required for protected endpoints)
func (r *Router) registerPortalRoutes(mux *http.ServeMux) {
	// Health check endpoint (public - no authentication per OpenAPI spec)
	mux.Handle("/api/v1/health",
		utils.PanicRecoveryMiddleware(http.HandlerFunc(r.portalHandler.HealthCheck)))

	// Consent endpoints (authentication required)
	mux.Handle("GET /api/v1/consents",
		utils.PanicRecoveryMiddleware(
			r.authMiddleware.Authenticate(http.HandlerFunc(r.portalHandler.ListConsents))))
	mux.Handle("GET /api/v1/consents/{consentId}",
		utils.PanicRecoveryMiddleware(
			r.authMiddleware.Authenticate(http.HandlerFunc(r.portalHandler.GetConsent))))
	mux.Handle("PUT /api/v1/consents/{consentId}",
		utils.PanicRecoveryMiddleware(
			r.authMiddleware.Authenticate(http.HandlerFunc(r.portalHandler.UpdateConsent))))
}

// ApplyCORS wraps a handler with CORS middleware
func (r *Router) ApplyCORS(handler http.Handler) http.Handler {
	return r.corsMiddleware(handler)
}
