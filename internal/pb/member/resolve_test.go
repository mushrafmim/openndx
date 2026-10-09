package member

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/openndx/openndx-core/internal/pb/auth"
	"github.com/openndx/openndx-core/internal/pb/database/dbtest"
	"github.com/openndx/openndx-core/internal/pb/idp/idptest"
	"github.com/stretchr/testify/assert"
)

func TestResolveMemberID_Caching(t *testing.T) {
	db := dbtest.SetupSQLiteDB(t, &Member{})
	service := NewService(db, &idptest.Mock{})
	ctx := context.Background()

	// Create a user
	user := &auth.AuthenticatedUser{
		IdpUserID: "test-user-id",
		Email:     "test@example.com",
	}

	// Case 1: Member not found in DB
	// Note: GetAllMembers uses DB, not IDP, so we don't need to mock IDP for this call

	id, err := service.ResolveMemberID(ctx, user)
	assert.Error(t, err)
	assert.Empty(t, id)
	// Service.getFilteredMembers returns error when record not found
	assert.Contains(t, err.Error(), "failed to fetch member")

	// Verify error is cached
	errCached := user.GetCachedMemberIDError()
	assert.Error(t, errCached)
	assert.Equal(t, err, errCached)

	// Case 2: Cached error is returned
	id, err = service.ResolveMemberID(ctx, user)
	assert.Error(t, err)
	assert.Empty(t, id)
	assert.Equal(t, errCached, err)

	// Case 3: Member exists
	// Clear cache
	user = &auth.AuthenticatedUser{
		IdpUserID: "test-user-id-2",
		Email:     "test2@example.com",
	}

	// Create member in DB with the user's IdpUserID
	m := Member{
		MemberID:    "mem_" + fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:        "Test Member",
		Email:       "test2@example.com",
		PhoneNumber: "1234567890",
		IdpUserID:   "test-user-id-2",
	}
	assert.NoError(t, db.Create(&m).Error)

	id, err = service.ResolveMemberID(ctx, user)
	assert.NoError(t, err)
	assert.Equal(t, m.MemberID, id)

	// Verify ID is cached
	cachedID, cached := user.GetCachedMemberID()
	assert.True(t, cached)
	assert.Equal(t, m.MemberID, cachedID)

	// Case 4: Cached ID is returned
	id, err = service.ResolveMemberID(ctx, user)
	assert.NoError(t, err)
	assert.Equal(t, m.MemberID, id)
}
