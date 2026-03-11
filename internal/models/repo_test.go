package models

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func setupRepoTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	schema := `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			username TEXT NOT NULL,
			email TEXT,
			avatar_url TEXT,
			access_token TEXT,
			refresh_token TEXT,
			is_admin BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(provider, provider_id)
		);

		CREATE TABLE projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider TEXT NOT NULL,
			namespace TEXT NOT NULL,
			name TEXT NOT NULL,
			full_name TEXT NOT NULL,
			clone_url TEXT NOT NULL,
			webhook_secret TEXT,
			webhook_id TEXT DEFAULT '',
			default_branch TEXT DEFAULT 'main',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(provider, full_name)
		);

		CREATE TABLE project_users (
			project_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			can_manage BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (project_id, user_id),
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE TABLE builds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			build_number INTEGER NOT NULL,
			commit_sha TEXT NOT NULL,
			commit_message TEXT,
			commit_author TEXT,
			branch TEXT,
			pull_request_number INTEGER,
			status TEXT DEFAULT 'pending',
			started_at DATETIME,
			finished_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(project_id, build_number)
		);

		CREATE TABLE notification_channels (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			config_encrypted BLOB NOT NULL,
			on_success BOOLEAN NOT NULL DEFAULT 0,
			on_failure BOOLEAN NOT NULL DEFAULT 1,
			on_cancelled BOOLEAN NOT NULL DEFAULT 0,
			enabled BOOLEAN NOT NULL DEFAULT 1,
			created_by INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY (created_by) REFERENCES users(id)
		);

		CREATE TABLE secrets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			encrypted_value BLOB NOT NULL,
			created_by INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(project_id, name),
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY (created_by) REFERENCES users(id)
		);

		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

func createRepoTestProject(t *testing.T, db *sqlx.DB) *Project {
	t.Helper()
	repo := NewProjectRepository(db)
	project := &Project{
		Provider:      "github",
		Namespace:     "testorg",
		Name:          "testrepo",
		FullName:      "testorg/testrepo",
		CloneURL:      "https://github.com/testorg/testrepo.git",
		DefaultBranch: "main",
	}
	if err := repo.Create(context.Background(), project); err != nil {
		t.Fatalf("failed to create test project: %v", err)
	}
	return project
}

func createRepoTestUser(t *testing.T, db *sqlx.DB) *User {
	t.Helper()
	repo := NewUserRepository(db)
	user := &User{
		Provider:   "github",
		ProviderID: "99",
		Username:   "testuser",
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	return user
}

// --- NotificationChannelRepository tests ---

func TestNotificationChannelRepository_CreateAndGetByID(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	user := createRepoTestUser(t, db)
	repo := NewNotificationChannelRepository(db)
	ctx := context.Background()

	ch := &NotificationChannel{
		ProjectID:       project.ID,
		Name:            "slack-alerts",
		Type:            "slack",
		ConfigEncrypted: []byte("encrypted-config"),
		OnSuccess:       true,
		OnFailure:       true,
		OnCancelled:     false,
		Enabled:         true,
		CreatedBy:       user.ID,
	}
	if err := repo.Create(ctx, ch); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if ch.ID == 0 {
		t.Error("expected ID to be set")
	}

	got, err := repo.GetByID(ctx, ch.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Name != "slack-alerts" {
		t.Errorf("Name = %q, want %q", got.Name, "slack-alerts")
	}
	if got.Type != "slack" {
		t.Errorf("Type = %q, want %q", got.Type, "slack")
	}
	if !got.OnSuccess {
		t.Error("OnSuccess = false, want true")
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true")
	}
}

func TestNotificationChannelRepository_GetByID_NotFound(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	repo := NewNotificationChannelRepository(db)
	_, err := repo.GetByID(context.Background(), 99999)
	if err != ErrNotFound {
		t.Errorf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestNotificationChannelRepository_ListByProject(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	user := createRepoTestUser(t, db)
	repo := NewNotificationChannelRepository(db)
	ctx := context.Background()

	channels := []*NotificationChannel{
		{ProjectID: project.ID, Name: "email-alerts", Type: "email", ConfigEncrypted: []byte("cfg1"), OnFailure: true, Enabled: true, CreatedBy: user.ID},
		{ProjectID: project.ID, Name: "discord-alerts", Type: "discord", ConfigEncrypted: []byte("cfg2"), OnFailure: true, Enabled: true, CreatedBy: user.ID},
	}
	for _, ch := range channels {
		if err := repo.Create(ctx, ch); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	list, err := repo.ListByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListByProject() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	// Sorted by name ASC
	if list[0].Name != "discord-alerts" {
		t.Errorf("list[0].Name = %q, want %q", list[0].Name, "discord-alerts")
	}
}

func TestNotificationChannelRepository_Update(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	user := createRepoTestUser(t, db)
	repo := NewNotificationChannelRepository(db)
	ctx := context.Background()

	ch := &NotificationChannel{
		ProjectID:       project.ID,
		Name:            "old-name",
		Type:            "slack",
		ConfigEncrypted: []byte("old-config"),
		OnSuccess:       false,
		OnFailure:       true,
		Enabled:         true,
		CreatedBy:       user.ID,
	}
	if err := repo.Create(ctx, ch); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	ch.Name = "new-name"
	ch.ConfigEncrypted = []byte("new-config")
	ch.OnSuccess = true
	ch.Enabled = false
	if err := repo.Update(ctx, ch); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := repo.GetByID(ctx, ch.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Name != "new-name" {
		t.Errorf("Name = %q, want %q", got.Name, "new-name")
	}
	if !got.OnSuccess {
		t.Error("OnSuccess = false, want true")
	}
	if got.Enabled {
		t.Error("Enabled = true, want false")
	}
}

func TestNotificationChannelRepository_Update_NotFound(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	repo := NewNotificationChannelRepository(db)
	ch := &NotificationChannel{ID: 99999, Name: "x", ConfigEncrypted: []byte("x")}
	err := repo.Update(context.Background(), ch)
	if err != ErrNotFound {
		t.Errorf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestNotificationChannelRepository_Delete(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	user := createRepoTestUser(t, db)
	repo := NewNotificationChannelRepository(db)
	ctx := context.Background()

	ch := &NotificationChannel{
		ProjectID:       project.ID,
		Name:            "to-delete",
		Type:            "slack",
		ConfigEncrypted: []byte("cfg"),
		OnFailure:       true,
		Enabled:         true,
		CreatedBy:       user.ID,
	}
	if err := repo.Create(ctx, ch); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.Delete(ctx, ch.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err := repo.GetByID(ctx, ch.ID)
	if err != ErrNotFound {
		t.Errorf("GetByID after Delete() error = %v, want ErrNotFound", err)
	}
}

func TestNotificationChannelRepository_Delete_NotFound(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	repo := NewNotificationChannelRepository(db)
	err := repo.Delete(context.Background(), 99999)
	if err != ErrNotFound {
		t.Errorf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestNotificationChannelRepository_CountByProject(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	user := createRepoTestUser(t, db)
	repo := NewNotificationChannelRepository(db)
	ctx := context.Background()

	count, err := repo.CountByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountByProject() error = %v", err)
	}
	if count != 0 {
		t.Errorf("CountByProject() = %d, want 0", count)
	}

	for i := 0; i < 3; i++ {
		ch := &NotificationChannel{
			ProjectID:       project.ID,
			Name:            "ch-" + string(rune('a'+i)),
			Type:            "slack",
			ConfigEncrypted: []byte("cfg"),
			OnFailure:       true,
			Enabled:         true,
			CreatedBy:       user.ID,
		}
		if err := repo.Create(ctx, ch); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	count, err = repo.CountByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountByProject() error = %v", err)
	}
	if count != 3 {
		t.Errorf("CountByProject() = %d, want 3", count)
	}
}

// --- SecretRepository tests ---

func TestSecretRepository_CreateAndGetByName(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	user := createRepoTestUser(t, db)
	repo := NewSecretRepository(db)
	ctx := context.Background()

	secret := &Secret{
		ProjectID:      project.ID,
		Name:           "DB_PASSWORD",
		EncryptedValue: []byte("encrypted-value"),
		CreatedBy:      user.ID,
	}
	if err := repo.Create(ctx, secret); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if secret.ID == 0 {
		t.Error("expected ID to be set")
	}

	got, err := repo.GetByName(ctx, project.ID, "DB_PASSWORD")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}
	if got.Name != "DB_PASSWORD" {
		t.Errorf("Name = %q, want %q", got.Name, "DB_PASSWORD")
	}
	if string(got.EncryptedValue) != "encrypted-value" {
		t.Errorf("EncryptedValue = %q, want %q", got.EncryptedValue, "encrypted-value")
	}
}

func TestSecretRepository_GetByName_NotFound(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	repo := NewSecretRepository(db)

	_, err := repo.GetByName(context.Background(), project.ID, "NONEXISTENT")
	if err != ErrNotFound {
		t.Errorf("GetByName() error = %v, want ErrNotFound", err)
	}
}

func TestSecretRepository_ListByProject(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	user := createRepoTestUser(t, db)
	repo := NewSecretRepository(db)
	ctx := context.Background()

	secrets := []*Secret{
		{ProjectID: project.ID, Name: "SECRET_B", EncryptedValue: []byte("val1"), CreatedBy: user.ID},
		{ProjectID: project.ID, Name: "SECRET_A", EncryptedValue: []byte("val2"), CreatedBy: user.ID},
	}
	for _, s := range secrets {
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	list, err := repo.ListByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListByProject() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	// Sorted by name ASC
	if list[0].Name != "SECRET_A" {
		t.Errorf("list[0].Name = %q, want %q", list[0].Name, "SECRET_A")
	}
}

func TestSecretRepository_Update(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	user := createRepoTestUser(t, db)
	repo := NewSecretRepository(db)
	ctx := context.Background()

	secret := &Secret{
		ProjectID:      project.ID,
		Name:           "MY_SECRET",
		EncryptedValue: []byte("old-value"),
		CreatedBy:      user.ID,
	}
	if err := repo.Create(ctx, secret); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	secret.EncryptedValue = []byte("new-value")
	if err := repo.Update(ctx, secret); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := repo.GetByName(ctx, project.ID, "MY_SECRET")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}
	if string(got.EncryptedValue) != "new-value" {
		t.Errorf("EncryptedValue = %q, want %q", got.EncryptedValue, "new-value")
	}
}

func TestSecretRepository_Update_NotFound(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	repo := NewSecretRepository(db)
	secret := &Secret{ID: 99999, EncryptedValue: []byte("x")}
	err := repo.Update(context.Background(), secret)
	if err != ErrNotFound {
		t.Errorf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestSecretRepository_Delete(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	user := createRepoTestUser(t, db)
	repo := NewSecretRepository(db)
	ctx := context.Background()

	secret := &Secret{
		ProjectID:      project.ID,
		Name:           "TO_DELETE",
		EncryptedValue: []byte("val"),
		CreatedBy:      user.ID,
	}
	if err := repo.Create(ctx, secret); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.Delete(ctx, project.ID, "TO_DELETE"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err := repo.GetByName(ctx, project.ID, "TO_DELETE")
	if err != ErrNotFound {
		t.Errorf("GetByName after Delete() error = %v, want ErrNotFound", err)
	}
}

func TestSecretRepository_Delete_NotFound(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	repo := NewSecretRepository(db)
	err := repo.Delete(context.Background(), 99999, "NONEXISTENT")
	if err != ErrNotFound {
		t.Errorf("Delete() error = %v, want ErrNotFound", err)
	}
}

// --- BuildRepository additional tests ---

func TestBuildRepository_GetByNumber(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	repo := NewBuildRepository(db)
	ctx := context.Background()

	b1 := &Build{ProjectID: project.ID, CommitSHA: "aaa", Status: BuildStatusPending}
	b2 := &Build{ProjectID: project.ID, CommitSHA: "bbb", Status: BuildStatusPending}
	if err := repo.Create(ctx, b1); err != nil {
		t.Fatalf("Create b1 error = %v", err)
	}
	if err := repo.Create(ctx, b2); err != nil {
		t.Fatalf("Create b2 error = %v", err)
	}

	got, err := repo.GetByNumber(ctx, project.ID, 2)
	if err != nil {
		t.Fatalf("GetByNumber() error = %v", err)
	}
	if got.CommitSHA != "bbb" {
		t.Errorf("CommitSHA = %q, want %q", got.CommitSHA, "bbb")
	}
}

func TestBuildRepository_GetByNumber_NotFound(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	repo := NewBuildRepository(db)

	_, err := repo.GetByNumber(context.Background(), project.ID, 999)
	if err != ErrNotFound {
		t.Errorf("GetByNumber() error = %v, want ErrNotFound", err)
	}
}

func TestBuildRepository_ListPending(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	repo := NewBuildRepository(db)
	ctx := context.Background()

	// Create pending and non-pending builds
	b1 := &Build{ProjectID: project.ID, CommitSHA: "aaa", Status: BuildStatusPending}
	b2 := &Build{ProjectID: project.ID, CommitSHA: "bbb", Status: BuildStatusPending}
	b3 := &Build{ProjectID: project.ID, CommitSHA: "ccc", Status: BuildStatusRunning}
	for _, b := range []*Build{b1, b2, b3} {
		if err := repo.Create(ctx, b); err != nil {
			t.Fatalf("Create error = %v", err)
		}
	}
	// b3 needs to be set to running (Create sets status to what we pass)
	if err := repo.UpdateStatus(ctx, b3.ID, BuildStatusRunning); err != nil {
		t.Fatalf("UpdateStatus error = %v", err)
	}

	pending, err := repo.ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending() error = %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("len(pending) = %d, want 2", len(pending))
	}
}

func TestBuildRepository_Update(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	repo := NewBuildRepository(db)
	ctx := context.Background()

	build := &Build{ProjectID: project.ID, CommitSHA: "abc", Status: BuildStatusPending}
	if err := repo.Create(ctx, build); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	now := time.Now()
	build.Status = BuildStatusRunning
	build.StartedAt = &now
	if err := repo.Update(ctx, build); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := repo.GetByID(ctx, build.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != BuildStatusRunning {
		t.Errorf("Status = %q, want %q", got.Status, BuildStatusRunning)
	}
	if got.StartedAt == nil {
		t.Error("StartedAt should be set")
	}
}

func TestBuildRepository_Update_NotFound(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	repo := NewBuildRepository(db)
	build := &Build{ID: 99999, Status: BuildStatusRunning}
	err := repo.Update(context.Background(), build)
	if err != ErrNotFound {
		t.Errorf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestBuildRepository_CountByProject(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	repo := NewBuildRepository(db)
	ctx := context.Background()

	count, err := repo.CountByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountByProject() error = %v", err)
	}
	if count != 0 {
		t.Errorf("CountByProject() = %d, want 0", count)
	}

	for i := 0; i < 3; i++ {
		b := &Build{ProjectID: project.ID, CommitSHA: "sha" + string(rune('0'+i)), Status: BuildStatusPending}
		if err := repo.Create(ctx, b); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	count, err = repo.CountByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountByProject() error = %v", err)
	}
	if count != 3 {
		t.Errorf("CountByProject() = %d, want 3", count)
	}
}

func TestBuildRepository_Count(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	repo := NewBuildRepository(db)
	ctx := context.Background()

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Errorf("Count() = %d, want 0", count)
	}

	for i := 0; i < 2; i++ {
		b := &Build{ProjectID: project.ID, CommitSHA: "sha" + string(rune('0'+i)), Status: BuildStatusPending}
		if err := repo.Create(ctx, b); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	count, err = repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 2 {
		t.Errorf("Count() = %d, want 2", count)
	}
}

func TestBuildRepository_ListByUser(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	project := createRepoTestProject(t, db)
	user := createRepoTestUser(t, db)

	puRepo := NewProjectUserRepository(db)
	ctx := context.Background()
	if err := puRepo.Add(ctx, project.ID, user.ID, false); err != nil {
		t.Fatalf("Add project user error = %v", err)
	}

	buildRepo := NewBuildRepository(db)
	for i := 0; i < 3; i++ {
		b := &Build{ProjectID: project.ID, CommitSHA: "sha" + string(rune('0'+i)), Status: BuildStatusPending}
		if err := buildRepo.Create(ctx, b); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	builds, err := buildRepo.ListByUser(ctx, user.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListByUser() error = %v", err)
	}
	if len(builds) != 3 {
		t.Errorf("len(builds) = %d, want 3", len(builds))
	}

	// User with no projects should get empty list
	otherUser := &User{Provider: "github", ProviderID: "other", Username: "other"}
	userRepo := NewUserRepository(db)
	if err := userRepo.Create(ctx, otherUser); err != nil {
		t.Fatalf("Create other user error = %v", err)
	}
	builds, err = buildRepo.ListByUser(ctx, otherUser.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListByUser() error = %v", err)
	}
	if len(builds) != 0 {
		t.Errorf("len(builds) = %d, want 0", len(builds))
	}
}

// --- Session cookie helper tests ---

func TestSetSessionCookie(t *testing.T) {
	session := &Session{
		ID:        "test-session-id",
		UserID:    1,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}

	w := httptest.NewRecorder()
	SetSessionCookie(w, session, false)

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected a cookie to be set")
	}

	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatalf("cookie %q not found", SessionCookieName)
	}
	if found.Value != "test-session-id" {
		t.Errorf("cookie value = %q, want %q", found.Value, "test-session-id")
	}
	if !found.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
	if found.Secure {
		t.Error("cookie should not be Secure when secure=false")
	}
}

func TestSetSessionCookie_Secure(t *testing.T) {
	session := &Session{
		ID:        "test-session-id",
		UserID:    1,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	w := httptest.NewRecorder()
	SetSessionCookie(w, session, true)

	cookies := w.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatalf("cookie %q not found", SessionCookieName)
	}
	if !found.Secure {
		t.Error("cookie should be Secure when secure=true")
	}
}

func TestGetSessionFromRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "my-session-id"})

	sessionID, err := GetSessionFromRequest(req)
	if err != nil {
		t.Fatalf("GetSessionFromRequest() error = %v", err)
	}
	if sessionID != "my-session-id" {
		t.Errorf("sessionID = %q, want %q", sessionID, "my-session-id")
	}
}

func TestGetSessionFromRequest_NoCookie(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)

	_, err := GetSessionFromRequest(req)
	if err == nil {
		t.Error("expected error when no cookie is set")
	}
}

func TestClearSessionCookie(t *testing.T) {
	w := httptest.NewRecorder()
	ClearSessionCookie(w)

	cookies := w.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatalf("cookie %q not found", SessionCookieName)
	}
	if found.Value != "" {
		t.Errorf("cookie value = %q, want empty", found.Value)
	}
	if found.MaxAge != -1 {
		t.Errorf("MaxAge = %d, want -1", found.MaxAge)
	}
}

func TestSetAndGetSessionCookie_RoundTrip(t *testing.T) {
	session := &Session{
		ID:        "roundtrip-session",
		UserID:    42,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	// Set the cookie
	w := httptest.NewRecorder()
	SetSessionCookie(w, session, false)

	// Extract cookie from response and add to request
	cookies := w.Result().Cookies()
	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}

	// Get session from request
	sessionID, err := GetSessionFromRequest(req)
	if err != nil {
		t.Fatalf("GetSessionFromRequest() error = %v", err)
	}
	if sessionID != "roundtrip-session" {
		t.Errorf("sessionID = %q, want %q", sessionID, "roundtrip-session")
	}
}
