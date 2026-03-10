// Package middleware provides HTTP middleware for FeatherCI.
package middleware

import (
	"context"
	"net/http"

	"github.com/featherci/featherci/internal/models"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	// UserContextKey is the context key for the authenticated user.
	UserContextKey contextKey = "user"
)

// AuthMiddleware provides authentication middleware.
type AuthMiddleware struct {
	sessions models.SessionStore
	users    models.UserRepository
}

// NewAuthMiddleware creates a new authentication middleware.
func NewAuthMiddleware(sessions models.SessionStore, users models.UserRepository) *AuthMiddleware {
	return &AuthMiddleware{
		sessions: sessions,
		users:    users,
	}
}

// RequireAuth is middleware that requires a valid authenticated session.
// If the user is not authenticated, it returns a 401 Unauthorized response.
func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := m.getAuthenticatedUser(r)
		if err != nil || user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := SetUserInContext(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalAuth is middleware that loads the user if authenticated,
// but allows the request to continue even if not authenticated.
func (m *AuthMiddleware) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := m.getAuthenticatedUser(r)
		if user != nil {
			ctx := SetUserInContext(r.Context(), user)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin is middleware that requires an authenticated admin user.
func (m *AuthMiddleware) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := m.getAuthenticatedUser(r)
		if err != nil || user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if !user.IsAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		ctx := SetUserInContext(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// getAuthenticatedUser retrieves the authenticated user from the request session.
func (m *AuthMiddleware) getAuthenticatedUser(r *http.Request) (*models.User, error) {
	sessionID, err := models.GetSessionFromRequest(r)
	if err != nil {
		return nil, err
	}

	session, err := m.sessions.Get(r.Context(), sessionID)
	if err != nil {
		return nil, err
	}

	if session.IsExpired() {
		// Clean up expired session
		_ = m.sessions.Delete(r.Context(), sessionID)
		return nil, models.ErrNotFound
	}

	return m.users.GetByID(r.Context(), session.UserID)
}

// UserFromContext retrieves the authenticated user from the context.
// Returns nil if no user is present.
func UserFromContext(ctx context.Context) *models.User {
	user, _ := ctx.Value(UserContextKey).(*models.User)
	return user
}

// SetUserInContext adds the user to the context.
func SetUserInContext(ctx context.Context, user *models.User) context.Context {
	return context.WithValue(ctx, UserContextKey, user)
}
