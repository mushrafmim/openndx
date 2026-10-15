package auth

import (
	"log/slog"
	"net/http"

	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/utils"
)

// FromAuthn returns middleware that builds the AuthenticatedUser from the
// principal internal/authn validated, so authorization and handlers keep
// working unchanged. Chain it after authn's RequireAuthMiddleware.
//
// This is a temporary bridge: once routes check scopes and handlers check the
// caller's organization unit directly, it goes away with this package.
func FromAuthn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCtx := authn.GetAuthContext(r.Context())
		if authCtx == nil {
			utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		user, err := NewAuthenticatedUser(authCtx.Subject(), authCtx.OUID(), authCtx.Email(), authCtx.Roles())
		if err != nil {
			slog.Warn("Rejected token without a valid role", "error", err, "path", r.URL.Path, "method", r.Method)
			utils.RespondWithError(w, http.StatusUnauthorized, "Invalid access token")
			return
		}

		next.ServeHTTP(w, r.WithContext(SetAuthenticatedUser(r.Context(), user)))
	})
}
