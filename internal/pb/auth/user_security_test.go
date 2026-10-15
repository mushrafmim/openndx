package auth

import (
	"testing"
)

// TestNewAuthenticatedUser_NoRoles tests that users with no roles are properly rejected
func TestNewAuthenticatedUser_NoRoles(t *testing.T) {
	// Test case 1: User with empty roles slice
	user, err := NewAuthenticatedUser("noroles-123", "", "noroles@example.com", []string{})
	if err == nil {
		t.Errorf("Expected error for user with no roles, but got none. User: %+v", user)
	}
	if user != nil {
		t.Errorf("Expected nil user for no roles, but got: %+v", user)
	}
	if err != nil && err.Error() != "access denied: no valid roles found for user noroles-123" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

// TestNewAuthenticatedUser_InvalidRoles tests that users with only invalid roles are rejected
func TestNewAuthenticatedUser_InvalidRoles(t *testing.T) {
	user, err := NewAuthenticatedUser("invalidroles-123", "", "invalidroles@example.com", []string{"InvalidRole", "AnotherInvalidRole"})
	if err == nil {
		t.Errorf("Expected error for user with invalid roles, but got none. User: %+v", user)
	}
	if user != nil {
		t.Errorf("Expected nil user for invalid roles, but got: %+v", user)
	}
}

// TestNewAuthenticatedUser_ValidRoles tests that users with valid roles are accepted
func TestNewAuthenticatedUser_ValidRoles(t *testing.T) {
	user, err := NewAuthenticatedUser("validroles-123", "", "validroles@example.com", []string{"OpenNDX_Member"})
	if err != nil {
		t.Errorf("Expected no error for user with valid roles, but got: %v", err)
	}
	if user == nil {
		t.Errorf("Expected user for valid roles, but got nil")
	}
	if user != nil && user.Email != "validroles@example.com" {
		t.Errorf("Expected email 'validroles@example.com', got: %s", user.Email)
	}
	if user != nil && len(user.Roles) != 1 {
		t.Errorf("Expected 1 role, got: %d", len(user.Roles))
	}
	if user != nil && user.Roles[0] != RoleMember {
		t.Errorf("Expected role RoleMember, got: %v", user.Roles[0])
	}
}

// TestNewAuthenticatedUser_MixedRoles tests that users with mixed valid/invalid roles get only valid roles
func TestNewAuthenticatedUser_MixedRoles(t *testing.T) {
	user, err := NewAuthenticatedUser("mixedroles-123", "", "mixedroles@example.com", []string{"OpenNDX_Admin", "InvalidRole", "OpenNDX_Member"})
	if err != nil {
		t.Errorf("Expected no error for user with mixed roles (some valid), but got: %v", err)
	}
	if user == nil {
		t.Errorf("Expected user for mixed roles (some valid), but got nil")
	}
	if user != nil && len(user.Roles) != 2 {
		t.Errorf("Expected 2 valid roles, got: %d (%v)", len(user.Roles), user.Roles)
	}
	if user != nil {
		// Check that only valid roles are present
		hasAdmin := false
		hasMember := false
		for _, role := range user.Roles {
			if role == RoleAdmin {
				hasAdmin = true
			} else if role == RoleMember {
				hasMember = true
			} else {
				t.Errorf("Unexpected role found: %v", role)
			}
		}
		if !hasAdmin {
			t.Errorf("Expected RoleAdmin to be present")
		}
		if !hasMember {
			t.Errorf("Expected RoleMember to be present")
		}
	}
}
