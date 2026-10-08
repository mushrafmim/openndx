package auth

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/openndx/openndx-core/internal/utils"
)

// AuthorizationConfig configures the authorization middleware behavior
type AuthorizationConfig struct {
	// Mode defines the behavior when no explicit permission is defined for an endpoint
	Mode AuthorizationMode

	// StrictMode when true, logs warnings about undefined endpoints in production
	StrictMode bool
}

// AuthorizationMiddleware provides role-based access control functionality
type AuthorizationMiddleware struct {
	config AuthorizationConfig
}

// NewAuthorizationMiddleware creates a new authorization middleware with default configuration
func NewAuthorizationMiddleware() *AuthorizationMiddleware {
	return NewAuthorizationMiddlewareWithConfig(AuthorizationConfig{
		Mode:       AuthorizationModeFailOpenAdminSystem, // Maintain backward compatibility
		StrictMode: false,
	})
}

// NewAuthorizationMiddlewareWithConfig creates a new authorization middleware with custom configuration
func NewAuthorizationMiddlewareWithConfig(config AuthorizationConfig) *AuthorizationMiddleware {
	return &AuthorizationMiddleware{
		config: config,
	}
}

// AuthorizeRequest returns a middleware function that checks user permissions for endpoints
func (a *AuthorizationMiddleware) AuthorizeRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip authorization for endpoints that don't require authentication
		if a.shouldSkipAuthorization(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Get authenticated user from context (should be set by JWT middleware)
		user, err := RequireAuthentication(r)
		if err != nil {
			slog.Warn("Authorization failed: user not authenticated", "path", r.URL.Path, "method", r.Method, "error", err)
			utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		// Find the endpoint permission requirement
		endpointPermission, found := FindEndpointPermission(r.Method, r.URL.Path)
		if !found {
			// Handle undefined endpoints based on configuration
			if a.handleUndefinedEndpoint(w, r, user) {
				return // Response already sent
			}
			// If handleUndefinedEndpoint returns false, continue to next handler
			next.ServeHTTP(w, r)
			return
		}

		// Check if user has the required permission
		if !user.HasPermission(endpointPermission.Permission) {
			slog.Warn("Access denied: insufficient permissions",
				"user", user.Email,
				"role", user.GetPrimaryRole(),
				"required_permission", endpointPermission.Permission,
				"path", r.URL.Path,
				"method", r.Method)
			utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
			return
		}

		// For endpoints that require ownership, we need to check resource ownership
		// This will be handled at the handler level since we need to extract resource IDs
		// For now, we just ensure the user has the permission

		slog.Info("Access granted",
			"user", user.Email,
			"role", user.GetPrimaryRole(),
			"permission", endpointPermission.Permission,
			"path", r.URL.Path,
			"method", r.Method)

		next.ServeHTTP(w, r)
	})
}

