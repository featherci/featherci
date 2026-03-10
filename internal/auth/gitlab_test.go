package auth

import (
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

func TestGitLabProvider_SelfHosted(t *testing.T) {
	p := NewGitLabProvider("id", "secret", "http://localhost/callback", "https://gitlab.example.com/")

	// Should strip trailing slash
	if p.baseURL != "https://gitlab.example.com" {
		t.Errorf("baseURL = %q, want %q", p.baseURL, "https://gitlab.example.com")
	}

	// Auth URL should use custom base
	url := p.AuthCodeURL("state")
	if !strings.Contains(url, "gitlab.example.com") {
		t.Errorf("AuthCodeURL() should use custom GitLab URL, got: %s", url)
	}
}

func TestGitLabProvider_Endpoints(t *testing.T) {
	p := NewGitLabProvider("id", "secret", "http://localhost/callback", "https://gitlab.example.com")

	if p.config.Endpoint.AuthURL != "https://gitlab.example.com/oauth/authorize" {
		t.Errorf("AuthURL = %q, want %q", p.config.Endpoint.AuthURL, "https://gitlab.example.com/oauth/authorize")
	}
	if p.config.Endpoint.TokenURL != "https://gitlab.example.com/oauth/token" {
		t.Errorf("TokenURL = %q, want %q", p.config.Endpoint.TokenURL, "https://gitlab.example.com/oauth/token")
	}
}

func TestGitLabProvider_Scopes(t *testing.T) {
	p := NewGitLabProvider("id", "secret", "http://localhost/callback", "https://gitlab.com")

	expectedScopes := []string{"read_user", "api", "read_repository"}
	if len(p.config.Scopes) != len(expectedScopes) {
		t.Errorf("Scopes count = %d, want %d", len(p.config.Scopes), len(expectedScopes))
	}
	for i, scope := range expectedScopes {
		if p.config.Scopes[i] != scope {
			t.Errorf("Scopes[%d] = %q, want %q", i, p.config.Scopes[i], scope)
		}
	}
}

// Ensure GitLabProvider implements Provider interface
func TestGitLabProvider_ImplementsProvider(t *testing.T) {
	var _ Provider = (*GitLabProvider)(nil)
}

// Helper to ensure we have oauth2.Config
func TestGitLabProvider_HasOAuth2Config(t *testing.T) {
	p := NewGitLabProvider("id", "secret", "http://localhost/callback", "https://gitlab.com")
	var _ *oauth2.Config = p.config
}
