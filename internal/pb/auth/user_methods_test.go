package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthenticatedUser_Methods(t *testing.T) {
	user := &AuthenticatedUser{
		Roles: []Role{RoleAdmin, RoleMember},
		permissions: []Permission{
			PermissionCreateSchema,
			PermissionReadSchema,
		},
	}

	// Test HasRole
	assert.True(t, user.HasRole(RoleAdmin))
	assert.True(t, user.HasRole(RoleMember))
	assert.False(t, user.HasRole(RoleSystem))

	// Test HasAnyRole
	assert.True(t, user.HasAnyRole(RoleAdmin, RoleSystem))
	assert.True(t, user.HasAnyRole(RoleSystem, RoleMember))
	assert.False(t, user.HasAnyRole(RoleSystem))

	// Test HasPermission
	assert.True(t, user.HasPermission(PermissionCreateSchema))

	userMemberOnly := &AuthenticatedUser{Roles: []Role{RoleMember}}
	assert.False(t, userMemberOnly.HasPermission(PermissionDeleteMember))

	// Test IsAdmin/IsMember/IsSystem
	assert.True(t, user.IsAdmin())
	assert.True(t, user.IsMember())
	assert.False(t, user.IsSystem())

	// Test GetPrimaryRole
	assert.Equal(t, RoleAdmin, user.GetPrimaryRole())

	userMember := &AuthenticatedUser{Roles: []Role{RoleMember}}
	assert.Equal(t, RoleMember, userMember.GetPrimaryRole())

	userSystem := &AuthenticatedUser{Roles: []Role{RoleSystem}}
	assert.Equal(t, RoleSystem, userSystem.GetPrimaryRole())

	// Test GetPermissions
	perms := user.GetPermissions()
	assert.Len(t, perms, 2)
	assert.Contains(t, perms, PermissionCreateSchema)
}

func TestAuthenticatedUser_CachedMemberID(t *testing.T) {
	user := &AuthenticatedUser{}

	// Initial state
	id, cached := user.GetCachedMemberID()
	assert.False(t, cached)
	assert.Empty(t, id)

	// Set cached ID
	user.SetCachedMemberID("mem-123", nil)

	// Check cached ID
	id, cached = user.GetCachedMemberID()
	assert.True(t, cached)
	assert.Equal(t, "mem-123", id)
	assert.NoError(t, user.GetCachedMemberIDError())

	// Check GetCachedMemberIDWithError
	id, cached, err := user.GetCachedMemberIDWithError()
	assert.True(t, cached)
	assert.Equal(t, "mem-123", id)
	assert.NoError(t, err)
}
