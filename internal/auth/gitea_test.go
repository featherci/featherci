package auth

import (
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

func TestGiteaProvider_TrailingSlash(t *testing.T) {
	p := NewGiteaProvider("id", "secret", "http://localhost/callback", "https://gitea.example.com/")

	// Should strip trailing slash
	if p.baseURL != "https://gitea.example.com" {
		t.Errorf("baseURL = %q, want %q", p.baseURL, "https://gitea.example.com")
	}
}

func TestGiteaProvider_Endpoints(t *testing.T) {
	p := NewGiteaProvider("id", "secret", "http://localhost/callback", "https://gitea.example.com")

	if p.config.Endpoint.AuthURL != "https://gitea.example.com/login/oauth/authorize" {
		t.Errorf("AuthURL = %q, want %q", p.config.Endpoint.AuthURL, "https://gitea.example.com/login/oauth/authorize")
	}
	if p.config.Endpoint.TokenURL != "https://gitea.example.com/login/oauth/access_token" {
		t.Errorf("TokenURL = %q, want %q", p.config.Endpoint.TokenURL, "https://gitea.example.com/login/oauth/access_token")
	}
}

func TestGiteaProvider_Scopes(t *testing.T) {
	p := NewGiteaProvider("id", "secret", "http://localhost/callback", "https://gitea.example.com")

	expectedScopes := []string{"read:user", "read:repository", "write:repository"}
	if len(p.config.Scopes) != len(expectedScopes) {
		t.Errorf("Scopes count = %d, want %d", len(p.config.Scopes), len(expectedScopes))
	}
	for i, scope := range expectedScopes {
		if p.config.Scopes[i] != scope {
			t.Errorf("Scopes[%d] = %q, want %q", i, p.config.Scopes[i], scope)
		}
	}
}

// Ensure GiteaProvider implements Provider interface
func TestGiteaProvider_ImplementsProvider(t *testing.T) {
	var _ Provider = (*GiteaProvider)(nil)
}

// Helper to ensure we have oauth2.Config
func TestGiteaProvider_HasOAuth2Config(t *testing.T) {
	p := NewGiteaProvider("id", "secret", "http://localhost/callback", "https://gitea.example.com")
	var _ *oauth2.Config = p.config
}
