package auth

import (
	"testing"

	"github.com/featherci/featherci/internal/config"
)

func TestRegistry_Empty(t *testing.T) {
	cfg := &config.Config{}
	r := NewRegistry(cfg)

	if r.Count() != 0 {
		t.Errorf("Count() = %d, want 0", r.Count())
	}

	available := r.Available()
	if len(available) != 0 {
		t.Errorf("Available() = %v, want empty", available)
	}
}

func TestRegistry_Get(t *testing.T) {
	cfg := &config.Config{
		BaseURL:            "http://localhost:8080",
		GitHubClientID:     "github-id",
		GitHubClientSecret: "github-secret",
	}
	r := NewRegistry(cfg)

	if r.Count() != 1 {
		t.Errorf("Count() = %d, want 1", r.Count())
	}

	p, ok := r.Get("github")
	if !ok {
		t.Fatal("Get(github) returned false")
	}
	if p.Name() != "github" {
		t.Errorf("Name() = %q, want %q", p.Name(), "github")
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	cfg := &config.Config{
		BaseURL:            "http://localhost:8080",
		GitHubClientID:     "github-id",
		GitHubClientSecret: "github-secret",
	}
	r := NewRegistry(cfg)

	_, ok := r.Get("gitlab")
	if ok {
		t.Error("Get(gitlab) should return false when GitLab is not configured")
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("Get(nonexistent) should return false")
	}
}

func TestRegistry_AllProviders(t *testing.T) {
	cfg := &config.Config{
		BaseURL:            "http://localhost:8080",
		GitHubClientID:     "github-id",
		GitHubClientSecret: "github-secret",
		GitLabURL:          "https://gitlab.com",
		GitLabClientID:     "gitlab-id",
		GitLabClientSecret: "gitlab-secret",
		GiteaURL:           "https://gitea.example.com",
		GiteaClientID:      "gitea-id",
		GiteaClientSecret:  "gitea-secret",
	}
	r := NewRegistry(cfg)

	if r.Count() != 3 {
		t.Errorf("Count() = %d, want 3", r.Count())
	}

	available := r.Available()
	if len(available) != 3 {
		t.Errorf("Available() length = %d, want 3", len(available))
	}

	for _, name := range []string{"github", "gitlab", "gitea"} {
		p, ok := r.Get(name)
		if !ok {
			t.Errorf("Get(%s) returned false", name)
			continue
		}
		if p.Name() != name {
			t.Errorf("Get(%s).Name() = %q, want %q", name, p.Name(), name)
		}
	}
}

func TestRegistry_CallbackURLs(t *testing.T) {
	cfg := &config.Config{
		BaseURL:            "https://ci.example.com",
		GitHubClientID:     "github-id",
		GitHubClientSecret: "github-secret",
	}
	r := NewRegistry(cfg)

	p, _ := r.Get("github")
	ghp := p.(*GitHubProvider)

	if ghp.config.RedirectURL != "https://ci.example.com/auth/github/callback" {
		t.Errorf("RedirectURL = %q, want %q", ghp.config.RedirectURL, "https://ci.example.com/auth/github/callback")
	}
}
