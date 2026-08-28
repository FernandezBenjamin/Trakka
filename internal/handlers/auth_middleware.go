package handlers

import (
	"context"
	"net/http"

	"trakka/internal/auth"
	"trakka/internal/models"
)

type contextKey string

const userContextKey contextKey = "trakka_user"

// RequireSession gates next behind a valid session cookie, injecting the
// resolved user into the request context. It's applied once, around the
// whole /api/v1/ subtree, in Routes().
func (app *Application) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(auth.SessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		user, err := app.Auth.ValidateSession(r.Context(), cookie.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	})
}

// userFromContext returns the authenticated user stored by RequireSession.
// Only ever called from within the /api/v1/ subtree, where it is always
// present.
func userFromContext(r *http.Request) *models.User {
	u, _ := r.Context().Value(userContextKey).(*models.User)
	return u
}
