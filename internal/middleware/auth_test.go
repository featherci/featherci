package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func createTestUser(t *testing.T, repo *models.SQLiteUserRepository, username string, isAdmin bool) *models.User {
	t.Helper()
	user := &models.User{
		Provider:   "github",
		ProviderID: username + "-id",
		Username:   username,
		IsAdmin:    isAdmin,
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("Create user error = %v", err)
	}
	return user
}

func TestRequireAuth_Authenticated(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := models.NewUserRepository(db.DB)
	sessionStore := models.NewSessionStore(db.DB)
	middleware := NewAuthMiddleware(sessionStore, userRepo)

	user := createTestUser(t, userRepo, "testuser", false)
	session, _ := sessionStore.Create(context.Background(), user.ID)

	var capturedUser *models.User
	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: models.SessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("RequireAuth() status = %d, want %d", rec.Code, http.StatusOK)
	}
	if capturedUser == nil {
		t.Error("RequireAuth() did not set user in context")
	}
	if capturedUser != nil && capturedUser.Username != "testuser" {
		t.Errorf("RequireAuth() username = %q, want %q", capturedUser.Username, "testuser")
	}
}

func TestRequireAuth_NoSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := models.NewUserRepository(db.DB)
	sessionStore := models.NewSessionStore(db.DB)
	middleware := NewAuthMiddleware(sessionStore, userRepo)

	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("RequireAuth() status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_InvalidSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := models.NewUserRepository(db.DB)
	sessionStore := models.NewSessionStore(db.DB)
	middleware := NewAuthMiddleware(sessionStore, userRepo)

	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: models.SessionCookieName, Value: "invalid-session-id"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("RequireAuth() status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_ExpiredSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := models.NewUserRepository(db.DB)
	sessionStore := models.NewSessionStore(db.DB)
	middleware := NewAuthMiddleware(sessionStore, userRepo)

	user := createTestUser(t, userRepo, "testuser", false)

	// Insert an expired session directly
	expiredID := "expired-session-id"
	_, err := db.Exec(
		"INSERT INTO sessions (id, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)",
		expiredID, user.ID, time.Now().Add(-24*time.Hour), time.Now().Add(-48*time.Hour),
	)
	if err != nil {
		t.Fatalf("Insert expired session error = %v", err)
	}

	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: models.SessionCookieName, Value: expiredID})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("RequireAuth() status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestOptionalAuth_Authenticated(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := models.NewUserRepository(db.DB)
	sessionStore := models.NewSessionStore(db.DB)
	middleware := NewAuthMiddleware(sessionStore, userRepo)

	user := createTestUser(t, userRepo, "testuser", false)
	session, _ := sessionStore.Create(context.Background(), user.ID)

	var capturedUser *models.User
	handler := middleware.OptionalAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: models.SessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("OptionalAuth() status = %d, want %d", rec.Code, http.StatusOK)
	}
	if capturedUser == nil {
		t.Error("OptionalAuth() did not set user in context when authenticated")
	}
}

func TestOptionalAuth_NotAuthenticated(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := models.NewUserRepository(db.DB)
	sessionStore := models.NewSessionStore(db.DB)
	middleware := NewAuthMiddleware(sessionStore, userRepo)

	var capturedUser *models.User
	handler := middleware.OptionalAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("OptionalAuth() status = %d, want %d", rec.Code, http.StatusOK)
	}
	if capturedUser != nil {
		t.Error("OptionalAuth() should not set user in context when not authenticated")
	}
}

func TestRequireAdmin_Admin(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := models.NewUserRepository(db.DB)
	sessionStore := models.NewSessionStore(db.DB)
	middleware := NewAuthMiddleware(sessionStore, userRepo)

	user := createTestUser(t, userRepo, "adminuser", true)
	session, _ := sessionStore.Create(context.Background(), user.ID)

	handler := middleware.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: models.SessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("RequireAdmin() status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRequireAdmin_NotAdmin(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := models.NewUserRepository(db.DB)
	sessionStore := models.NewSessionStore(db.DB)
	middleware := NewAuthMiddleware(sessionStore, userRepo)

	user := createTestUser(t, userRepo, "regularuser", false)
	session, _ := sessionStore.Create(context.Background(), user.ID)

	handler := middleware.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: models.SessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("RequireAdmin() status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequireAdmin_NotAuthenticated(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := models.NewUserRepository(db.DB)
	sessionStore := models.NewSessionStore(db.DB)
	middleware := NewAuthMiddleware(sessionStore, userRepo)

	handler := middleware.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("RequireAdmin() status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestUserFromContext_NoUser(t *testing.T) {
	ctx := context.Background()
	user := UserFromContext(ctx)
	if user != nil {
		t.Error("UserFromContext() should return nil when no user in context")
	}
}

func TestSetUserInContext(t *testing.T) {
	user := &models.User{ID: 123, Username: "testuser"}
	ctx := SetUserInContext(context.Background(), user)

	retrieved := UserFromContext(ctx)
	if retrieved == nil {
		t.Error("UserFromContext() returned nil after SetUserInContext()")
	}
	if retrieved != nil && retrieved.ID != user.ID {
		t.Errorf("UserFromContext() ID = %d, want %d", retrieved.ID, user.ID)
	}
}
