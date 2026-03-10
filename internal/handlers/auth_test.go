package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/featherci/featherci/internal/auth"
	"github.com/featherci/featherci/internal/config"
	"github.com/featherci/featherci/internal/database"
	"github.com/featherci/featherci/internal/models"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}
	return db
}

func TestGetProviderFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/auth/github", "github"},
		{"/auth/github/callback", "github"},
		{"/auth/gitlab", "gitlab"},
		{"/auth/gitlab/callback", "gitlab"},
		{"/auth/gitea", "gitea"},
		{"/auth/gitea/callback", "gitea"},
		{"/auth/", ""},
		{"/auth", ""},
		{"/", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := getProviderFromPath(tt.path); got != tt.want {
				t.Errorf("getProviderFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestGenerateStateToken(t *testing.T) {
	token1, err := generateStateToken()
	if err != nil {
		t.Fatalf("generateStateToken() error = %v", err)
	}

	if token1 == "" {
		t.Error("generateStateToken() returned empty string")
	}

	// Generate another token to ensure uniqueness
	token2, err := generateStateToken()
	if err != nil {
		t.Fatalf("generateStateToken() error = %v", err)
	}

	if token1 == token2 {
		t.Error("generateStateToken() generated duplicate tokens")
	}
}

func TestHandleLogout(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := models.NewUserRepository(db.DB)
	sessionStore := models.NewSessionStore(db.DB)
	cfg := &config.Config{
		BaseURL: "http://localhost:8080",
	}

	// Create a test user and session
	user := &models.User{
		Provider:   "github",
		ProviderID: "12345",
		Username:   "testuser",
	}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("Create user error = %v", err)
	}

	session, err := sessionStore.Create(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("Create session error = %v", err)
	}

	handler := NewAuthHandler(nil, userRepo, sessionStore, cfg)

	// Test POST method
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: models.SessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	handler.HandleLogout(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Errorf("HandleLogout() status = %d, want %d", rec.Code, http.StatusTemporaryRedirect)
	}

	// Verify session was deleted
	_, err = sessionStore.Get(context.Background(), session.ID)
	if err != models.ErrNotFound {
		t.Error("HandleLogout() did not delete session")
	}

	// Check that session cookie was cleared
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == models.SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Error("HandleLogout() did not set session cookie")
	} else if sessionCookie.MaxAge != -1 {
		t.Errorf("HandleLogout() cookie MaxAge = %d, want -1", sessionCookie.MaxAge)
	}
}

func TestHandleLogout_MethodNotAllowed(t *testing.T) {
	cfg := &config.Config{
		BaseURL: "http://localhost:8080",
	}
	handler := NewAuthHandler(nil, nil, nil, cfg)

	req := httptest.NewRequest(http.MethodGet, "/auth/logout", nil)
	rec := httptest.NewRecorder()

	handler.HandleLogout(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("HandleLogout() GET status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDevLogin(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := models.NewUserRepository(db.DB)
	sessionStore := models.NewSessionStore(db.DB)
	cfg := &config.Config{
		BaseURL: "http://localhost:8080",
		DevMode: true,
		Admins:  []string{"devadmin"},
	}

	handler := NewAuthHandler(nil, userRepo, sessionStore, cfg)

	req := httptest.NewRequest(http.MethodGet, "/auth/dev", nil)
	rec := httptest.NewRecorder()

	handler.HandleDevLogin(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Errorf("HandleDevLogin() status = %d, want %d", rec.Code, http.StatusTemporaryRedirect)
	}

	// Verify session cookie was set
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == models.SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Error("HandleDevLogin() did not set session cookie")
	}

	// Verify user was created
	user, err := userRepo.GetByProviderID(context.Background(), "dev", "dev-user-001")
	if err != nil {
		t.Errorf("HandleDevLogin() did not create user: %v", err)
	}
	if user != nil && user.Username != "devadmin" {
		t.Errorf("HandleDevLogin() username = %q, want %q", user.Username, "devadmin")
	}
	if user != nil && !user.IsAdmin {
		t.Error("HandleDevLogin() user is not admin")
	}
}

func TestHandleDevLogin_NotAvailable(t *testing.T) {
	cfg := &config.Config{
		BaseURL: "http://localhost:8080",
		DevMode: false,
	}
	handler := NewAuthHandler(nil, nil, nil, cfg)

	req := httptest.NewRequest(http.MethodGet, "/auth/dev", nil)
	rec := httptest.NewRecorder()

	handler.HandleDevLogin(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("HandleDevLogin() status = %d, want %d (dev mode disabled)", rec.Code, http.StatusNotFound)
	}
}

func TestHandleLogin_UnknownProvider(t *testing.T) {
	cfg := &config.Config{
		BaseURL:   "http://localhost:8080",
		SecretKey: []byte("featherci-dev-key-do-not-use!!__"),
	}

	// Create an empty registry (no providers configured)
	emptyRegistry := auth.NewRegistry(cfg)

	handler := NewAuthHandler(emptyRegistry, nil, nil, cfg)

	req := httptest.NewRequest(http.MethodGet, "/auth/unknown", nil)
	rec := httptest.NewRecorder()

	handler.HandleLogin(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleLogin() unknown provider status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleCallback_NoState(t *testing.T) {
	cfg := &config.Config{
		BaseURL: "http://localhost:8080",
	}
	emptyRegistry := auth.NewRegistry(cfg)
	handler := NewAuthHandler(emptyRegistry, nil, nil, cfg)

	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=abc123", nil)
	rec := httptest.NewRecorder()

	handler.HandleCallback(rec, req)

	// Should fail because state doesn't match (no state cookie)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleCallback() no state status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleCallback_InvalidState(t *testing.T) {
	cfg := &config.Config{
		BaseURL: "http://localhost:8080",
	}
	emptyRegistry := auth.NewRegistry(cfg)
	handler := NewAuthHandler(emptyRegistry, nil, nil, cfg)

	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=abc123&state=wrongstate", nil)
	req.AddCookie(&http.Cookie{Name: StateCookieName, Value: "correctstate"})
	rec := httptest.NewRecorder()

	handler.HandleCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleCallback() invalid state status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestIsSecure(t *testing.T) {
	tests := []struct {
		baseURL string
		want    bool
	}{
		{"https://example.com", true},
		{"https://localhost:8443", true},
		{"http://localhost:8080", false},
		{"http://example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.baseURL, func(t *testing.T) {
			h := &AuthHandler{config: &config.Config{BaseURL: tt.baseURL}}
			if got := h.isSecure(); got != tt.want {
				t.Errorf("isSecure() = %v, want %v for baseURL %q", got, tt.want, tt.baseURL)
			}
		})
	}
}
