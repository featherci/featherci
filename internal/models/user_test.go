package models

import (
	"context"
	"testing"
	"time"

	"github.com/featherci/featherci/internal/database"
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

func TestUserRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewUserRepository(db.DB)
	ctx := context.Background()

	user := &User{
		Provider:   "github",
		ProviderID: "12345",
		Username:   "testuser",
		Email:      "test@example.com",
		AvatarURL:  "https://example.com/avatar.png",
		IsAdmin:    false,
	}

	err := repo.Create(ctx, user)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if user.ID == 0 {
		t.Error("Create() did not set user ID")
	}
	if user.CreatedAt.IsZero() {
		t.Error("Create() did not set CreatedAt")
	}
	if user.UpdatedAt.IsZero() {
		t.Error("Create() did not set UpdatedAt")
	}
}

func TestUserRepository_GetByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewUserRepository(db.DB)
	ctx := context.Background()

	// Create a user first
	user := &User{
		Provider:   "github",
		ProviderID: "12345",
		Username:   "testuser",
		Email:      "test@example.com",
	}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Test GetByID
	found, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if found.Username != user.Username {
		t.Errorf("GetByID() username = %q, want %q", found.Username, user.Username)
	}
	if found.ProviderID != user.ProviderID {
		t.Errorf("GetByID() providerID = %q, want %q", found.ProviderID, user.ProviderID)
	}
}

func TestUserRepository_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewUserRepository(db.DB)
	ctx := context.Background()

	_, err := repo.GetByID(ctx, 99999)
	if err != ErrNotFound {
		t.Errorf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestUserRepository_GetByProviderID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewUserRepository(db.DB)
	ctx := context.Background()

	user := &User{
		Provider:   "github",
		ProviderID: "12345",
		Username:   "testuser",
		Email:      "test@example.com",
	}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	found, err := repo.GetByProviderID(ctx, "github", "12345")
	if err != nil {
		t.Fatalf("GetByProviderID() error = %v", err)
	}

	if found.ID != user.ID {
		t.Errorf("GetByProviderID() ID = %d, want %d", found.ID, user.ID)
	}
}

func TestUserRepository_GetByProviderID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewUserRepository(db.DB)
	ctx := context.Background()

	_, err := repo.GetByProviderID(ctx, "github", "nonexistent")
	if err != ErrNotFound {
		t.Errorf("GetByProviderID() error = %v, want ErrNotFound", err)
	}
}

func TestUserRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewUserRepository(db.DB)
	ctx := context.Background()

	user := &User{
		Provider:   "github",
		ProviderID: "12345",
		Username:   "testuser",
		Email:      "test@example.com",
		IsAdmin:    false,
	}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update user
	user.Username = "newusername"
	user.IsAdmin = true
	if err := repo.Update(ctx, user); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify update
	found, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if found.Username != "newusername" {
		t.Errorf("Update() username = %q, want %q", found.Username, "newusername")
	}
	if !found.IsAdmin {
		t.Error("Update() IsAdmin = false, want true")
	}
}

