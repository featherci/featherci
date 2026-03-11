package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

// urlRewriteTransport rewrites request URLs to point at a test server,
// preserving the request path and query string.
type urlRewriteTransport struct {
	server *httptest.Server
}

func (t *urlRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.server.URL, "http://")
	return http.DefaultTransport.RoundTrip(req)
}

// testContext returns a context that forces oauth2's config.Client to use the
// given httptest.Server instead of the real endpoint.
func testContext(server *httptest.Server) context.Context {
	httpClient := &http.Client{Transport: &urlRewriteTransport{server: server}}
	return context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)
}

func TestGitHubProvider_Name(t *testing.T) {
	p := NewGitHubProvider("id", "secret", "http://localhost/callback")
	if p.Name() != "github" {
		t.Errorf("Name() = %q, want %q", p.Name(), "github")
	}
}

func TestGitHubProvider_AuthCodeURL(t *testing.T) {
	p := NewGitHubProvider("client-id", "secret", "http://localhost/callback")
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
}

func TestGitHubProvider_GetUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":         12345,
				"login":      "testuser",
				"email":      "test@example.com",
				"avatar_url": "https://example.com/avatar.png",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewGitHubProvider("client-id", "secret", "http://localhost/callback")
	token := &oauth2.Token{AccessToken: "test-token"}
	ctx := testContext(server)

	user, err := p.GetUser(ctx, token)
	if err != nil {
		t.Fatalf("GetUser() error = %v", err)
	}
	if user.ID != "12345" {
		t.Errorf("ID = %q, want %q", user.ID, "12345")
	}
	if user.Username != "testuser" {
		t.Errorf("Username = %q, want %q", user.Username, "testuser")
	}
	if user.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", user.Email, "test@example.com")
	}
	if user.AvatarURL != "https://example.com/avatar.png" {
		t.Errorf("AvatarURL = %q, want %q", user.AvatarURL, "https://example.com/avatar.png")
	}
}

func TestGitHubProvider_GetUser_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewGitHubProvider("client-id", "secret", "http://localhost/callback")
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

func TestGitHubProvider_GetRepositories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/repos" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":             1,
					"full_name":      "owner/repo1",
					"name":           "repo1",
					"owner":          map[string]string{"login": "owner"},
					"clone_url":      "https://github.com/owner/repo1.git",
					"ssh_url":        "git@github.com:owner/repo1.git",
					"default_branch": "main",
					"private":        false,
					"permissions": map[string]bool{
						"admin": true,
						"push":  true,
						"pull":  true,
					},
				},
				{
					"id":             2,
					"full_name":      "owner/repo2",
					"name":           "repo2",
					"owner":          map[string]string{"login": "owner"},
					"clone_url":      "https://github.com/owner/repo2.git",
					"ssh_url":        "git@github.com:owner/repo2.git",
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

	p := NewGitHubProvider("client-id", "secret", "http://localhost/callback")
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
	if r.ID != "1" {
		t.Errorf("repos[0].ID = %q, want %q", r.ID, "1")
	}
	if r.FullName != "owner/repo1" {
		t.Errorf("repos[0].FullName = %q, want %q", r.FullName, "owner/repo1")
	}
	if r.Namespace != "owner" {
		t.Errorf("repos[0].Namespace = %q, want %q", r.Namespace, "owner")
	}
	if r.Name != "repo1" {
		t.Errorf("repos[0].Name = %q, want %q", r.Name, "repo1")
	}
	if r.CloneURL != "https://github.com/owner/repo1.git" {
		t.Errorf("repos[0].CloneURL = %q", r.CloneURL)
	}
	if r.SSHURL != "git@github.com:owner/repo1.git" {
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
	if r2.ID != "2" {
		t.Errorf("repos[1].ID = %q, want %q", r2.ID, "2")
	}
	if !r2.Private {
		t.Error("repos[1].Private = false, want true")
	}
	if r2.Admin {
		t.Error("repos[1].Admin = true, want false")
	}
	if r2.DefaultBranch != "develop" {
		t.Errorf("repos[1].DefaultBranch = %q, want %q", r2.DefaultBranch, "develop")
	}
}

func TestGitHubProvider_GetRepositories_Pagination(t *testing.T) {
	page1Called := false
	page2Called := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/repos" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")

		if page == "" || page == "1" {
			page1Called = true
			// Return exactly 100 repos to trigger pagination
			repos := make([]map[string]interface{}, 100)
			for i := 0; i < 100; i++ {
				repos[i] = map[string]interface{}{
					"id":             i + 1,
					"full_name":      "owner/repo",
					"name":           "repo",
					"owner":          map[string]string{"login": "owner"},
					"clone_url":      "https://github.com/owner/repo.git",
					"ssh_url":        "git@github.com:owner/repo.git",
					"default_branch": "main",
					"private":        false,
					"permissions": map[string]bool{
						"admin": false,
						"push":  false,
						"pull":  true,
					},
				}
			}
			json.NewEncoder(w).Encode(repos)
			return
		}

		if page == "2" {
			page2Called = true
			// Return empty to stop pagination
			json.NewEncoder(w).Encode([]map[string]interface{}{})
			return
		}

		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	p := NewGitHubProvider("client-id", "secret", "http://localhost/callback")
	token := &oauth2.Token{AccessToken: "test-token"}
	ctx := testContext(server)

	repos, err := p.GetRepositories(ctx, token)
	if err != nil {
		t.Fatalf("GetRepositories() error = %v", err)
	}
	if len(repos) != 100 {
		t.Errorf("got %d repos, want 100", len(repos))
	}
	if !page1Called {
		t.Error("page 1 was not requested")
	}
	if !page2Called {
		t.Error("page 2 was not requested (pagination not triggered)")
	}
}
