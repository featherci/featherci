package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/featherci/featherci/internal/config"
	"github.com/featherci/featherci/internal/database"
)

func setupTestServer(t *testing.T) (*Server, *database.DB) {
	t.Helper()

	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	cfg := &config.Config{
		BaseURL:   "http://localhost:8080",
		BindAddr:  ":8080",
		SecretKey: []byte("featherci-dev-key-do-not-use!!__"),
		DevMode:   true,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, err := New(cfg, db, logger)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	return srv, db
}

func TestHealth(t *testing.T) {
	srv, db := setupTestServer(t)
	defer db.Close()

	router := srv.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("health status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("health status = %q, want %q", response["status"], "ok")
	}
}

func TestReady(t *testing.T) {
	srv, db := setupTestServer(t)
	defer db.Close()

	router := srv.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ready status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["status"] != "ready" {
		t.Errorf("ready status = %q, want %q", response["status"], "ready")
	}
}

func TestDashboard_NotAuthenticated(t *testing.T) {
	srv, db := setupTestServer(t)
	defer db.Close()

	router := srv.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("dashboard status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !containsString(body, "Sign in") {
		t.Error("dashboard should show login options when not authenticated")
	}
	if !containsString(body, "Dev Login") {
		t.Error("dashboard should show dev login in dev mode")
	}
}

func TestDashboard_Authenticated(t *testing.T) {
	srv, db := setupTestServer(t)
	defer db.Close()

	// First do dev login to get a session
	router := srv.setupRoutes()

	// Do dev login
	loginReq := httptest.NewRequest(http.MethodGet, "/auth/dev", nil)
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)

	// Extract session cookie
	var sessionCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == "featherci_session" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Fatal("dev login did not set session cookie")
	}

	// Request dashboard with session
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("dashboard status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !containsString(body, "Welcome") {
		t.Errorf("dashboard should show welcome message when authenticated, got: %s", body[:min(len(body), 500)])
	}
	if !containsString(body, "Sign out") {
		t.Error("dashboard should show sign out link when authenticated")
	}
}

func TestDevLogin(t *testing.T) {
	srv, db := setupTestServer(t)
	defer db.Close()

	router := srv.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/auth/dev", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Errorf("dev login status = %d, want %d", rec.Code, http.StatusTemporaryRedirect)
	}

	// Check session cookie was set
	var found bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == "featherci_session" && c.Value != "" {
			found = true
			break
		}
	}

	if !found {
		t.Error("dev login should set session cookie")
	}
}

func TestDevLogin_DisabledInProduction(t *testing.T) {
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	cfg := &config.Config{
		BaseURL:   "http://localhost:8080",
		BindAddr:  ":8080",
		SecretKey: []byte("featherci-dev-key-do-not-use!!__"),
		DevMode:   false, // Production mode
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, err := New(cfg, db, logger)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	router := srv.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/auth/dev", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// Should get 400 because route falls through to /auth/{provider}
	// which returns "Unknown provider" for "dev"
	if rec.Code != http.StatusBadRequest {
		t.Errorf("dev login in production status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestWorkerAPIRequiresAuth(t *testing.T) {
	// Create a server with WorkerSecret set to enable worker API routes
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}
	cfg := &config.Config{
		BaseURL:      "http://localhost:8080",
		BindAddr:     ":8080",
		SecretKey:    []byte("featherci-dev-key-do-not-use!!__"),
		DevMode:      true,
		WorkerSecret: "test-secret",
		Mode:         config.ModeStandalone,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, err := New(cfg, db, logger)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	router := srv.setupRoutes()

	// Request without auth should be rejected
	req := httptest.NewRequest(http.MethodGet, "/api/worker/steps/ready", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("worker API without auth status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// Request with correct auth should succeed
	req = httptest.NewRequest(http.MethodGet, "/api/worker/steps/ready", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("worker API with auth status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestServerStart_Shutdown(t *testing.T) {
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	cfg := &config.Config{
		BaseURL:   "http://localhost:18080",
		BindAddr:  "127.0.0.1:18080",
		SecretKey: []byte("featherci-dev-key-do-not-use!!__"),
		DevMode:   true,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, err := New(cfg, db, logger)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Give server time to start
	// We'll just cancel immediately to test graceful shutdown
	cancel()

	err = <-errCh
	if err != nil {
		t.Errorf("server shutdown error: %v", err)
	}
}

func TestProjectsListRequiresAuth(t *testing.T) {
	srv, db := setupTestServer(t)
	defer db.Close()

	router := srv.setupRoutes()

	// Try to access projects without auth
	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// Should return 401 Unauthorized
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("projects without auth status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestProjectsListWithAuth(t *testing.T) {
	srv, db := setupTestServer(t)
	defer db.Close()

	router := srv.setupRoutes()

	// First get a session via dev login
	loginReq := httptest.NewRequest(http.MethodGet, "/auth/dev", nil)
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)

	var sessionCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == "featherci_session" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Fatal("no session cookie returned from dev login")
	}

	// Now access projects with auth
	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("projects with auth status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Should contain the projects page content
	body := rec.Body.String()
	if !containsString(body, "Projects") {
		t.Errorf("response body should contain 'Projects'")
	}
}

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || len(needle) == 0 ||
		(len(haystack) > 0 && containsSubstring(haystack, needle)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
