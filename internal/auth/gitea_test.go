package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestGiteaProvider_Name(t *testing.T) {
	p := NewGiteaProvider("id", "secret", "http://localhost/callback", "https://gitea.example.com")
	if p.Name() != "gitea" {
		t.Errorf("Name() = %q, want %q", p.Name(), "gitea")
	}
}

func TestGiteaProvider_AuthCodeURL(t *testing.T) {
	p := NewGiteaProvider("client-id", "secret", "http://localhost/callback", "https://gitea.example.com")
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
	if !strings.Contains(url, "gitea.example.com") {
		t.Errorf("AuthCodeURL() should use Gitea URL, got: %s", url)
	}
}

func TestGiteaProvider_GetUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/user" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":         99999,
				"login":      "giteauser",
				"email":      "giteauser@example.com",
				"avatar_url": "https://gitea.example.com/avatar.png",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewGiteaProvider("client-id", "secret", "http://localhost/callback", server.URL)
	token := &oauth2.Token{AccessToken: "test-token"}
	ctx := testContext(server)

	user, err := p.GetUser(ctx, token)
	if err != nil {
		t.Fatalf("GetUser() error = %v", err)
	}
	if user.ID != "99999" {
		t.Errorf("ID = %q, want %q", user.ID, "99999")
	}
	if user.Username != "giteauser" {
		t.Errorf("Username = %q, want %q", user.Username, "giteauser")
	}
	if user.Email != "giteauser@example.com" {
		t.Errorf("Email = %q, want %q", user.Email, "giteauser@example.com")
	}
	if user.AvatarURL != "https://gitea.example.com/avatar.png" {
		t.Errorf("AvatarURL = %q, want %q", user.AvatarURL, "https://gitea.example.com/avatar.png")
	}
}

func TestGiteaProvider_GetUser_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewGiteaProvider("client-id", "secret", "http://localhost/callback", server.URL)
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

func TestGiteaProvider_GetRepositories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/user/repos" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":             501,
					"full_name":      "giteauser/myrepo",
					"name":           "myrepo",
					"owner":          map[string]string{"login": "giteauser"},
					"clone_url":      "https://gitea.example.com/giteauser/myrepo.git",
					"ssh_url":        "git@gitea.example.com:giteauser/myrepo.git",
					"default_branch": "main",
					"private":        false,
					"permissions": map[string]bool{
						"admin": true,
						"push":  true,
						"pull":  true,
					},
				},
				{
					"id":             502,
					"full_name":      "org/private-repo",
					"name":           "private-repo",
					"owner":          map[string]string{"login": "org"},
					"clone_url":      "https://gitea.example.com/org/private-repo.git",
					"ssh_url":        "git@gitea.example.com:org/private-repo.git",
					"default_branch": "develop",
					"private":        true,
					"permissions": map[string]bool{
						"admin": false,
						"push":  true,
						"pull":  true,
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewGiteaProvider("client-id", "secret", "http://localhost/callback", server.URL)
	token := &oauth2.Token{AccessToken: "test-token"}
	ctx := testContext(server)

	repos, err := p.GetRepositories(ctx, token)
	if err != nil {
		t.Fatalf("GetRepositories() error = %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("got %d repos, want 2", len(repos))
	}

	r := repos[0]
	if r.ID != "501" {
		t.Errorf("repos[0].ID = %q, want %q", r.ID, "501")
	}
	if r.FullName != "giteauser/myrepo" {
		t.Errorf("repos[0].FullName = %q, want %q", r.FullName, "giteauser/myrepo")
	}
	if r.Namespace != "giteauser" {
		t.Errorf("repos[0].Namespace = %q, want %q", r.Namespace, "giteauser")
	}
	if r.Name != "myrepo" {
		t.Errorf("repos[0].Name = %q, want %q", r.Name, "myrepo")
	}
	if r.CloneURL != "https://gitea.example.com/giteauser/myrepo.git" {
		t.Errorf("repos[0].CloneURL = %q", r.CloneURL)
	}
	if r.SSHURL != "git@gitea.example.com:giteauser/myrepo.git" {
		t.Errorf("repos[0].SSHURL = %q", r.SSHURL)
	}
	if r.DefaultBranch != "main" {
		t.Errorf("repos[0].DefaultBranch = %q, want %q", r.DefaultBranch, "main")
	}
	if r.Private {
		t.Error("repos[0].Private = true, want false")
	}
	if !r.Admin {
		t.Error("repos[0].Admin = false, want true")
	}
	if !r.Push {
		t.Error("repos[0].Push = false, want true")
	}

	r2 := repos[1]
	if r2.ID != "502" {
		t.Errorf("repos[1].ID = %q, want %q", r2.ID, "502")
	}
	if !r2.Private {
		t.Error("repos[1].Private = false, want true")
	}
	if r2.Admin {
		t.Error("repos[1].Admin = true, want false")
	}
	if !r2.Push {
		t.Error("repos[1].Push = false, want true")
	}
	if r2.DefaultBranch != "develop" {
		t.Errorf("repos[1].DefaultBranch = %q, want %q", r2.DefaultBranch, "develop")
	}
}
