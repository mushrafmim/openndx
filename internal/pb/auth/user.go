// Package auth handles authorization for the Portal Backend: the
// authenticated user, roles and permissions, and the middleware that enforces
// them. Token validation is done by internal/authn; FromAuthn bridges its
// principal into the AuthenticatedUser used here.
package auth

import (
	"fmt"
	"log/slog"
	"sync"
)

// AuthenticatedUser represents the authenticated user context
type AuthenticatedUser struct {
	IdpUserID string `json:"idpUserId"`
	OuID      string `json:"ouId"`
	Email     string `json:"email"`
	Roles     []Role `json:"roles"`

	// Cached permissions - computed once during user creation for performance
	permissions []Permission `json:"-"` // Don't expose in JSON, use GetPermissions() method

	// Cached member ID - populated on first access to avoid repeated database queries
	// Protected by mutex for thread safety
	memberIDMutex sync.RWMutex `json:"-"` // Protects memberID and memberIDError fields
	memberID      string       `json:"-"` // Don't expose in JSON
	memberIDError error        `json:"-"` // Cache the error state as well
}

// HasRole checks if the user has a specific role
func (u *AuthenticatedUser) HasRole(role Role) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasAnyRole checks if the user has any of the specified roles
func (u *AuthenticatedUser) HasAnyRole(roles ...Role) bool {
	for _, requiredRole := range roles {
		for _, userRole := range u.Roles {
			if userRole == requiredRole {
				return true
			}
		}
	}
	return false
}

// HasPermission checks if the user has a specific permission based on their roles
func (u *AuthenticatedUser) HasPermission(permission Permission) bool {
	for _, role := range u.Roles {
		if role.HasPermission(permission) {
			return true
		}
	}
	return false
}

// IsAdmin checks if the user has admin role
func (u *AuthenticatedUser) IsAdmin() bool {
	return u.HasRole(RoleAdmin)
}

// IsMember checks if the user has member role
func (u *AuthenticatedUser) IsMember() bool {
	return u.HasRole(RoleMember)
}

// IsSystem checks if the user has system role
func (u *AuthenticatedUser) IsSystem() bool {
	return u.HasRole(RoleSystem)
}

// GetPrimaryRole returns the highest priority role (Admin > System > Member)
func (u *AuthenticatedUser) GetPrimaryRole() Role {
	if u.HasRole(RoleAdmin) {
		return RoleAdmin
	}
	if u.HasRole(RoleSystem) {
		return RoleSystem
	}
	if u.HasRole(RoleMember) {
		return RoleMember
	}
	return RoleMember // Default to member if no roles found
}

// GetPermissions returns all permissions the user has based on their roles
// Uses cached permissions computed during user creation for optimal performance
func (u *AuthenticatedUser) GetPermissions() []Permission {
	// Return a copy of the cached permissions to prevent external modification
	result := make([]Permission, len(u.permissions))
	copy(result, u.permissions)
	return result
}

// GetCachedMemberID returns the cached member ID if available
// Thread-safe using read lock
func (u *AuthenticatedUser) GetCachedMemberID() (string, bool) {
	u.memberIDMutex.RLock()
	defer u.memberIDMutex.RUnlock()
	return u.memberID, u.memberID != ""
}

// SetCachedMemberID sets the cached member ID and error state
// Thread-safe using write lock
func (u *AuthenticatedUser) SetCachedMemberID(memberID string, err error) {
	u.memberIDMutex.Lock()
	defer u.memberIDMutex.Unlock()
	u.memberID = memberID
	u.memberIDError = err
}

// GetCachedMemberIDError returns the cached error from member ID lookup
// Thread-safe using read lock
func (u *AuthenticatedUser) GetCachedMemberIDError() error {
	u.memberIDMutex.RLock()
	defer u.memberIDMutex.RUnlock()
	return u.memberIDError
}

// GetCachedMemberIDWithError atomically returns both the cached member ID and error state
// This ensures consistency when reading both values together
func (u *AuthenticatedUser) GetCachedMemberIDWithError() (memberID string, cached bool, err error) {
	u.memberIDMutex.RLock()
	defer u.memberIDMutex.RUnlock()
	return u.memberID, u.memberID != "", u.memberIDError
}

// computePermissions calculates all permissions for the given roles
func computePermissions(roles []Role) []Permission {
	permissionSet := make(map[Permission]bool)

	for _, role := range roles {
		if permissions, exists := RolePermissions[role]; exists {
			for _, permission := range permissions {
				permissionSet[permission] = true
			}
		}
	}

	var permissions []Permission
	for permission := range permissionSet {
		permissions = append(permissions, permission)
	}

	return permissions
}

// NewAuthenticatedUser creates a new authenticated user from the caller's IdP
// user ID, organization unit, email and role names.
// Returns an error if none of the role names is a valid role
func NewAuthenticatedUser(idpUserID, ouID, email string, roleNames []string) (*AuthenticatedUser, error) {
	// Convert string roles to Role type, tracking invalid ones for security logging
	var roles []Role
	var invalidRoles []string
	for _, roleStr := range roleNames {
		role := Role(roleStr)
		if role.IsValid() {
			roles = append(roles, role)
		} else {
			invalidRoles = append(invalidRoles, roleStr)
		}
	}

	// Log security-relevant event when invalid roles are filtered
	if len(invalidRoles) > 0 {
		slog.Warn("Invalid roles filtered from token", "user", idpUserID, "invalid_roles", invalidRoles)
	}

	// If no valid roles found, deny access for security
	if len(roles) == 0 {
		return nil, fmt.Errorf("access denied: no valid roles found for user %s", idpUserID)
	}

	// Compute permissions once during user creation for optimal performance
	permissions := computePermissions(roles)

	return &AuthenticatedUser{
		IdpUserID:   idpUserID,
		OuID:        ouID,
		Email:       email,
		Roles:       roles,
		permissions: permissions,
	}, nil
}
