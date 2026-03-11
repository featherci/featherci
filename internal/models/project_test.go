package models

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func setupProjectTestDB(t *testing.T) *sqlx.DB {
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
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

func createProjectTestUser(t *testing.T, db *sqlx.DB, providerID, username string) *User {
	t.Helper()
	repo := NewUserRepository(db)
	user := &User{
		Provider:   "github",
		ProviderID: providerID,
		Username:   username,
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	return user
}

func TestProjectRepository_CreateAndGetByID(t *testing.T) {
	db := setupProjectTestDB(t)
	defer db.Close()

	repo := NewProjectRepository(db)
	ctx := context.Background()

	project := &Project{
		Provider:      "github",
		Namespace:     "myorg",
		Name:          "myrepo",
		FullName:      "myorg/myrepo",
		CloneURL:      "https://github.com/myorg/myrepo.git",
		DefaultBranch: "main",
	}
	if err := repo.Create(ctx, project); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if project.ID == 0 {
		t.Error("expected project.ID to be set")
	}
	if project.WebhookSecret == "" {
		t.Error("expected WebhookSecret to be auto-generated")
	}

	got, err := repo.GetByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.FullName != "myorg/myrepo" {
		t.Errorf("FullName = %q, want %q", got.FullName, "myorg/myrepo")
	}
	if got.CloneURL != project.CloneURL {
		t.Errorf("CloneURL = %q, want %q", got.CloneURL, project.CloneURL)
	}
}

func TestProjectRepository_GetByID_NotFound(t *testing.T) {
	db := setupProjectTestDB(t)
	defer db.Close()

	repo := NewProjectRepository(db)
	_, err := repo.GetByID(context.Background(), 99999)
	if err != ErrNotFound {
		t.Errorf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestProjectRepository_GetByFullName(t *testing.T) {
	db := setupProjectTestDB(t)
	defer db.Close()

	repo := NewProjectRepository(db)
	ctx := context.Background()

	project := &Project{
		Provider:      "github",
		Namespace:     "myorg",
		Name:          "myrepo",
		FullName:      "myorg/myrepo",
		CloneURL:      "https://github.com/myorg/myrepo.git",
		DefaultBranch: "main",
	}
	if err := repo.Create(ctx, project); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByFullName(ctx, "github", "myorg/myrepo")
	if err != nil {
		t.Fatalf("GetByFullName() error = %v", err)
	}
	if got.ID != project.ID {
		t.Errorf("ID = %d, want %d", got.ID, project.ID)
	}
}

func TestProjectRepository_GetByFullName_NotFound(t *testing.T) {
	db := setupProjectTestDB(t)
	defer db.Close()

	repo := NewProjectRepository(db)
	_, err := repo.GetByFullName(context.Background(), "github", "nonexistent/repo")
	if err != ErrNotFound {
		t.Errorf("GetByFullName() error = %v, want ErrNotFound", err)
	}
}

func TestProjectRepository_List(t *testing.T) {
	db := setupProjectTestDB(t)
	defer db.Close()

	repo := NewProjectRepository(db)
	ctx := context.Background()

	projects := []*Project{
		{Provider: "github", Namespace: "org1", Name: "repoB", FullName: "org1/repoB", CloneURL: "https://github.com/org1/repoB.git", DefaultBranch: "main"},
		{Provider: "github", Namespace: "org1", Name: "repoA", FullName: "org1/repoA", CloneURL: "https://github.com/org1/repoA.git", DefaultBranch: "main"},
		{Provider: "github", Namespace: "org2", Name: "repoC", FullName: "org2/repoC", CloneURL: "https://github.com/org2/repoC.git", DefaultBranch: "main"},
	}
	for _, p := range projects {
		if err := repo.Create(ctx, p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len(list) = %d, want 3", len(list))
	}
	// Should be sorted by full_name ASC
	if list[0].FullName != "org1/repoA" {
		t.Errorf("list[0].FullName = %q, want %q", list[0].FullName, "org1/repoA")
	}
	if list[1].FullName != "org1/repoB" {
		t.Errorf("list[1].FullName = %q, want %q", list[1].FullName, "org1/repoB")
	}
}

func TestProjectRepository_ListWithStatus(t *testing.T) {
	db := setupProjectTestDB(t)
	defer db.Close()

	projRepo := NewProjectRepository(db)
	buildRepo := NewBuildRepository(db)
	ctx := context.Background()

	p1 := &Project{Provider: "github", Namespace: "org", Name: "repo1", FullName: "org/repo1", CloneURL: "https://example.com/1.git", DefaultBranch: "main"}
	p2 := &Project{Provider: "github", Namespace: "org", Name: "repo2", FullName: "org/repo2", CloneURL: "https://example.com/2.git", DefaultBranch: "main"}

	if err := projRepo.Create(ctx, p1); err != nil {
		t.Fatalf("Create p1 error = %v", err)
	}
	if err := projRepo.Create(ctx, p2); err != nil {
		t.Fatalf("Create p2 error = %v", err)
	}

	// Create builds for p1: first pending, second success
	b1 := &Build{ProjectID: p1.ID, CommitSHA: "aaa", Status: BuildStatusPending}
	b2 := &Build{ProjectID: p1.ID, CommitSHA: "bbb", Status: BuildStatusSuccess}
	if err := buildRepo.Create(ctx, b1); err != nil {
		t.Fatalf("Create b1 error = %v", err)
	}
	if err := buildRepo.Create(ctx, b2); err != nil {
		t.Fatalf("Create b2 error = %v", err)
	}
	// Update b2 status to success
	if err := buildRepo.UpdateStatus(ctx, b2.ID, BuildStatusSuccess); err != nil {
		t.Fatalf("UpdateStatus error = %v", err)
	}

	list, err := projRepo.ListWithStatus(ctx)
	if err != nil {
		t.Fatalf("ListWithStatus() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}

	// p1 should have last build status = success (build_number 2 is the max)
	for _, p := range list {
		if p.FullName == "org/repo1" {
			if p.LastBuildStatus == nil {
				t.Error("expected LastBuildStatus for repo1 to be set")
			} else if *p.LastBuildStatus != string(BuildStatusSuccess) {
				t.Errorf("LastBuildStatus = %q, want %q", *p.LastBuildStatus, BuildStatusSuccess)
			}
		}
		if p.FullName == "org/repo2" {
			if p.LastBuildStatus != nil {
				t.Errorf("expected LastBuildStatus for repo2 to be nil, got %q", *p.LastBuildStatus)
			}
		}
	}
}

func TestProjectRepository_CountAll(t *testing.T) {
	db := setupProjectTestDB(t)
	defer db.Close()

	repo := NewProjectRepository(db)
	ctx := context.Background()

	count, err := repo.CountAll(ctx)
	if err != nil {
		t.Fatalf("CountAll() error = %v", err)
	}
	if count != 0 {
		t.Errorf("CountAll() = %d, want 0", count)
	}

	for i := 0; i < 3; i++ {
		p := &Project{
			Provider:      "github",
			Namespace:     "org",
			Name:          string(rune('a' + i)),
			FullName:      "org/" + string(rune('a'+i)),
			CloneURL:      "https://example.com/repo.git",
			DefaultBranch: "main",
		}
		if err := repo.Create(ctx, p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	count, err = repo.CountAll(ctx)
	if err != nil {
		t.Fatalf("CountAll() error = %v", err)
	}
	if count != 3 {
		t.Errorf("CountAll() = %d, want 3", count)
	}
}

func TestProjectRepository_Update(t *testing.T) {
	db := setupProjectTestDB(t)
	defer db.Close()

	repo := NewProjectRepository(db)
	ctx := context.Background()

	project := &Project{
		Provider:      "github",
		Namespace:     "org",
		Name:          "oldname",
		FullName:      "org/oldname",
		CloneURL:      "https://github.com/org/oldname.git",
		DefaultBranch: "main",
	}
	if err := repo.Create(ctx, project); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	project.Name = "newname"
	project.FullName = "org/newname"
	project.DefaultBranch = "develop"
	if err := repo.Update(ctx, project); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := repo.GetByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Name != "newname" {
		t.Errorf("Name = %q, want %q", got.Name, "newname")
	}
	if got.FullName != "org/newname" {
		t.Errorf("FullName = %q, want %q", got.FullName, "org/newname")
	}
	if got.DefaultBranch != "develop" {
		t.Errorf("DefaultBranch = %q, want %q", got.DefaultBranch, "develop")
	}
}

func TestProjectRepository_Delete(t *testing.T) {
	db := setupProjectTestDB(t)
	defer db.Close()

	repo := NewProjectRepository(db)
	ctx := context.Background()

	project := &Project{
		Provider:      "github",
		Namespace:     "org",
		Name:          "repo",
		FullName:      "org/repo",
		CloneURL:      "https://github.com/org/repo.git",
		DefaultBranch: "main",
	}
	if err := repo.Create(ctx, project); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.Delete(ctx, project.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err := repo.GetByID(ctx, project.ID)
	if err != ErrNotFound {
		t.Errorf("GetByID after Delete() error = %v, want ErrNotFound", err)
	}
}

// --- ProjectUserRepository tests ---

func TestProjectUserRepository_AddAndGetUsersForProject(t *testing.T) {
	db := setupProjectTestDB(t)
	defer db.Close()

	projRepo := NewProjectRepository(db)
	puRepo := NewProjectUserRepository(db)
	ctx := context.Background()

	project := &Project{Provider: "github", Namespace: "org", Name: "repo", FullName: "org/repo", CloneURL: "https://example.com/repo.git", DefaultBranch: "main"}
	if err := projRepo.Create(ctx, project); err != nil {
		t.Fatalf("Create project error = %v", err)
	}

	user1 := createProjectTestUser(t, db, "1", "alice")
	user2 := createProjectTestUser(t, db, "2", "bob")

	if err := puRepo.Add(ctx, project.ID, user1.ID, true); err != nil {
		t.Fatalf("Add user1 error = %v", err)
	}
	if err := puRepo.Add(ctx, project.ID, user2.ID, false); err != nil {
		t.Fatalf("Add user2 error = %v", err)
	}

	users, err := puRepo.GetUsersForProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetUsersForProject() error = %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("len(users) = %d, want 2", len(users))
	}
	// Should be sorted by username ASC
	if users[0].Username != "alice" {
		t.Errorf("users[0].Username = %q, want %q", users[0].Username, "alice")
	}
	if users[1].Username != "bob" {
		t.Errorf("users[1].Username = %q, want %q", users[1].Username, "bob")
	}
}

func TestProjectUserRepository_GetProjectsForUser(t *testing.T) {
	db := setupProjectTestDB(t)
	defer db.Close()

	projRepo := NewProjectRepository(db)
	puRepo := NewProjectUserRepository(db)
	ctx := context.Background()

	p1 := &Project{Provider: "github", Namespace: "org", Name: "repoA", FullName: "org/repoA", CloneURL: "https://example.com/a.git", DefaultBranch: "main"}
	p2 := &Project{Provider: "github", Namespace: "org", Name: "repoB", FullName: "org/repoB", CloneURL: "https://example.com/b.git", DefaultBranch: "main"}
	p3 := &Project{Provider: "github", Namespace: "org", Name: "repoC", FullName: "org/repoC", CloneURL: "https://example.com/c.git", DefaultBranch: "main"}

	for _, p := range []*Project{p1, p2, p3} {
		if err := projRepo.Create(ctx, p); err != nil {
			t.Fatalf("Create project error = %v", err)
		}
	}

	user := createProjectTestUser(t, db, "1", "alice")

	// user has access to p1 and p3
	if err := puRepo.Add(ctx, p1.ID, user.ID, false); err != nil {
		t.Fatalf("Add error = %v", err)
	}
	if err := puRepo.Add(ctx, p3.ID, user.ID, false); err != nil {
		t.Fatalf("Add error = %v", err)
	}

	projects, err := puRepo.GetProjectsForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetProjectsForUser() error = %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("len(projects) = %d, want 2", len(projects))
	}
	if projects[0].FullName != "org/repoA" {
		t.Errorf("projects[0].FullName = %q, want %q", projects[0].FullName, "org/repoA")
	}
	if projects[1].FullName != "org/repoC" {
		t.Errorf("projects[1].FullName = %q, want %q", projects[1].FullName, "org/repoC")
	}
}

func TestProjectUserRepository_GetProjectsForUserWithStatus(t *testing.T) {
	db := setupProjectTestDB(t)
	defer db.Close()

	projRepo := NewProjectRepository(db)
	buildRepo := NewBuildRepository(db)
	puRepo := NewProjectUserRepository(db)
	ctx := context.Background()

	p1 := &Project{Provider: "github", Namespace: "org", Name: "repo1", FullName: "org/repo1", CloneURL: "https://example.com/1.git", DefaultBranch: "main"}
	if err := projRepo.Create(ctx, p1); err != nil {
		t.Fatalf("Create project error = %v", err)
	}

	user := createProjectTestUser(t, db, "1", "alice")
	if err := puRepo.Add(ctx, p1.ID, user.ID, false); err != nil {
		t.Fatalf("Add error = %v", err)
	}

	// Create a build
	b := &Build{ProjectID: p1.ID, CommitSHA: "abc", Status: BuildStatusFailure}
	if err := buildRepo.Create(ctx, b); err != nil {
		t.Fatalf("Create build error = %v", err)
	}
	if err := buildRepo.UpdateStatus(ctx, b.ID, BuildStatusFailure); err != nil {
		t.Fatalf("UpdateStatus error = %v", err)
	}

	projects, err := puRepo.GetProjectsForUserWithStatus(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetProjectsForUserWithStatus() error = %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("len(projects) = %d, want 1", len(projects))
	}
	if projects[0].LastBuildStatus == nil {
		t.Fatal("expected LastBuildStatus to be set")
	}
	if *projects[0].LastBuildStatus != string(BuildStatusFailure) {
		t.Errorf("LastBuildStatus = %q, want %q", *projects[0].LastBuildStatus, BuildStatusFailure)
	}
}

func TestProjectUserRepository_CanUserAccess(t *testing.T) {
	db := setupProjectTestDB(t)
	defer db.Close()

	projRepo := NewProjectRepository(db)
	puRepo := NewProjectUserRepository(db)
	ctx := context.Background()

	project := &Project{Provider: "github", Namespace: "org", Name: "repo", FullName: "org/repo", CloneURL: "https://example.com/repo.git", DefaultBranch: "main"}
	if err := projRepo.Create(ctx, project); err != nil {
		t.Fatalf("Create project error = %v", err)
	}

	user := createProjectTestUser(t, db, "1", "alice")

	// Before adding
	can, err := puRepo.CanUserAccess(ctx, project.ID, user.ID)
	if err != nil {
		t.Fatalf("CanUserAccess() error = %v", err)
	}
	if can {
		t.Error("CanUserAccess() = true before Add, want false")
	}

	// After adding
	if err := puRepo.Add(ctx, project.ID, user.ID, false); err != nil {
		t.Fatalf("Add error = %v", err)
	}

	can, err = puRepo.CanUserAccess(ctx, project.ID, user.ID)
	if err != nil {
		t.Fatalf("CanUserAccess() error = %v", err)
	}
	if !can {
		t.Error("CanUserAccess() = false after Add, want true")
	}
}

func TestProjectUserRepository_CanUserManage(t *testing.T) {
	db := setupProjectTestDB(t)
	defer db.Close()

	projRepo := NewProjectRepository(db)
	puRepo := NewProjectUserRepository(db)
	ctx := context.Background()

	project := &Project{Provider: "github", Namespace: "org", Name: "repo", FullName: "org/repo", CloneURL: "https://example.com/repo.git", DefaultBranch: "main"}
	if err := projRepo.Create(ctx, project); err != nil {
		t.Fatalf("Create project error = %v", err)
	}

	user1 := createProjectTestUser(t, db, "1", "manager")
	user2 := createProjectTestUser(t, db, "2", "viewer")

	if err := puRepo.Add(ctx, project.ID, user1.ID, true); err != nil {
		t.Fatalf("Add error = %v", err)
	}
	if err := puRepo.Add(ctx, project.ID, user2.ID, false); err != nil {
		t.Fatalf("Add error = %v", err)
	}

	can, err := puRepo.CanUserManage(ctx, project.ID, user1.ID)
	if err != nil {
		t.Fatalf("CanUserManage() error = %v", err)
	}
	if !can {
		t.Error("CanUserManage() = false for manager, want true")
	}

	can, err = puRepo.CanUserManage(ctx, project.ID, user2.ID)
	if err != nil {
		t.Fatalf("CanUserManage() error = %v", err)
	}
	if can {
		t.Error("CanUserManage() = true for viewer, want false")
	}

	// Non-existent user
	can, err = puRepo.CanUserManage(ctx, project.ID, 99999)
	if err != nil {
		t.Fatalf("CanUserManage() error = %v", err)
	}
	if can {
		t.Error("CanUserManage() = true for nonexistent user, want false")
	}
}

func TestProjectUserRepository_Remove(t *testing.T) {
	db := setupProjectTestDB(t)
	defer db.Close()

	projRepo := NewProjectRepository(db)
	puRepo := NewProjectUserRepository(db)
	ctx := context.Background()

	project := &Project{Provider: "github", Namespace: "org", Name: "repo", FullName: "org/repo", CloneURL: "https://example.com/repo.git", DefaultBranch: "main"}
	if err := projRepo.Create(ctx, project); err != nil {
		t.Fatalf("Create project error = %v", err)
	}

	user := createProjectTestUser(t, db, "1", "alice")
	if err := puRepo.Add(ctx, project.ID, user.ID, true); err != nil {
		t.Fatalf("Add error = %v", err)
	}

	// Verify access
	can, _ := puRepo.CanUserAccess(ctx, project.ID, user.ID)
	if !can {
		t.Fatal("expected user to have access before Remove")
	}

	// Remove
	if err := puRepo.Remove(ctx, project.ID, user.ID); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	// Verify no access
	can, _ = puRepo.CanUserAccess(ctx, project.ID, user.ID)
	if can {
		t.Error("CanUserAccess() = true after Remove, want false")
	}
}
