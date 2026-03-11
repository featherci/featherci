// Package handlers provides HTTP handlers for FeatherCI.
package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/featherci/featherci/internal/auth"
	"github.com/featherci/featherci/internal/config"
	"github.com/featherci/featherci/internal/models"
)

const (
	// StateCookieName is the name of the CSRF state cookie.
	StateCookieName = "featherci_oauth_state"

	// StateCookieMaxAge is how long the state cookie is valid.
	StateCookieMaxAge = 10 * time.Minute

	// stateTokenLength is the byte length of state tokens before base64 encoding.
	stateTokenLength = 32
)

// AuthHandler handles OAuth authentication endpoints.
type AuthHandler struct {
	providers *auth.Registry
	users     models.UserRepository
	sessions  models.SessionStore
	config    *config.Config
}

// NewAuthHandler creates a new authentication handler.
func NewAuthHandler(
	providers *auth.Registry,
	users models.UserRepository,
	sessions models.SessionStore,
	cfg *config.Config,
) *AuthHandler {
	return &AuthHandler{
		providers: providers,
		users:     users,
		sessions:  sessions,
		config:    cfg,
	}
}

// HandleLogin initiates the OAuth flow by redirecting to the provider.
// GET /auth/:provider
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	providerName := getProviderFromPath(r.URL.Path)
	if providerName == "" {
		http.Error(w, "Provider not specified", http.StatusBadRequest)
		return
	}

	provider, ok := h.providers.Get(providerName)
	if !ok {
		http.Error(w, "Unknown provider", http.StatusBadRequest)
		return
	}

	// Generate and store state token for CSRF protection
	state, err := generateStateToken()
	if err != nil {
		slog.Error("failed to generate state token", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	setStateCookie(w, state, h.isSecure())

	// Redirect to provider's authorization URL
	authURL := provider.AuthCodeURL(state)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// HandleCallback handles the OAuth callback from the provider.
// GET /auth/:provider/callback
func (h *AuthHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	providerName := getProviderFromPath(r.URL.Path)
	if providerName == "" {
		http.Error(w, "Provider not specified", http.StatusBadRequest)
		return
	}

	provider, ok := h.providers.Get(providerName)
	if !ok {
		http.Error(w, "Unknown provider", http.StatusBadRequest)
		return
	}

	// Verify state token
	state := r.URL.Query().Get("state")
	if !h.validateState(r, state) {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}
	clearStateCookie(w)

	// Check for error from provider
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		errDesc := r.URL.Query().Get("error_description")
		slog.Error("oauth error from provider", "provider", providerName, "error", errMsg, "description", errDesc)
		http.Error(w, "Authentication failed: "+errMsg, http.StatusBadRequest)
		return
	}

	// Exchange code for token
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	token, err := provider.Exchange(r.Context(), code)
	if err != nil {
		slog.Error("failed to exchange code", "provider", providerName, "error", err)
		http.Error(w, "Failed to authenticate", http.StatusInternalServerError)
		return
	}

	// Get user info from provider
	userInfo, err := provider.GetUser(r.Context(), token)
	if err != nil {
		slog.Error("failed to get user info", "provider", providerName, "error", err)
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}

	// Find or create user
	user, err := h.findOrCreateUser(r, providerName, userInfo, token.AccessToken, token.RefreshToken)
	if err != nil {
		slog.Error("failed to find or create user", "error", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	// Create session
	session, err := h.sessions.Create(r.Context(), user.ID)
	if err != nil {
		slog.Error("failed to create session", "error", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	models.SetSessionCookie(w, session, h.isSecure())

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

// HandleLogout terminates the user's session.
// POST /auth/logout
func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID, err := models.GetSessionFromRequest(r)
	if err == nil {
		// Delete the session from the store
		if err := h.sessions.Delete(r.Context(), sessionID); err != nil {
			slog.Error("failed to delete session", "error", err)
		}
	}

	models.ClearSessionCookie(w)

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

// HandleDevLogin handles auto-login in development mode.
// GET /auth/dev
func (h *AuthHandler) HandleDevLogin(w http.ResponseWriter, r *http.Request) {
	if !h.config.DevMode {
		http.Error(w, "Not available", http.StatusNotFound)
		return
	}

	// Find or create a dev user
	user, err := h.findOrCreateDevUser(r)
	if err != nil {
		slog.Error("failed to create dev user", "error", err)
		http.Error(w, "Failed to create dev user", http.StatusInternalServerError)
		return
	}

	// Create session
	session, err := h.sessions.Create(r.Context(), user.ID)
	if err != nil {
		slog.Error("failed to create session", "error", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	models.SetSessionCookie(w, session, h.isSecure())

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

// findOrCreateUser finds an existing user or creates a new one.
func (h *AuthHandler) findOrCreateUser(
	r *http.Request,
	provider string,
	info *auth.UserInfo,
	accessToken, refreshToken string,
) (*models.User, error) {
	ctx := r.Context()

	// Try to find existing user
	user, err := h.users.GetByProviderID(ctx, provider, info.ID)
	if err == nil {
		// Update user info and tokens
		user.Username = info.Username
		user.Email = info.Email
		user.AvatarURL = info.AvatarURL
		user.AccessToken = accessToken
		user.RefreshToken = refreshToken
		user.IsAdmin = h.config.IsAdmin(info.Username)

		if err := h.users.Update(ctx, user); err != nil {
			return nil, err
		}
		return user, nil
	}

	if !errors.Is(err, models.ErrNotFound) {
		return nil, err
	}

	// Create new user
	user = &models.User{
		Provider:     provider,
		ProviderID:   info.ID,
		Username:     info.Username,
		Email:        info.Email,
		AvatarURL:    info.AvatarURL,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		IsAdmin:      h.config.IsAdmin(info.Username),
	}

	if err := h.users.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// findOrCreateDevUser finds or creates the development mode user.
func (h *AuthHandler) findOrCreateDevUser(r *http.Request) (*models.User, error) {
	ctx := r.Context()

	// Use a fixed provider ID for the dev user
	devProviderID := "dev-user-001"
	devUsername := "dev"

	// Check if configured admins exist, use first one as dev username
	if len(h.config.Admins) > 0 {
		devUsername = h.config.Admins[0]
	}

	user, err := h.users.GetByProviderID(ctx, "dev", devProviderID)
	if err == nil {
		// Update admin status in case config changed
		user.IsAdmin = true
		user.Username = devUsername
		if err := h.users.Update(ctx, user); err != nil {
			return nil, err
		}
		return user, nil
	}

	if !errors.Is(err, models.ErrNotFound) {
		return nil, err
	}

	// Create dev user
	user = &models.User{
		Provider:   "dev",
		ProviderID: devProviderID,
		Username:   devUsername,
		Email:      "dev@localhost",
		IsAdmin:    true,
	}

	if err := h.users.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// isSecure returns true if the server is using HTTPS.
func (h *AuthHandler) isSecure() bool {
	return strings.HasPrefix(h.config.BaseURL, "https://")
}

// validateState checks that the state parameter matches the cookie.
func (h *AuthHandler) validateState(r *http.Request, state string) bool {
	cookie, err := r.Cookie(StateCookieName)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) == 1
}

// getProviderFromPath extracts the provider name from a URL path.
// Expected paths: /auth/:provider or /auth/:provider/callback
func getProviderFromPath(path string) string {
	// Remove leading slash and split
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	// parts[0] = "auth", parts[1] = provider name
	return parts[1]
}

// generateStateToken creates a cryptographically secure random state token.
func generateStateToken() (string, error) {
	b := make([]byte, stateTokenLength)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// setStateCookie sets the OAuth state cookie.
func setStateCookie(w http.ResponseWriter, state string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   int(StateCookieMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearStateCookie removes the OAuth state cookie.
func clearStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}