func TestUserRepository_Update_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewUserRepository(db.DB)
	ctx := context.Background()

	user := &User{
		ID:       99999,
		Username: "ghost",
	}

	err := repo.Update(ctx, user)
	if err != ErrNotFound {
		t.Errorf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestUserRepository_UpdateTokens(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewUserRepository(db.DB)
	ctx := context.Background()

	user := &User{
		Provider:     "github",
		ProviderID:   "12345",
		Username:     "testuser",
		AccessToken:  "old_token",
		RefreshToken: "old_refresh",
	}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err := repo.UpdateTokens(ctx, user.ID, "new_token", "new_refresh")
	if err != nil {
		t.Fatalf("UpdateTokens() error = %v", err)
	}

	found, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if found.AccessToken != "new_token" {
		t.Errorf("UpdateTokens() AccessToken = %q, want %q", found.AccessToken, "new_token")
	}
	if found.RefreshToken != "new_refresh" {
		t.Errorf("UpdateTokens() RefreshToken = %q, want %q", found.RefreshToken, "new_refresh")
	}
}

func TestUserRepository_List(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewUserRepository(db.DB)
	ctx := context.Background()

	// Create multiple users
	users := []*User{
		{Provider: "github", ProviderID: "1", Username: "user1"},
		{Provider: "github", ProviderID: "2", Username: "user2"},
		{Provider: "gitlab", ProviderID: "3", Username: "user3"},
	}

	for _, u := range users {
		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("List() returned %d users, want 3", len(list))
	}
}

func TestUserRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewUserRepository(db.DB)
	ctx := context.Background()

	user := &User{
		Provider:   "github",
		ProviderID: "12345",
		Username:   "testuser",
	}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err := repo.Delete(ctx, user.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(ctx, user.ID)
	if err != ErrNotFound {
		t.Errorf("GetByID after Delete() error = %v, want ErrNotFound", err)
	}
}

func TestUserRepository_Delete_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewUserRepository(db.DB)
	ctx := context.Background()

	err := repo.Delete(ctx, 99999)
	if err != ErrNotFound {
		t.Errorf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestUserRepository_UniqueConstraint(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewUserRepository(db.DB)
	ctx := context.Background()

	user1 := &User{
		Provider:   "github",
		ProviderID: "12345",
		Username:   "user1",
	}
	if err := repo.Create(ctx, user1); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Try to create another user with same provider+providerID
	user2 := &User{
		Provider:   "github",
		ProviderID: "12345",
		Username:   "user2",
	}
	err := repo.Create(ctx, user2)
	if err == nil {
		t.Error("Create() should fail with duplicate provider+providerID")
	}
}

// Session tests
func TestSessionStore_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a user first
	userRepo := NewUserRepository(db.DB)
	user := &User{Provider: "github", ProviderID: "1", Username: "testuser"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("Create user error = %v", err)
	}

	store := NewSessionStore(db.DB)
	ctx := context.Background()

	session, err := store.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if session.ID == "" {
		t.Error("Create() did not set session ID")
	}
	if session.UserID != user.ID {
		t.Errorf("Create() UserID = %d, want %d", session.UserID, user.ID)
	}
	if session.IsExpired() {
		t.Error("Create() created an expired session")
	}
}

func TestSessionStore_Get(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := NewUserRepository(db.DB)
	user := &User{Provider: "github", ProviderID: "1", Username: "testuser"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("Create user error = %v", err)
	}

	store := NewSessionStore(db.DB)
	ctx := context.Background()

	session, err := store.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	found, err := store.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if found.ID != session.ID {
		t.Errorf("Get() ID = %q, want %q", found.ID, session.ID)
	}
	if found.UserID != session.UserID {
		t.Errorf("Get() UserID = %d, want %d", found.UserID, session.UserID)
	}
}

func TestSessionStore_Get_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := NewSessionStore(db.DB)
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent-session-id")
	if err != ErrNotFound {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestSessionStore_Delete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := NewUserRepository(db.DB)
	user := &User{Provider: "github", ProviderID: "1", Username: "testuser"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("Create user error = %v", err)
	}

	store := NewSessionStore(db.DB)
	ctx := context.Background()

	session, err := store.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err = store.Delete(ctx, session.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = store.Get(ctx, session.ID)
	if err != ErrNotFound {
		t.Errorf("Get after Delete() error = %v, want ErrNotFound", err)
	}
}

func TestSessionStore_DeleteAllForUser(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := NewUserRepository(db.DB)
	user := &User{Provider: "github", ProviderID: "1", Username: "testuser"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("Create user error = %v", err)
	}

	store := NewSessionStore(db.DB)
	ctx := context.Background()

	// Create multiple sessions for the user
	session1, _ := store.Create(ctx, user.ID)
	session2, _ := store.Create(ctx, user.ID)

	err := store.DeleteAllForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("DeleteAllForUser() error = %v", err)
	}

	// Both sessions should be gone
	_, err = store.Get(ctx, session1.ID)
	if err != ErrNotFound {
		t.Errorf("Get session1 after DeleteAllForUser() error = %v, want ErrNotFound", err)
	}
	_, err = store.Get(ctx, session2.ID)
	if err != ErrNotFound {
		t.Errorf("Get session2 after DeleteAllForUser() error = %v, want ErrNotFound", err)
	}
}

func TestSessionStore_Cleanup(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := NewUserRepository(db.DB)
	user := &User{Provider: "github", ProviderID: "1", Username: "testuser"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("Create user error = %v", err)
	}

	store := NewSessionStore(db.DB)
	ctx := context.Background()

	// Create a valid session
	validSession, _ := store.Create(ctx, user.ID)

	// Manually insert an expired session
	expiredID := "expired-session-id"
	_, err := db.Exec(
		"INSERT INTO sessions (id, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)",
		expiredID, user.ID, time.Now().Add(-24*time.Hour), time.Now().Add(-48*time.Hour),
	)
	if err != nil {
		t.Fatalf("Insert expired session error = %v", err)
	}

	err = store.Cleanup(ctx)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	// Valid session should still exist
	_, err = store.Get(ctx, validSession.ID)
	if err != nil {
		t.Errorf("Get valid session after Cleanup() error = %v", err)
	}

	// Expired session should be gone
	_, err = store.Get(ctx, expiredID)
	if err != ErrNotFound {
		t.Errorf("Get expired session after Cleanup() error = %v, want ErrNotFound", err)
	}
}

func TestSession_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "not expired",
			expiresAt: time.Now().Add(time.Hour),
			want:      false,
		},
		{
			name:      "expired",
			expiresAt: time.Now().Add(-time.Hour),
			want:      true,
		},
		{
			name:      "just expired",
			expiresAt: time.Now().Add(-time.Second),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{ExpiresAt: tt.expiresAt}
			if got := s.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSessionStore_CascadeDelete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userRepo := NewUserRepository(db.DB)
	user := &User{Provider: "github", ProviderID: "1", Username: "testuser"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("Create user error = %v", err)
	}

	store := NewSessionStore(db.DB)
	ctx := context.Background()

	session, _ := store.Create(ctx, user.ID)

	// Delete the user - session should be cascade deleted
	if err := userRepo.Delete(ctx, user.ID); err != nil {
		t.Fatalf("Delete user error = %v", err)
	}

	_, err := store.Get(ctx, session.ID)
	if err != ErrNotFound {
		t.Errorf("Session should be cascade deleted when user is deleted, got error = %v", err)
	}
}
