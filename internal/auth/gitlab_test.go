package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestGitLabProvider_Name(t *testing.T) {
	p := NewGitLabProvider("id", "secret", "http://localhost/callback", "https://gitlab.com")
	if p.Name() != "gitlab" {
		t.Errorf("Name() = %q, want %q", p.Name(), "gitlab")
	}
}

func TestGitLabProvider_AuthCodeURL(t *testing.T) {
	p := NewGitLabProvider("client-id", "secret", "http://localhost/callback", "https://gitlab.com")
	url := p.AuthCodeURL("test-state")

	if url == "" {
		t.Error("AuthCodeURL() returned empty string")
	}
	if !strings.Contains(url, "client_id=client-id") {
		t.Errorf("AuthCodeURL() missing client_id, got: %s", url)
	}
	if !strings.Contains(url, "state=test-state") {
		t.Errorf("AuthCodeURL() missing state, got: %s", url)
	}
	if !strings.Contains(url, "gitlab.com") {
		t.Errorf("AuthCodeURL() should use GitLab URL, got: %s", url)
	}
}

func TestGitLabProvider_GetUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/user" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":         67890,
				"username":   "gluser",
				"email":      "gluser@example.com",
				"avatar_url": "https://gitlab.com/avatar.png",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewGitLabProvider("client-id", "secret", "http://localhost/callback", server.URL)
	token := &oauth2.Token{AccessToken: "test-token"}
	ctx := testContext(server)

	user, err := p.GetUser(ctx, token)
	if err != nil {
		t.Fatalf("GetUser() error = %v", err)
	}
	if user.ID != "67890" {
		t.Errorf("ID = %q, want %q", user.ID, "67890")
	}
	if user.Username != "gluser" {
		t.Errorf("Username = %q, want %q", user.Username, "gluser")
	}
	if user.Email != "gluser@example.com" {
		t.Errorf("Email = %q, want %q", user.Email, "gluser@example.com")
	}
	if user.AvatarURL != "https://gitlab.com/avatar.png" {
		t.Errorf("AvatarURL = %q, want %q", user.AvatarURL, "https://gitlab.com/avatar.png")
	}
}

func TestGitLabProvider_GetUser_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewGitLabProvider("client-id", "secret", "http://localhost/callback", server.URL)
	token := &oauth2.Token{AccessToken: "test-token"}
	ctx := testContext(server)

	_, err := p.GetUser(ctx, token)
	if err == nil {
		t.Fatal("GetUser() expected error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status 500, got: %v", err)
	}
}

func TestGitLabProvider_GetRepositories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":                  100,
					"path_with_namespace": "group/project1",
					"path":                "project1",
					"namespace":           map[string]string{"path": "group"},
					"http_url_to_repo":    "https://gitlab.com/group/project1.git",
					"ssh_url_to_repo":     "git@gitlab.com:group/project1.git",
					"default_branch":      "main",
					"visibility":          "private",
					"permissions": map[string]interface{}{
						"project_access": map[string]interface{}{
							"access_level": 40, // Maintainer
						},
						"group_access": nil,
					},
				},
				{
					"id":                  101,
					"path_with_namespace": "group/project2",
					"path":                "project2",
					"namespace":           map[string]string{"path": "group"},
					"http_url_to_repo":    "https://gitlab.com/group/project2.git",
					"ssh_url_to_repo":     "git@gitlab.com:group/project2.git",
					"default_branch":      "develop",
					"visibility":          "public",
					"permissions": map[string]interface{}{
						"project_access": map[string]interface{}{
							"access_level": 30, // Developer
						},
						"group_access": nil,
					},
				},
				{
					"id":                  102,
					"path_with_namespace": "org/project3",
					"path":                "project3",
					"namespace":           map[string]string{"path": "org"},
					"http_url_to_repo":    "https://gitlab.com/org/project3.git",
					"ssh_url_to_repo":     "git@gitlab.com:org/project3.git",
					"default_branch":      "main",
					"visibility":          "internal",
					"permissions": map[string]interface{}{
						"project_access": nil,
						"group_access": map[string]interface{}{
							"access_level": 50, // Owner via group
						},
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewGitLabProvider("client-id", "secret", "http://localhost/callback", server.URL)
	token := &oauth2.Token{AccessToken: "test-token"}
	ctx := testContext(server)

	repos, err := p.GetRepositories(ctx, token)
	if err != nil {
		t.Fatalf("GetRepositories() error = %v", err)
	}
	if len(repos) != 3 {
		t.Fatalf("got %d repos, want 3", len(repos))
	}

	// Repo 1: Maintainer (40) on private project
	r := repos[0]
	if r.ID != "100" {
		t.Errorf("repos[0].ID = %q, want %q", r.ID, "100")
	}
	if r.FullName != "group/project1" {
		t.Errorf("repos[0].FullName = %q, want %q", r.FullName, "group/project1")
	}
	if r.Namespace != "group" {
		t.Errorf("repos[0].Namespace = %q, want %q", r.Namespace, "group")
	}
	if r.Name != "project1" {
		t.Errorf("repos[0].Name = %q, want %q", r.Name, "project1")
	}
	if r.CloneURL != "https://gitlab.com/group/project1.git" {
		t.Errorf("repos[0].CloneURL = %q", r.CloneURL)
	}
	if r.SSHURL != "git@gitlab.com:group/project1.git" {
		t.Errorf("repos[0].SSHURL = %q", r.SSHURL)
	}
	if r.DefaultBranch != "main" {
		t.Errorf("repos[0].DefaultBranch = %q, want %q", r.DefaultBranch, "main")
	}
	if !r.Private {
		t.Error("repos[0].Private = false, want true (visibility=private)")
	}
	if !r.Admin {
		t.Error("repos[0].Admin = false, want true (access_level=40)")
	}
	if !r.Push {
		t.Error("repos[0].Push = false, want true (access_level=40)")
	}

	// Repo 2: Developer (30) on public project
	r2 := repos[1]
	if r2.ID != "101" {
		t.Errorf("repos[1].ID = %q, want %q", r2.ID, "101")
	}
	if r2.Private {
		t.Error("repos[1].Private = true, want false (visibility=public)")
	}
	if r2.Admin {
		t.Error("repos[1].Admin = true, want false (access_level=30)")
	}
	if !r2.Push {
		t.Error("repos[1].Push = false, want true (access_level=30)")
	}

	// Repo 3: Owner (50) via group on internal project
	r3 := repos[2]
	if r3.ID != "102" {
		t.Errorf("repos[2].ID = %q, want %q", r3.ID, "102")
	}
	if !r3.Private {
		t.Error("repos[2].Private = false, want true (visibility=internal)")
	}
	if !r3.Admin {
		t.Error("repos[2].Admin = false, want true (access_level=50 via group)")
	}
	if !r3.Push {
		t.Error("repos[2].Push = false, want true (access_level=50 via group)")
	}
}