// RequireRole returns a middleware that requires a specific role
func (a *AuthorizationMiddleware) RequireRole(requiredRole Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := RequireRole(r, requiredRole)
			if err != nil {
				slog.Warn("Role requirement not met",
					"required_role", requiredRole,
					"path", r.URL.Path,
					"method", r.Method,
					"error", err)
				utils.RespondWithError(w, http.StatusForbidden, "Insufficient privileges")
				return
			}

			slog.Info("Role requirement satisfied",
				"user", user.Email,
				"required_role", requiredRole,
				"user_roles", user.Roles,
				"path", r.URL.Path,
				"method", r.Method)

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyRole returns a middleware that requires any of the specified roles
func (a *AuthorizationMiddleware) RequireAnyRole(requiredRoles ...Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := RequireAnyRole(r, requiredRoles...)
			if err != nil {
				roleNames := make([]string, len(requiredRoles))
				for i, role := range requiredRoles {
					roleNames[i] = role.String()
				}

				slog.Warn("Role requirement not met",
					"required_roles", strings.Join(roleNames, ", "),
					"path", r.URL.Path,
					"method", r.Method,
					"error", err)
				utils.RespondWithError(w, http.StatusForbidden, "Insufficient privileges")
				return
			}

			slog.Info("Role requirement satisfied",
				"user", user.Email,
				"required_roles", requiredRoles,
				"user_roles", user.Roles,
				"path", r.URL.Path,
				"method", r.Method)

			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission returns a middleware that requires a specific permission
func (a *AuthorizationMiddleware) RequirePermission(requiredPermission Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := RequirePermission(r, requiredPermission)
			if err != nil {
				slog.Warn("Permission requirement not met",
					"required_permission", requiredPermission,
					"path", r.URL.Path,
					"method", r.Method,
					"error", err)
				utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
				return
			}

			slog.Info("Permission requirement satisfied",
				"user", user.Email,
				"required_permission", requiredPermission,
				"user_permissions", user.GetPermissions(),
				"path", r.URL.Path,
				"method", r.Method)

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdminRole is a convenience middleware that requires admin role
func (a *AuthorizationMiddleware) RequireAdminRole() func(http.Handler) http.Handler {
	return a.RequireRole(RoleAdmin)
}

// RequireMemberRole is a convenience middleware that requires member role
func (a *AuthorizationMiddleware) RequireMemberRole() func(http.Handler) http.Handler {
	return a.RequireRole(RoleMember)
}

// RequireSystemRole is a convenience middleware that requires system role
func (a *AuthorizationMiddleware) RequireSystemRole() func(http.Handler) http.Handler {
	return a.RequireRole(RoleSystem)
}

// RequireAdminOrSystemRole requires either admin or system role
func (a *AuthorizationMiddleware) RequireAdminOrSystemRole() func(http.Handler) http.Handler {
	return a.RequireAnyRole(RoleAdmin, RoleSystem)
}

// CheckResourceOwnership is a helper function to be used in handlers to verify resource ownership
func (a *AuthorizationMiddleware) CheckResourceOwnership(user *AuthenticatedUser, resourceOwnerIdpUserId string, permission Permission) bool {
	return CanAccessResource(user, permission, resourceOwnerIdpUserId)
}

// handleUndefinedEndpoint handles access control for endpoints without explicit permission mappings
// Returns true if response was sent (request should stop), false if request should continue
func (a *AuthorizationMiddleware) handleUndefinedEndpoint(w http.ResponseWriter, r *http.Request, user *AuthenticatedUser) bool {
	// Log warning if in strict mode - helps identify missing permission mappings
	if a.config.StrictMode {
		slog.Warn("SECURITY: Undefined endpoint accessed - consider adding explicit permission mapping",
			"user", user.Email,
			"role", user.GetPrimaryRole(),
			"path", r.URL.Path,
			"method", r.Method,
			"mode", a.config.Mode)
	}

	switch a.config.Mode {
	case AuthorizationModeFailClosed:
		// Most secure: deny all access to undefined endpoints
		slog.Warn("Access denied to undefined endpoint (fail-closed mode)",
			"user", user.Email,
			"role", user.GetPrimaryRole(),
			"path", r.URL.Path,
			"method", r.Method)
		utils.RespondWithError(w, http.StatusForbidden, "Endpoint access not explicitly permitted")
		return true

	case AuthorizationModeFailOpenAdmin:
		// Allow only admin users
		if user.IsAdmin() {
			slog.Info("Access granted to undefined endpoint (admin-only mode)",
				"user", user.Email,
				"role", user.GetPrimaryRole(),
				"path", r.URL.Path,
				"method", r.Method)
			return false // Continue to handler
		}

		slog.Warn("Access denied to undefined endpoint (admin-only mode)",
			"user", user.Email,
			"role", user.GetPrimaryRole(),
			"path", r.URL.Path,
			"method", r.Method)
		utils.RespondWithError(w, http.StatusForbidden, "Administrative access required")
		return true

	case AuthorizationModeFailOpenAdminSystem:
		// Legacy behavior: allow admin and system users
		if user.IsAdmin() || user.IsSystem() {
			slog.Info("Access granted to undefined endpoint (admin/system mode)",
				"user", user.Email,
				"role", user.GetPrimaryRole(),
				"path", r.URL.Path,
				"method", r.Method)
			return false // Continue to handler
		}

		slog.Warn("Access denied to undefined endpoint (admin/system mode)",
			"user", user.Email,
			"role", user.GetPrimaryRole(),
			"path", r.URL.Path,
			"method", r.Method)
		utils.RespondWithError(w, http.StatusForbidden, "Administrative or system access required")
		return true

	default:
		// Fallback to most secure mode if configuration is invalid
		slog.Error("Invalid authorization mode, defaulting to fail-closed",
			"mode", a.config.Mode,
			"path", r.URL.Path,
			"method", r.Method)
		utils.RespondWithError(w, http.StatusForbidden, "Access denied")
		return true
	}
}

// shouldSkipAuthorization determines if authorization should be skipped for this path
func (a *AuthorizationMiddleware) shouldSkipAuthorization(path string) bool {
	skipPaths := []string{
		"/health",
		"/debug",
		"/openapi.yaml",
		"/favicon.ico",
	}

	for _, skipPath := range skipPaths {
		if strings.HasPrefix(path, skipPath) {
			return true
		}
	}
	return false
}

// GetUserFromRequest is a helper to extract the authenticated user from request context
func GetUserFromRequest(r *http.Request) (*AuthenticatedUser, error) {
	return GetAuthenticatedUser(r.Context())
}

// GetAuthContextFromRequest is a helper to extract the auth context from request context
func GetAuthContextFromRequest(r *http.Request) (*AuthContext, error) {
	return GetAuthContext(r.Context())
}
