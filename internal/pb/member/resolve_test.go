package member

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/openndx/openndx-core/internal/pb/auth"
	"github.com/openndx/openndx-core/internal/pb/database/dbtest"
	"github.com/stretchr/testify/assert"
)

func TestResolveMemberID_Caching(t *testing.T) {
	db := dbtest.SetupSQLiteDB(t, &Member{})
	service := NewService(db)
	ctx := context.Background()

	// Create a user
	user := &auth.AuthenticatedUser{
		IdpUserID: "test-user-id",
		OuID:      "test-ou-id",
		Email:     "test@example.com",
	}

	// Case 1: Member not found in DB
	id, err := service.ResolveMemberID(ctx, user)
	assert.Error(t, err)
	assert.Empty(t, id)
	// No member is linked to the user's organization unit
	assert.Contains(t, err.Error(), "user member record not found")

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
		OuID:      "test-ou-id-2",
		Email:     "test2@example.com",
	}

	// Create member in DB linked to the user's organization unit
	m := Member{
		MemberID:    "mem_" + fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:        "Test Member",
		Email:       "org@example.com",
		PhoneNumber: "1234567890",
		OuID:        "test-ou-id-2",
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

func TestResolveMemberID_NoOuClaim(t *testing.T) {
	db := dbtest.SetupSQLiteDB(t, &Member{})
	service := NewService(db)

	// A member exists, but a user without an OU claim must not resolve to it
	assert.NoError(t, db.Create(&Member{
		MemberID:    "mem_1",
		Name:        "Test Member",
		Email:       "org@example.com",
		PhoneNumber: "1234567890",
		OuID:        "test-ou-id",
	}).Error)

	user := &auth.AuthenticatedUser{IdpUserID: "test-user-id"}

	id, err := service.ResolveMemberID(context.Background(), user)
	assert.Error(t, err)
	assert.Empty(t, id)
	assert.Contains(t, err.Error(), "no organization unit")
	assert.Equal(t, err, user.GetCachedMemberIDError())
}
