// Package auth holds the Portal Backend's view of the caller: the OAuth2
// scopes its routes require, and the AuthenticatedUser handlers use for
// ownership checks. Token validation is done by internal/authn and scope
// checks on routes by github.com/OpenNSW/core/authz.
package auth

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"

	"github.com/OpenNSW/core/authz"
	"github.com/openndx/openndx-core/internal/authn"
)

// AuthenticatedUser is the caller of a request, as handlers see it.
type AuthenticatedUser struct {
	IdpUserID string   `json:"idpUserId"`
	OuID      string   `json:"ouId"`
	Email     string   `json:"email"`
	Scopes    []string `json:"scopes"`

	// Cached member ID - populated on first access to avoid repeated database queries
	// Protected by mutex for thread safety
	memberIDMutex sync.RWMutex `json:"-"` // Protects memberID and memberIDError fields
	memberID      string       `json:"-"` // Don't expose in JSON
	memberIDError error        `json:"-"` // Cache the error state as well
}

// PrincipalFromContext is the authz.Extractor for PB: the principal
// internal/authn validated, or false when the request is unauthenticated.
func PrincipalFromContext(ctx context.Context) (authz.Principal, bool) {
	authCtx := authn.GetAuthContext(ctx)
	if authCtx == nil {
		return nil, false
	}
	return authCtx, true
}

// GetUserFromRequest returns the caller of r, built from the principal
// internal/authn validated. It fails when the request is unauthenticated.
func GetUserFromRequest(r *http.Request) (*AuthenticatedUser, error) {
	authCtx := authn.GetAuthContext(r.Context())
	if authCtx == nil {
		return nil, errors.New("no authenticated user found in context")
	}
	return &AuthenticatedUser{
		IdpUserID: authCtx.Subject(),
		OuID:      authCtx.OUID(),
		Email:     authCtx.Email(),
		Scopes:    authCtx.Scopes(),
	}, nil
}

// HasScope reports whether the user's token was granted scope.
func (u *AuthenticatedUser) HasScope(scope string) bool {
	return slices.Contains(u.Scopes, scope)
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
