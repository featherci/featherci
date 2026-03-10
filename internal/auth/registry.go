package auth

import (
	"github.com/featherci/featherci/internal/config"
)

// Registry manages available OAuth providers.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry creates a new provider registry based on configuration.
func NewRegistry(cfg *config.Config) *Registry {
	r := &Registry{
		providers: make(map[string]Provider),
	}

	// Register GitHub provider if configured
	if cfg.HasGitHubAuth() {
		callbackURL := cfg.BaseURL + "/auth/github/callback"
		r.providers["github"] = NewGitHubProvider(
			cfg.GitHubClientID,
			cfg.GitHubClientSecret,
			callbackURL,
		)
	}

	// Register GitLab provider if configured
	if cfg.HasGitLabAuth() {
		callbackURL := cfg.BaseURL + "/auth/gitlab/callback"
		r.providers["gitlab"] = NewGitLabProvider(
			cfg.GitLabClientID,
			cfg.GitLabClientSecret,
			callbackURL,
			cfg.GitLabURL,
		)
	}

	// Register Gitea provider if configured
	if cfg.HasGiteaAuth() {
		callbackURL := cfg.BaseURL + "/auth/gitea/callback"
		r.providers["gitea"] = NewGiteaProvider(
			cfg.GiteaClientID,
			cfg.GiteaClientSecret,
			callbackURL,
			cfg.GiteaURL,
		)
	}

	return r
}

// Get returns a provider by name.
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// Available returns a list of available provider names.
func (r *Registry) Available() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// Count returns the number of registered providers.
func (r *Registry) Count() int {
	return len(r.providers)
}
